package cloudagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"hamvoipconfiggui/internal/config"
)

func newConfigTestAgent(t *testing.T) *Agent {
	t.Helper()
	asteriskDir := t.TempDir()
	fixture := "[2000]\n" +
		"rxchannel = SimpleUSB/usb\n" +
		"duplex = 1\n" +
		"telemetry = telemetry2000\n"
	if err := os.WriteFile(filepath.Join(asteriskDir, config.RptConfFile), []byte(fixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	store := config.NewStore(asteriskDir)
	return newTestAgent(t, filepath.Join(t.TempDir(), "settings.json"), store, "asterisk")
}

func TestActionConfigListNodes(t *testing.T) {
	a := newConfigTestAgent(t)
	result, err := a.dispatch(context.Background(), "config.listNodes", nil)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	nodes, ok := result.([]string)
	if !ok {
		t.Fatalf("result type = %T, want []string", result)
	}
	if len(nodes) != 1 || nodes[0] != "2000" {
		t.Fatalf("nodes = %v, want [2000]", nodes)
	}
}

func TestActionConfigLoadNode(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "2000"})
	result, err := a.dispatch(context.Background(), "config.loadNode", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	node, ok := result.(*config.Node)
	if !ok {
		t.Fatalf("result type = %T, want *config.Node", result)
	}
	if node.Number != "2000" || node.RXChannel != "SimpleUSB/usb" || node.Duplex != "1" {
		t.Fatalf("node = %+v", node)
	}
}

func TestActionConfigLoadNodeUnknownNumber(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "9999"})
	if _, err := a.dispatch(context.Background(), "config.loadNode", params); err == nil {
		t.Fatal("dispatch error = nil, want an error for a node that doesn't exist")
	}
}

func TestActionConfigSaveNodeCreatesAndSyncsExtensions(t *testing.T) {
	a := newConfigTestAgent(t)
	newNode := config.Node{Number: "3000", RXChannel: "SimpleUSB/usb2", Duplex: "1"}
	params, _ := json.Marshal(newNode)

	result, err := a.dispatch(context.Background(), "config.saveNode", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	saved, ok := result.(*config.Node)
	if !ok {
		t.Fatalf("result type = %T, want *config.Node", result)
	}
	if saved.Number != "3000" || saved.RXChannel != "SimpleUSB/usb2" {
		t.Fatalf("saved = %+v", saved)
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
		t.Fatalf("ListNodes() = %v, want it to include the newly created 3000", nodes)
	}
}

func TestActionConfigSaveNodeUpdatesExisting(t *testing.T) {
	a := newConfigTestAgent(t)
	updated := config.Node{Number: "2000", RXChannel: "SimpleUSB/usb", Duplex: "3"}
	params, _ := json.Marshal(updated)

	if _, err := a.dispatch(context.Background(), "config.saveNode", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}

	node, err := a.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	if node.Duplex != "3" {
		t.Fatalf("Duplex = %q, want 3 (the update)", node.Duplex)
	}
}

func TestActionConfigSaveNodeRejectsInvalidNumber(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(config.Node{Number: "not-a-number"})
	if _, err := a.dispatch(context.Background(), "config.saveNode", params); err == nil {
		t.Fatal("dispatch error = nil, want rejection of a non-numeric node number")
	}
}

func TestActionConfigDeleteNode(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "2000"})
	if _, err := a.dispatch(context.Background(), "config.deleteNode", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	nodes, err := a.store.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("ListNodes() = %v, want empty after delete", nodes)
	}
}

func TestActionConfigSetCourtesyTones(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{
		"number":      "2000",
		"unlinkedCT":  "|t(660,0,150,2048)",
		"remoteCT":    "|t(440,0,150,2048)",
		"linkUnkeyCT": "|t(220,0,150,2048)",
	})
	if _, err := a.dispatch(context.Background(), "config.setCourtesyTones", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	node, err := a.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	if node.UnlinkedCT != "|t(660,0,150,2048)" || node.RemoteCT != "|t(440,0,150,2048)" || node.LinkUnkeyCT != "|t(220,0,150,2048)" {
		t.Fatalf("node courtesy tones = %+v", node)
	}
}

func TestActionConfigListTelemetry(t *testing.T) {
	a := newConfigTestAgent(t)
	// 2000's fixture already points Telemetry at "telemetry2000" -- set
	// one tone entry and one non-tone (sound reference) entry directly
	// via the store, matching how a real stanza mixes both kinds.
	if err := a.store.SetTelemetryEntry("telemetry2000", "ct1", "|t(660,0,150,2048)"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetTelemetryEntry("telemetry2000", "patchup", "rpt/callproceeding"); err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]string{"number": "2000"})
	result, err := a.dispatch(context.Background(), "config.listTelemetry", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	entries, ok := result.([]telemetryEntryResult)
	if !ok {
		t.Fatalf("result type = %T, want []telemetryEntryResult", result)
	}
	byKey := map[string]telemetryEntryResult{}
	for _, e := range entries {
		byKey[e.Key] = e
	}
	ct1, ok := byKey["ct1"]
	if !ok || ct1.Tone == nil || ct1.Tone.Freq1 != 660 || ct1.Tone.DurationMS != 150 {
		t.Fatalf("ct1 entry = %+v, want a parsed tone", ct1)
	}
	patchup, ok := byKey["patchup"]
	if !ok || patchup.Tone != nil || patchup.Value != "rpt/callproceeding" {
		t.Fatalf("patchup entry = %+v, want a raw sound reference with no parsed tone", patchup)
	}
}

