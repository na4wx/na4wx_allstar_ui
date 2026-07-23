package cloudagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"hamvoipconfiggui/internal/skywarnplus"
)

func TestActionSkywarnListCountiesWorksWithoutInstall(t *testing.T) {
	a := newTestAgent(t, t.TempDir()+"/settings.json", nil, "asterisk")
	result, err := a.dispatch(context.Background(), "skywarn.listCounties", nil)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	if _, ok := result.([]skywarnplus.CountyOption); !ok {
		t.Fatalf("result type = %T, want []skywarnplus.CountyOption", result)
	}
}

func TestActionSkywarnGetStatusNotInstalledIsError(t *testing.T) {
	a := New(t.TempDir()+"/settings.json", nil, "asterisk", nil, nil, nil, t.TempDir(), "818-prog", "", "")
	if _, err := a.dispatch(context.Background(), "skywarn.getStatus", nil); err == nil {
		t.Fatal("dispatch() error = nil, want an error when SkywarnPlus.py isn't present")
	}
}

// newFakeSkywarnDir sets up a directory that reads as "installed" (see
// skywarnplus.IsInstalled) with a fake sky_configure.py that answers
// "status" with a minimal valid JSON document -- exercising the real
// python3-shelling-out path, not a mock.
func newFakeSkywarnDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SkywarnPlus.py"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	script := `import json, sys
print(json.dumps({
    "enable": True, "sayalert": True, "sayallclear": False, "tailmessage": False, "alertscript": False,
    "courtesytone_enable": False, "idchange_enable": False, "active_alert_count": 2,
    "countycodes": ["ALC001"], "nodes": ["2000"],
    "pushover": {"enable": False, "userkey": "", "apitoken": "", "debug": False},
    "skydescribe": {"apikey": "", "language": "en-us", "speed": 0, "voice": "", "maxwords": 150},
}))
`
	if err := os.WriteFile(filepath.Join(dir, "sky_configure.py"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestActionSkywarnGetStatusInstalled(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found on PATH, skipping")
	}
	dir := newFakeSkywarnDir(t)
	a := New(t.TempDir()+"/settings.json", nil, "asterisk", nil, nil, nil, dir, "818-prog", "", "")

	result, err := a.dispatch(context.Background(), "skywarn.getStatus", nil)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	status, ok := result.(skywarnplus.Status)
	if !ok {
		t.Fatalf("result type = %T, want skywarnplus.Status", result)
	}
	if !status.Enable || status.ActiveAlertCount != 2 || len(status.CountyCodes) != 1 {
		t.Errorf("status = %+v", status)
	}
}
