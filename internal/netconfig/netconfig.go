// Package netconfig manages a static IP override for HamVoIP's network
// interface via dhcpcd.conf (the standard Raspbian/Raspberry Pi OS
// network configuration mechanism dhcpcd uses for static addressing).
//
// Rather than parsing dhcpcd.conf's full grammar, edits are confined to
// a clearly marked, idempotent block appended at the end of the file.
// That keeps every edit trivially reversible (delete the block to go
// back to DHCP) and guarantees the rest of a hand-tuned dhcpcd.conf is
// never touched — getting this wrong can strand the node off-network
// with no way to fix it except a console cable.
package netconfig

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	beginMarker = "# --- hamvoip-gui managed static config: begin ---"
	endMarker   = "# --- hamvoip-gui managed static config: end ---"
)

// StaticConfig is a static IPv4 override for one interface.
type StaticConfig struct {
	Interface string // e.g. "eth0"
	Address   string // CIDR, e.g. "192.168.1.10/24"
	Router    string // gateway IP
	DNS       string // space-separated nameserver IPs
}

func (c StaticConfig) block() string {
	var b strings.Builder
	b.WriteString(beginMarker)
	b.WriteString("\n")
	fmt.Fprintf(&b, "interface %s\n", c.Interface)
	fmt.Fprintf(&b, "static ip_address=%s\n", c.Address)
	if c.Router != "" {
		fmt.Fprintf(&b, "static routers=%s\n", c.Router)
	}
	if c.DNS != "" {
		fmt.Fprintf(&b, "static domain_name_servers=%s\n", c.DNS)
	}
	b.WriteString(endMarker)
	b.WriteString("\n")
	return b.String()
}

// ReadManagedBlock parses the hamvoip-gui managed block out of path, if
// present. A nil result (with no error) means no static override is
// configured, i.e. the interface uses DHCP.
func ReadManagedBlock(path string) (*StaticConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_, block, ok := extractBlock(string(data))
	if !ok {
		return nil, nil
	}

	cfg := &StaticConfig{}
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "interface "):
			cfg.Interface = strings.TrimSpace(strings.TrimPrefix(line, "interface "))
		case strings.HasPrefix(line, "static ip_address="):
			cfg.Address = strings.TrimPrefix(line, "static ip_address=")
		case strings.HasPrefix(line, "static routers="):
			cfg.Router = strings.TrimPrefix(line, "static routers=")
		case strings.HasPrefix(line, "static domain_name_servers="):
			cfg.DNS = strings.TrimPrefix(line, "static domain_name_servers=")
		}
	}
	if cfg.Interface == "" || cfg.Address == "" {
		return nil, nil
	}
	return cfg, nil
}

// WriteManagedBlock replaces (or removes, if cfg is nil) the managed
// static-config block in path, leaving everything else in the file
// untouched. It does not itself apply the change to the running
// network stack — that requires a reboot or `systemctl restart dhcpcd`,
// left to the caller so it can be done deliberately.
func WriteManagedBlock(path string, cfg *StaticConfig) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	before, _, _ := extractBlock(string(data))
	out := strings.TrimRight(before, "\n")
	if out != "" {
		out += "\n"
	}
	if cfg != nil {
		if out != "" {
			out += "\n"
		}
		out += cfg.block()
	}

	return os.WriteFile(path, []byte(out), 0644)
}

// extractBlock returns the file content with the managed block removed
// (before), the block's inner content if found (block), and whether a
// block was present (ok).
func extractBlock(content string) (before, block string, ok bool) {
	start := strings.Index(content, beginMarker)
	if start == -1 {
		return content, "", false
	}
	end := strings.Index(content[start:], endMarker)
	if end == -1 {
		return content, "", false
	}
	end += start + len(endMarker)
	// Consume a single trailing newline after the end marker, if present.
	rest := strings.TrimPrefix(content[end:], "\n")

	return content[:start] + rest, content[start:end], true
}

// Interface is a live snapshot of one network interface's IPv4 state,
// read directly from the kernel via `ip`, independent of what
// dhcpcd.conf says should happen.
type Interface struct {
	Name      string
	Addresses []string // CIDR strings, e.g. "192.168.1.10/24"
	Up        bool
}

// ListInterfaces reads current IPv4 addresses for all interfaces via
// `ip -4 -o addr show`.
func ListInterfaces(ctx context.Context) ([]Interface, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var stderr bytes.Buffer
	c := exec.CommandContext(ctx, "ip", "-4", "-o", "addr", "show")
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("ip -4 -o addr show: %w: %s", err, stderr.String())
	}

	byName := map[string]*Interface{}
	var order []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		// Format: "<idx>: <name>    inet <addr/prefix> brd <bcast> scope <scope> <name>\..."
		if len(fields) < 4 || fields[2] != "inet" {
			continue
		}
		name := strings.TrimSuffix(fields[1], ":")
		addr := fields[3]
		iface, ok := byName[name]
		if !ok {
			iface = &Interface{Name: name, Up: true}
			byName[name] = iface
			order = append(order, name)
		}
		iface.Addresses = append(iface.Addresses, addr)
	}

	out2 := make([]Interface, 0, len(order))
	for _, name := range order {
		out2 = append(out2, *byName[name])
	}
	return out2, nil
}
