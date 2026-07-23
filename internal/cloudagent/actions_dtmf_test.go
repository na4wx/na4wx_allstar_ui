package cloudagent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeAsteriskBinTool writes a fake "asterisk" executable that just
// echoes its own "-rx" argument, matching AsteriskRX's own "asterisk
// -rx <cmd>" invocation shape closely enough to assert what command
// string this action actually built.
func fakeAsteriskBinTool(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake asterisk fixture is a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "asterisk")
	script := "#!/bin/sh\necho \"got: $2\"\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake asterisk: %v", err)
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no /bin/sh available")
	}
	return path
}

func TestActionSystemDTMF(t *testing.T) {
	a := newConfigTestAgent(t)
	a.asteriskBin = fakeAsteriskBinTool(t)

	params, _ := json.Marshal(map[string]string{"number": "2000", "digits": "1"})
	result, err := a.dispatch(context.Background(), "system.dtmf", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	out, ok := result.(map[string]string)
	if !ok {
		t.Fatalf("result type = %T, want map[string]string", result)
	}
	if out["output"] != "got: rpt fun 2000 1\n" {
		t.Fatalf("output = %q, want the built \"rpt fun 2000 1\" command echoed back", out["output"])
	}
}

func TestActionSystemDTMFRejectsInvalidDigits(t *testing.T) {
	a := newConfigTestAgent(t)
	a.asteriskBin = fakeAsteriskBinTool(t)

	params, _ := json.Marshal(map[string]string{"number": "2000", "digits": "1; rm -rf /"})
	if _, err := a.dispatch(context.Background(), "system.dtmf", params); err == nil {
		t.Fatal("dispatch error = nil, want rejection of digits containing disallowed characters")
	}
}

func TestActionSystemDTMFAllowsExtendedDigits(t *testing.T) {
	a := newConfigTestAgent(t)
	a.asteriskBin = fakeAsteriskBinTool(t)

	for _, digits := range []string{"123", "*1", "#9", "ABCD", "abcd"} {
		params, _ := json.Marshal(map[string]string{"number": "2000", "digits": digits})
		if _, err := a.dispatch(context.Background(), "system.dtmf", params); err != nil {
			t.Fatalf("digits = %q: dispatch error = %v, want it accepted", digits, err)
		}
	}
}
