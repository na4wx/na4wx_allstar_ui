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
	return New(filepath.Join(t.TempDir(), "settings.json"), store, "asterisk")
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
