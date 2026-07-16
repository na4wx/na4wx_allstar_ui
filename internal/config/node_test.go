package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRptConf = `[nodes]
2000 = radio@127.0.0.1:4569/2000,NONE

[2000]
rxchannel = USBRADIO/usb
duplex = 1
hangtime = 5000
totime = 200000
telemetry = telemetry
functions = functions
`

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RptConfFile), []byte(testRptConf), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return NewStore(dir)
}

func TestListNodes(t *testing.T) {
	s := newTestStore(t)
	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0] != "2000" {
		t.Fatalf("ListNodes = %v, want [2000]", nodes)
	}
}

func TestLoadNode(t *testing.T) {
	s := newTestStore(t)
	n, err := s.LoadNode("2000")
	if err != nil {
		t.Fatalf("LoadNode: %v", err)
	}
	if n.RXChannel != "USBRADIO/usb" {
		t.Fatalf("RXChannel = %q", n.RXChannel)
	}
	if n.DialString != "radio@127.0.0.1:4569/2000,NONE" {
		t.Fatalf("DialString = %q", n.DialString)
	}
	if n.HangTime != "5000" {
		t.Fatalf("HangTime = %q", n.HangTime)
	}
}

func TestLoadNodeNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.LoadNode("9999"); err == nil {
		t.Fatalf("expected error for missing node")
	}
}

func TestSaveNodeUpdatesExisting(t *testing.T) {
	s := newTestStore(t)
	n, err := s.LoadNode("2000")
	if err != nil {
		t.Fatalf("LoadNode: %v", err)
	}
	n.HangTime = "3000"
	n.AltHangTime = "1000" // was previously unset
	if err := s.SaveNode(n); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}

	n2, err := s.LoadNode("2000")
	if err != nil {
		t.Fatalf("LoadNode after save: %v", err)
	}
	if n2.HangTime != "3000" {
		t.Fatalf("HangTime after save = %q", n2.HangTime)
	}
	if n2.AltHangTime != "1000" {
		t.Fatalf("AltHangTime after save = %q", n2.AltHangTime)
	}
	// Untouched fields must survive.
	if n2.RXChannel != "USBRADIO/usb" {
		t.Fatalf("RXChannel after save = %q", n2.RXChannel)
	}

	raw, _ := os.ReadFile(filepath.Join(s.dir, RptConfFile))
	if !strings.Contains(string(raw), "[nodes]") {
		t.Fatalf("file structure corrupted:\n%s", raw)
	}
}

func TestSaveNodeCreatesNew(t *testing.T) {
	s := newTestStore(t)
	n := &Node{
		Number:     "3000",
		DialString: "radio@127.0.0.1:4569/3000,NONE",
		RXChannel:  "USBRADIO/usb1",
		Duplex:     "1",
	}
	if err := s.SaveNode(n); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}

	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("ListNodes = %v, want 2 entries", nodes)
	}

	n2, err := s.LoadNode("3000")
	if err != nil {
		t.Fatalf("LoadNode 3000: %v", err)
	}
	if n2.RXChannel != "USBRADIO/usb1" {
		t.Fatalf("RXChannel = %q", n2.RXChannel)
	}
}

func TestSaveNodeDefaultsBlankDialString(t *testing.T) {
	// Regression test: a node saved with no dial string used to be
	// silently left out of [nodes] entirely, making it invisible to
	// ListNodes (and, since [nodes] is what the dialplan actually uses
	// to invoke a node, likely non-functional in Asterisk too) despite
	// its own [<number>] section existing.
	s := newTestStore(t)
	n := &Node{Number: "4000", RXChannel: "USBRADIO/usb2"}
	if err := s.SaveNode(n); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}
	if n.DialString == "" {
		t.Fatalf("SaveNode should have filled in n.DialString")
	}

	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	found := false
	for _, num := range nodes {
		if num == "4000" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListNodes = %v, want it to include 4000", nodes)
	}

	n2, err := s.LoadNode("4000")
	if err != nil {
		t.Fatalf("LoadNode 4000: %v", err)
	}
	if n2.DialString == "" {
		t.Fatalf("LoadNode 4000 DialString is still empty on disk")
	}
}

func TestDeleteNode(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteNode("2000"); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("ListNodes after delete = %v, want empty", nodes)
	}
	if _, err := s.LoadNode("2000"); err == nil {
		t.Fatalf("expected LoadNode to fail after delete")
	}
}

func TestClearingFieldRemovesKey(t *testing.T) {
	s := newTestStore(t)
	n, err := s.LoadNode("2000")
	if err != nil {
		t.Fatalf("LoadNode: %v", err)
	}
	n.Telemetry = ""
	if err := s.SaveNode(n); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(s.dir, RptConfFile))
	if strings.Contains(string(raw), "telemetry") {
		t.Fatalf("telemetry key should have been removed:\n%s", raw)
	}
}
