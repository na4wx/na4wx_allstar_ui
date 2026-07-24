package cloudagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"hamvoipconfiggui/internal/sa818"
)

// fakeSA818Tool writes a fake 818-prog that echoes tail and exits 0 --
// mirrors internal/sa818's own fakeTool test double (not exported, so
// this is a small local copy rather than a cross-package dependency).
func fakeSA818Tool(t *testing.T, tail string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-818-prog")
	script := "#!/bin/sh\ncat > /dev/null\necho '" + tail + "'\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake tool: %v", err)
	}
	return path
}

func TestActionSA818ProgramSuccess(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	statePath := filepath.Join(t.TempDir(), "sa818-last.json")
	a := New(settingsPath, "", nil, "asterisk", nil, nil, nil, "", fakeSA818Tool(t, "OK"), statePath, "")

	params, _ := json.Marshal(sa818.Settings{TxFreqMHz: "446.1000", RxFreqMHz: "446.1000", Squelch: 5, Volume: 4})
	result, err := a.dispatch(context.Background(), "sa818.program", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	res, ok := result.(sa818ProgramResult)
	if !ok {
		t.Fatalf("result type = %T, want sa818ProgramResult", result)
	}
	if !res.OK {
		t.Errorf("res.OK = false, want true: %s", res.Output)
	}

	last, err := sa818.LoadLast(statePath)
	if err != nil {
		t.Fatalf("LoadLast() error = %v", err)
	}
	if !last.Success || last.TxFreqMHz != "446.1000" {
		t.Errorf("last applied = %+v, want a recorded successful attempt", last)
	}
}

func TestActionSA818LastNoRecordYet(t *testing.T) {
	a := New(filepath.Join(t.TempDir(), "settings.json"), "", nil, "asterisk", nil, nil, nil, "",
		"818-prog", filepath.Join(t.TempDir(), "sa818-last.json"), "")

	result, err := a.dispatch(context.Background(), "sa818.last", nil)
	if err != nil {
		t.Fatalf("dispatch error = %v, want nil (a missing state file isn't an error)", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil when nothing has been sent yet", result)
	}
}

func TestActionSA818LastAfterProgram(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "sa818-last.json")
	a := New(filepath.Join(t.TempDir(), "settings.json"), "", nil, "asterisk", nil, nil, nil, "", fakeSA818Tool(t, "OK"), statePath, "")

	params, _ := json.Marshal(sa818.Settings{TxFreqMHz: "446.1000", RxFreqMHz: "446.1000", Squelch: 5, Volume: 4})
	if _, err := a.dispatch(context.Background(), "sa818.program", params); err != nil {
		t.Fatalf("dispatch(sa818.program) error = %v", err)
	}

	result, err := a.dispatch(context.Background(), "sa818.last", nil)
	if err != nil {
		t.Fatalf("dispatch(sa818.last) error = %v", err)
	}
	last, ok := result.(*sa818.LastApplied)
	if !ok {
		t.Fatalf("result type = %T, want *sa818.LastApplied", result)
	}
	if last == nil || last.TxFreqMHz != "446.1000" || !last.Success {
		t.Errorf("last applied = %+v, want the settings just programmed", last)
	}
}

func TestActionSA818ProgramModuleRejection(t *testing.T) {
	a := New(filepath.Join(t.TempDir(), "settings.json"), "", nil, "asterisk", nil, nil, nil, "",
		fakeSA818Tool(t, "Error, invalid information"), filepath.Join(t.TempDir(), "sa818-last.json"), "")

	params, _ := json.Marshal(sa818.Settings{})
	result, err := a.dispatch(context.Background(), "sa818.program", params)
	if err != nil {
		t.Fatalf("dispatch error = %v, want nil error (the tool ran fine, the module rejected the settings)", err)
	}
	if res := result.(sa818ProgramResult); res.OK {
		t.Error("res.OK = true, want false when the module's own output reports an error")
	}
}
