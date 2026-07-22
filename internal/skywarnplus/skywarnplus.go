// Package skywarnplus configures an already-installed copy of
// SkywarnPlus (https://github.com/Mason10198/SkywarnPlus) — a
// third-party, no-longer-maintained tool that announces National
// Weather Service alerts over an AllStar node. Installation itself
// happens in install.sh (an interactive, opt-in step run once on the
// Pi, not from this running app) — see that script's own SkywarnPlus
// section for exactly what it sets up and why. This package only ever
// configures a copy that's already there.
//
// Every write goes through one of SkywarnPlus's own scripts rather than
// editing its config.yaml directly: boolean feature toggles (enable,
// sayalert, sayallclear, tailmessage, courtesytone, idchange,
// alertscript) go through its own SkyControl.py, and the two things that
// script doesn't cover (the county-code list and the node-number list,
// both YAML sequences) go through deploy/sky_configure.py — a small
// companion script this app ships (install.sh copies it in alongside
// SkywarnPlus) that uses the exact same ruamel.yaml dependency
// SkywarnPlus itself already requires, so config.yaml's own extensive
// inline comments survive edits the same way they already do through
// SkyControl.py. This package only ever talks JSON to that companion
// script — it never parses YAML itself, and this app has no YAML
// dependency of its own.
package skywarnplus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// commandTimeout bounds one shelled-out call. Generous: even a cold
// Python interpreter start on a Pi is well under this.
const commandTimeout = 15 * time.Second

// IsInstalled reports whether SkywarnPlus appears to be installed at
// dir — checked by looking for its own main script, not this package's
// companion script, since a partially-set-up directory (e.g. an
// interrupted install.sh run) should read as "not installed" rather than
// as installed-but-broken.
func IsInstalled(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "SkywarnPlus.py"))
	return err == nil
}

// Status is SkywarnPlus's current configuration, as reported by
// sky_configure.py status.
type Status struct {
	Enable      bool
	SayAlert    bool
	SayAllClear bool
	Tailmessage bool
	CountyCodes []string
	Nodes       []string
}

// runPython runs `python3 <dir>/<script> <args...>`, matching this
// app's established exec.CommandContext + context.WithTimeout +
// stderr-capture + wrapped-error pattern (see internal/system) — the
// tool's own exit code and stderr are the source of truth for whether
// it worked, not an assumption.
func runPython(ctx context.Context, dir, script string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	fullArgs := append([]string{filepath.Join(dir, script)}, args...)
	cmd := exec.CommandContext(ctx, "python3", fullArgs...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("python3 %s: %w: %s", script, err, stderr.String())
	}
	return out.String(), nil
}

// GetStatus reads SkywarnPlus's current configuration.
func GetStatus(ctx context.Context, dir string) (Status, error) {
	out, err := runPython(ctx, dir, "sky_configure.py", "status")
	if err != nil {
		return Status{}, err
	}
	var raw struct {
		Enable      bool     `json:"enable"`
		SayAlert    bool     `json:"sayalert"`
		SayAllClear bool     `json:"sayallclear"`
		Tailmessage bool     `json:"tailmessage"`
		CountyCodes []string `json:"countycodes"`
		Nodes       []string `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return Status{}, fmt.Errorf("parse sky_configure.py status output: %w", err)
	}
	return Status{
		Enable:      raw.Enable,
		SayAlert:    raw.SayAlert,
		SayAllClear: raw.SayAllClear,
		Tailmessage: raw.Tailmessage,
		CountyCodes: raw.CountyCodes,
		Nodes:       raw.Nodes,
	}, nil
}

// SetCounties replaces the full county-code list.
func SetCounties(ctx context.Context, dir string, codes []string) (string, error) {
	return runPython(ctx, dir, "sky_configure.py", "set-counties", strings.Join(codes, ","))
}

// AddNode registers node with SkywarnPlus (a no-op if it's already
// registered — sky_configure.py's add-node is idempotent).
func AddNode(ctx context.Context, dir, node string) (string, error) {
	return runPython(ctx, dir, "sky_configure.py", "add-node", node)
}

// validToggleKeys mirrors SkyControl.py's own VALID_KEYS exactly (minus
// changect/changeid, which take "wx"/"normal" rather than a boolean and
// aren't exposed by this app) — rejecting anything else here means a
// typo can't silently become a confusing error from the Python side.
var validToggleKeys = map[string]bool{
	"enable":       true,
	"sayalert":     true,
	"sayallclear":  true,
	"tailmessage":  true,
	"courtesytone": true,
	"idchange":     true,
	"alertscript":  true,
}

// SetToggle flips one of SkywarnPlus's own boolean features by shelling
// out to its own SkyControl.py directly, reusing its already-safe
// ruamel.yaml editing rather than reimplementing it.
func SetToggle(ctx context.Context, dir, key string, value bool) (string, error) {
	if !validToggleKeys[key] {
		return "", fmt.Errorf("skywarnplus: unrecognized toggle %q", key)
	}
	v := "false"
	if value {
		v = "true"
	}
	return runPython(ctx, dir, "SkyControl.py", key, v)
}
