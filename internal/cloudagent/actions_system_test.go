package cloudagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hamvoipconfiggui/internal/config"
)

// fakeAsterisk writes a fake "asterisk" binary that logs every "-rx
// <cmd>" it's called with to logPath and exits 0 unconditionally --
// same shape as internal/system's own test double, reused here since
// this package needs one too and the two aren't sharable across
// packages without exporting test-only helpers.
func fakeAsterisk(t *testing.T, logPath string, _ ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "asterisk")
	script := "#!/bin/sh\necho \"$2\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake asterisk: %v", err)
	}
	return path
}

func TestActionSystemRestartAsteriskRefusedWhenDisabled(t *testing.T) {
	a := newConfigTestAgent(t)
	// AllowRemoteReboot defaults to false -- no settings saved at all.
	if _, err := a.dispatch(context.Background(), "system.restartAsterisk", nil); err == nil {
		t.Fatal("dispatch() error = nil, want refusal when AllowRemoteReboot is off")
	}
}

// TestActionSystemRebootRefusedWhenDisabled is the only test for
// actionSystemReboot's dispatch path -- unlike restartAsterisk, system.Reboot
// shells out to the real "systemctl reboot" with no injectable binary
// path, so there is no safe way to exercise its enabled/success path in
// a unit test. The capability-gate check happens before that call, so
// this still verifies the one property that matters most here: refused
// when the flag is off.
func TestActionSystemRebootRefusedWhenDisabled(t *testing.T) {
	a := newConfigTestAgent(t)
	if _, err := a.dispatch(context.Background(), "system.reboot", nil); err == nil {
		t.Fatal("dispatch() error = nil, want refusal when AllowRemoteReboot is off")
	}
}

func TestActionSystemRestartAsteriskRunsWhenEnabled(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "calls.log")
	bin := fakeAsterisk(t, logPath, "restart now")

	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store := config.NewStore(t.TempDir())
	a := New(settingsPath, store, bin)
	if err := a.Settings().Save(Settings{Enabled: true, AllowRemoteReboot: true}); err != nil {
		t.Fatal(err)
	}

	if _, err := a.dispatch(context.Background(), "system.restartAsterisk", nil); err != nil {
		t.Fatalf("dispatch() error = %v, want success when AllowRemoteReboot is on", err)
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "restart now") {
		t.Fatalf("calls = %q, want it to include \"restart now\"", calls)
	}
}
