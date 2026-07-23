package cloudagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"hamvoipconfiggui/internal/config"
)

func newRawConfigTestAgent(t *testing.T, allowEdit bool) *Agent {
	t.Helper()
	asteriskDir := t.TempDir()
	fixture := "[2000]\n" +
		"rxchannel = SimpleUSB/usb\n" +
		"duplex = 1\n"
	if err := os.WriteFile(filepath.Join(asteriskDir, config.RptConfFile), []byte(fixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	store := config.NewStore(asteriskDir)
	a := newTestAgent(t, filepath.Join(t.TempDir(), "settings.json"), store, "asterisk")
	if err := a.Settings().Save(Settings{Enabled: true, AllowRawConfigEdit: allowEdit}); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestActionRawConfigListFilesWorksEvenWhenDisabled(t *testing.T) {
	a := newRawConfigTestAgent(t, false)
	result, err := a.dispatch(context.Background(), "rawconfig.listFiles", nil)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	files, ok := result.([]string)
	if !ok || len(files) == 0 {
		t.Fatalf("result = %v (%T), want a non-empty []string", result, result)
	}
}

func TestActionRawConfigGetFileRefusedWhenDisabled(t *testing.T) {
	a := newRawConfigTestAgent(t, false)
	params, _ := json.Marshal(map[string]string{"file": "rpt.conf"})
	if _, err := a.dispatch(context.Background(), "rawconfig.getFile", params); err == nil {
		t.Fatal("dispatch() error = nil, want refusal when AllowRawConfigEdit is off")
	}
}

func TestActionRawConfigGetFileRejectsDisallowedFile(t *testing.T) {
	a := newRawConfigTestAgent(t, true)
	params, _ := json.Marshal(map[string]string{"file": "/etc/passwd"})
	if _, err := a.dispatch(context.Background(), "rawconfig.getFile", params); err == nil {
		t.Fatal("dispatch() error = nil, want rejection of a file not on the allowlist")
	}
}

func TestActionRawConfigGetFileWhenEnabled(t *testing.T) {
	a := newRawConfigTestAgent(t, true)
	params, _ := json.Marshal(map[string]string{"file": "rpt.conf"})
	result, err := a.dispatch(context.Background(), "rawconfig.getFile", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	fileResult, ok := result.(rawConfigFileResult)
	if !ok {
		t.Fatalf("result type = %T, want rawConfigFileResult", result)
	}
	if len(fileResult.Sections) != 1 || fileResult.Sections[0].Name != "2000" {
		t.Fatalf("sections = %+v", fileResult.Sections)
	}
	if len(fileResult.Sections[0].Keys) != 2 || fileResult.Sections[0].Keys[1].Value != "1" {
		t.Fatalf("keys = %+v", fileResult.Sections[0].Keys)
	}
}

func TestActionRawConfigSetKeyRefusedWhenDisabled(t *testing.T) {
	a := newRawConfigTestAgent(t, false)
	params, _ := json.Marshal(rawConfigSetKeyParams{File: "rpt.conf", Section: "2000", Index: 1, Value: "3"})
	if _, err := a.dispatch(context.Background(), "rawconfig.setKey", params); err == nil {
		t.Fatal("dispatch() error = nil, want refusal when AllowRawConfigEdit is off")
	}
}

func TestActionRawConfigSetKeyActuallyChangesTheFile(t *testing.T) {
	a := newRawConfigTestAgent(t, true)
	params, _ := json.Marshal(rawConfigSetKeyParams{File: "rpt.conf", Section: "2000", Index: 1, Value: "3"})
	if _, err := a.dispatch(context.Background(), "rawconfig.setKey", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	node, err := a.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	if node.Duplex != "3" {
		t.Fatalf("Duplex = %q, want 3 (the raw edit)", node.Duplex)
	}
}

func TestActionRawConfigAddKeyAndAddSection(t *testing.T) {
	a := newRawConfigTestAgent(t, true)

	addKeyParams, _ := json.Marshal(rawConfigAddKeyParams{File: "rpt.conf", Section: "2000", Key: "hangtime", Value: "5000"})
	if _, err := a.dispatch(context.Background(), "rawconfig.addKey", addKeyParams); err != nil {
		t.Fatalf("addKey error = %v", err)
	}
	node, err := a.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	if node.HangTime != "5000" {
		t.Fatalf("HangTime = %q, want 5000", node.HangTime)
	}

	addSectionParams, _ := json.Marshal(rawConfigAddSectionParams{File: "rpt.conf", Section: "3000"})
	if _, err := a.dispatch(context.Background(), "rawconfig.addSection", addSectionParams); err != nil {
		t.Fatalf("addSection error = %v", err)
	}
	nodes, err := a.store.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range nodes {
		if n == "3000" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListNodes() = %v, want it to include the newly added 3000 section", nodes)
	}
}
