// Package system wraps the handful of external commands the GUI needs to
// run: talking to the Asterisk CLI, restarting services, and reading
// basic host status. All command arguments are passed as separate exec
// args (never through a shell), so nothing here is shell-injectable.
package system

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// AsteriskRX runs `<bin> -rx "<cmd>"` and returns its output. cmd is a
// single Asterisk CLI command (e.g. "rpt reload", "iax2 show registry").
// bin is normally "asterisk" (resolved via PATH), but on distributions
// that install it somewhere non-standard (e.g. HamVoIP's
// /usr/local/hamvoip-asterisk/sbin/asterisk) it needs to be the full
// path — see the -asterisk-bin flag.
func AsteriskRX(ctx context.Context, bin, cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, bin, "-rx", cmd)
	var out, stderr bytes.Buffer
	c.Stdout = &out
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("%s -rx %q: %w: %s", bin, cmd, err, stderr.String())
	}
	return out.String(), nil
}

// AsteriskRunning reports whether Asterisk is up and responding to CLI
// commands. This deliberately doesn't go through systemd/systemctl:
// Asterisk is very often supervised some other way (a safe_asterisk
// watchdog script, a custom init script, etc.) rather than as a native
// systemd unit, which makes asking Asterisk itself the only check that
// works regardless of how it's actually being run.
func AsteriskRunning(ctx context.Context, bin string) bool {
	_, err := AsteriskRX(ctx, bin, "core show version")
	return err == nil
}

// AsteriskRestart restarts Asterisk via its own CLI restart command
// rather than through systemd, for the same reason as AsteriskRunning:
// this works regardless of what (if anything) is supervising the
// process. "restart now" restarts immediately rather than waiting for
// active channels to clear, matching what an operator clicking a
// "restart" button in the UI expects to happen.
func AsteriskRestart(ctx context.Context, bin string) error {
	_, err := AsteriskRX(ctx, bin, "restart now")
	return err
}

// ServiceRestart restarts a systemd unit. Not used for Asterisk itself
// (see AsteriskRestart) — kept for any other systemd-managed service
// this tool might need to restart in the future.
func ServiceRestart(ctx context.Context, unit string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	c := exec.CommandContext(ctx, "systemctl", "restart", unit)
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("systemctl restart %s: %w: %s", unit, err, stderr.String())
	}
	return nil
}

// Uptime returns the output of `uptime -p` (e.g. "up 3 days, 2 hours").
func Uptime(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "uptime", "-p").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Hostname returns the current system hostname.
func Hostname(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "hostname").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// hostnameRe matches a single valid RFC 1123 hostname label.
var hostnameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// SetHostname validates and applies a new hostname: writes /etc/hostname
// and applies it to the running system immediately via the `hostname`
// command. A reboot is recommended afterward so everything that cached
// the old name (avahi/mDNS, Asterisk's own startup banner, etc.) picks
// up the change.
func SetHostname(ctx context.Context, name string) error {
	if !hostnameRe.MatchString(name) {
		return fmt.Errorf("invalid hostname %q", name)
	}
	if err := os.WriteFile("/etc/hostname", []byte(name+"\n"), 0644); err != nil {
		return fmt.Errorf("write /etc/hostname: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	c := exec.CommandContext(ctx, "hostname", name)
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("hostname %s: %w: %s", name, err, stderr.String())
	}
	return nil
}

// Reboot schedules an immediate system reboot. It returns once the
// command has been issued, not once the reboot completes — the HTTP
// response will race the shutdown, so callers should tell the user to
// expect the connection to drop.
func Reboot(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	c := exec.CommandContext(ctx, "systemctl", "reboot")
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("systemctl reboot: %w: %s", err, stderr.String())
	}
	return nil
}

// LogTail returns roughly the last n lines of path, without reading the
// whole file into memory: it seeks to the last maxTailBytes of the file
// (if the file is larger than that) before scanning for line breaks.
func LogTail(path string, n int) ([]string, error) {
	const maxTailBytes = 512 * 1024

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	start := int64(0)
	if fi.Size() > maxTailBytes {
		start = fi.Size() - maxTailBytes
	}
	if _, err := f.Seek(start, 0); err != nil {
		return nil, err
	}

	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	// A trailing newline would otherwise split into a bogus empty final
	// "line" that displaces the real last line when we take the tail.
	buf = bytes.TrimSuffix(buf, []byte("\n"))

	lines := strings.Split(string(buf), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // drop a possibly-truncated first line
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// SoundCard is one entry from /proc/asound/cards: a sound device the
// kernel currently sees, which is what a HamVoIP node's USB radio
// interface shows up as. HamVoIP conventionally renames each card's ID
// to match the device name used in usbradio.conf/simpleusb.conf (e.g.
// "usb", "usb1") via a udev rule, so the ID here is normally exactly
// what a device stanza should be named.
type SoundCard struct {
	Index       int
	ID          string
	Description string
	IsUSB       bool
}

var soundCardLineRe = regexp.MustCompile(`^\s*(\d+)\s*\[([^\]]*)\]:\s*(.*)$`)

// ListSoundCards reads /proc/asound/cards. Returns an empty slice (not
// an error) if the file doesn't exist, e.g. when running off a Pi —
// callers should treat "no cards" as "detection unavailable, fall back
// to manual entry" rather than a hard failure.
func ListSoundCards() ([]SoundCard, error) {
	data, err := os.ReadFile("/proc/asound/cards")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseSoundCards(string(data)), nil
}

func parseSoundCards(data string) []SoundCard {
	var cards []SoundCard
	for _, line := range strings.Split(data, "\n") {
		m := soundCardLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		index := 0
		fmt.Sscanf(m[1], "%d", &index)
		id := strings.TrimSpace(m[2])
		desc := strings.TrimSpace(m[3])
		cards = append(cards, SoundCard{
			Index:       index,
			ID:          id,
			Description: desc,
			IsUSB:       strings.Contains(strings.ToUpper(desc), "USB"),
		})
	}
	return cards
}

// ListNetworkInterfaces lists network interface names the kernel
// currently knows about (e.g. "eth0", "wlan0"), read from
// /sys/class/net so it includes interfaces that don't have an address
// yet — unlike netconfig.ListInterfaces, which only sees ones `ip`
// reports as already addressed. Returns nil (not an error) if
// unavailable, e.g. off-Pi.
func ListNetworkInterfaces() ([]string, error) {
	return listNetworkInterfaces("/sys/class/net")
}

func listNetworkInterfaces(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.Name() == "lo" {
			continue // loopback isn't a useful choice for repeater networking
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// Status is a snapshot of host/Asterisk state for the dashboard.
type Status struct {
	AsteriskRunning bool   `json:"asterisk_running"`
	Uptime          string `json:"uptime"`
	Hostname        string `json:"hostname"`
	Error           string `json:"error,omitempty"`
}

// Snapshot gathers a Status, best-effort: individual command failures are
// recorded in Error but do not abort the rest of the snapshot.
func Snapshot(ctx context.Context, asteriskBin string) Status {
	var s Status
	var errs []string

	s.AsteriskRunning = AsteriskRunning(ctx, asteriskBin)

	if up, err := Uptime(ctx); err != nil {
		errs = append(errs, "uptime: "+err.Error())
	} else {
		s.Uptime = up
	}

	if hn, err := Hostname(ctx); err != nil {
		errs = append(errs, "hostname: "+err.Error())
	} else {
		s.Hostname = hn
	}

	if len(errs) > 0 {
		s.Error = strings.Join(errs, "; ")
	}
	return s
}