func TestActionConfigListTelemetryDefaultsSectionWhenNodeFieldBlank(t *testing.T) {
	a := newConfigTestAgent(t)
	newNode := config.Node{Number: "4000", RXChannel: "SimpleUSB/usb3", Duplex: "1"} // Telemetry left blank
	saveParams, _ := json.Marshal(newNode)
	if _, err := a.dispatch(context.Background(), "config.saveNode", saveParams); err != nil {
		t.Fatalf("save node: %v", err)
	}
	if err := a.store.SetTelemetryEntry("telemetry", "remotetx", "|t(500,0,100,2048)"); err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]string{"number": "4000"})
	result, err := a.dispatch(context.Background(), "config.listTelemetry", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	entries := result.([]telemetryEntryResult)
	if len(entries) != 1 || entries[0].Key != "remotetx" {
		t.Fatalf("entries = %+v, want the bare \"telemetry\" section's one entry", entries)
	}
}

func TestActionConfigSetTelemetry(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "2000", "key": "ct1", "value": "|t(660,0,150,2048)"})
	if _, err := a.dispatch(context.Background(), "config.setTelemetry", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	entries, err := a.store.ListTelemetryEntries("telemetry2000")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Key == "ct1" && e.Value == "|t(660,0,150,2048)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("entries = %+v, want ct1 set on telemetry2000", entries)
	}
}

func TestActionConfigApplyStandardCommandSet(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "2000"})
	if _, err := a.dispatch(context.Background(), "config.applyStandardCommandSet", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	node, err := a.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	if node.Functions != "functions2000" || node.Macro != "macro2000" || node.Morse != "morse2000" {
		t.Fatalf("node = %+v, want the standard node-scoped section names applied", node)
	}
	macros, err := a.store.ListFunctionMacros("functions2000")
	if err != nil {
		t.Fatal(err)
	}
	if len(macros) == 0 {
		t.Fatal("expected the standard functions table to be populated")
	}
}

func TestActionConfigApplyStandardCommandSetRejectsUnknownNode(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "9999"})
	if _, err := a.dispatch(context.Background(), "config.applyStandardCommandSet", params); err == nil {
		t.Fatal("dispatch error = nil, want rejection of a node that doesn't exist")
	}
}

func TestActionConfigCloneNodeConfig(t *testing.T) {
	a := newConfigTestAgent(t)
	// Give the source node (2000) a real standard command set to clone.
	applyParams, _ := json.Marshal(map[string]string{"number": "2000"})
	if _, err := a.dispatch(context.Background(), "config.applyStandardCommandSet", applyParams); err != nil {
		t.Fatalf("apply standard to source: %v", err)
	}
	newNode := config.Node{Number: "3000", RXChannel: "SimpleUSB/usb2", Duplex: "1"}
	saveParams, _ := json.Marshal(newNode)
	if _, err := a.dispatch(context.Background(), "config.saveNode", saveParams); err != nil {
		t.Fatalf("save destination node: %v", err)
	}

	cloneParams, _ := json.Marshal(map[string]string{"srcNumber": "2000", "dstNumber": "3000"})
	if _, err := a.dispatch(context.Background(), "config.cloneNodeConfig", cloneParams); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	node, err := a.store.LoadNode("3000")
	if err != nil {
		t.Fatal(err)
	}
	if node.Functions != "functions3000" || node.Macro != "macro3000" {
		t.Fatalf("node = %+v, want cloned section names scoped to the destination node", node)
	}
	macros, err := a.store.ListFunctionMacros("functions3000")
	if err != nil {
		t.Fatal(err)
	}
	if len(macros) == 0 {
		t.Fatal("expected the cloned functions table to be populated from the source")
	}
}

func TestActionConfigCloneNodeConfigRejectsSameSourceAndDestination(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"srcNumber": "2000", "dstNumber": "2000"})
	if _, err := a.dispatch(context.Background(), "config.cloneNodeConfig", params); err == nil {
		t.Fatal("dispatch error = nil, want rejection of cloning a node onto itself")
	}
}

func TestActionConfigNormalizeNodeConfigNothingLeftToRepairOnSecondRun(t *testing.T) {
	a := newConfigTestAgent(t)
	applyParams, _ := json.Marshal(map[string]string{"number": "2000"})
	if _, err := a.dispatch(context.Background(), "config.applyStandardCommandSet", applyParams); err != nil {
		t.Fatalf("apply standard: %v", err)
	}
	// ApplyStandardCommandSet doesn't touch the scheduler field, so the
	// first normalize still has one thing to repair -- run it once to
	// reach a fully self-contained state, then assert the second run is a no-op.
	params, _ := json.Marshal(map[string]string{"number": "2000"})
	if _, err := a.dispatch(context.Background(), "config.normalizeNodeConfig", params); err != nil {
		t.Fatalf("first normalize: %v", err)
	}

	result, err := a.dispatch(context.Background(), "config.normalizeNodeConfig", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	out, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	changed, ok := out["changed"].([]string)
	if !ok {
		t.Fatalf("changed field type = %T, want []string", out["changed"])
	}
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want empty on the second run -- nothing left to repair", changed)
	}
}

func TestActionConfigNormalizeNodeConfigRepairsMismatchedSections(t *testing.T) {
	a := newConfigTestAgent(t)
	// 2000's fixture already has telemetry pointed at telemetry2000 (matches),
	// but functions/macro/morse are unset -- normalize should fix those.
	params, _ := json.Marshal(map[string]string{"number": "2000"})
	result, err := a.dispatch(context.Background(), "config.normalizeNodeConfig", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	out := result.(map[string]any)
	changed := out["changed"].([]string)
	if len(changed) == 0 {
		t.Fatal("changed = [], want at least functions/macro/morse to be reported as repaired")
	}
	node, err := a.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	if node.Functions != "functions2000" || node.Macro != "macro2000" || node.Morse != "morse2000" {
		t.Fatalf("node = %+v, want normalized section names", node)
	}
}
