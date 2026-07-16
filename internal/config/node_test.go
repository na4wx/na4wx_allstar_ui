package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testRptConf mirrors the shape of a real HamVoIP node-config.sh
// generated rpt.conf: node 2000 has an explicit [nodes] entry (someone
// set a local/private dial string on purpose), while node 2001 has none
// at all — the normal case for an AllStarLink-registered node — plus a
// handful of the other section types node-config.sh generates
// (morse/functions/macro/etc., suffixed with a node number) that must
// NOT be mistaken for node sections themselves.
const testRptConf = `[morse2000]
speed=20

[functions2000]
1=ilink,1

[macro2000]
1=*81 *80#

[nodes]
2000 = radio@127.0.0.1:4569/2000,NONE

[2000]
rxchannel = USBRADIO/usb
duplex = 1
hangtime = 5000
totime = 200000
telemetry = telemetry
functions = functions

[2001]
rxchannel = SimpleUSB/usb
duplex = 4
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
	// Must find both real node sections, including 2001 which has no
	// [nodes] entry at all, and must not pick up the decoy sections
	// (morse2000, functions2000, macro2000, nodes) that share the same
	// node-number suffix but aren't node sections themselves.
	if len(nodes) != 2 || nodes[0] != "2000" || nodes[1] != "2001" {
		t.Fatalf("ListNodes = %v, want [2000 2001]", nodes)
	}
}

func TestLoadNodeWithNoDialStringEntry(t *testing.T) {
	// Regression test: on a real HamVoIP node, [nodes] is documented (in
	// its own generated comments) as being for local-LAN-only or private
	// node aliases, not a master registry — a normal AllStarLink node has
	// no entry there at all. That must not stop it from loading/listing.
	s := newTestStore(t)
	n, err := s.LoadNode("2001")
	if err != nil {
		t.Fatalf("LoadNode: %v", err)
	}
	if n.DialString != "" {
		t.Fatalf("DialString = %q, want empty", n.DialString)
	}
	if n.RXChannel != "SimpleUSB/usb" {
		t.Fatalf("RXChannel = %q", n.RXChannel)
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
	if len(nodes) != 3 {
		t.Fatalf("ListNodes = %v, want 3 entries", nodes)
	}

	n2, err := s.LoadNode("3000")
	if err != nil {
		t.Fatalf("LoadNode 3000: %v", err)
	}
	if n2.RXChannel != "USBRADIO/usb1" {
		t.Fatalf("RXChannel = %q", n2.RXChannel)
	}
}

func TestSaveNodeWithoutDialStringIsStillListed(t *testing.T) {
	// A blank dial string is the normal case (see Node.DialString) and
	// must not stop the node from being saved, listed, or loaded, nor
	// cause SaveNode to invent a [nodes] entry that wasn't asked for.
	s := newTestStore(t)
	n := &Node{Number: "4000", RXChannel: "USBRADIO/usb2"}
	if err := s.SaveNode(n); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}
	if n.DialString != "" {
		t.Fatalf("SaveNode should not have invented a DialString, got %q", n.DialString)
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

	raw, _ := os.ReadFile(filepath.Join(s.dir, RptConfFile))
	if strings.Contains(string(raw), "4000 = ") {
		t.Fatalf("SaveNode should not have written a [nodes] entry for 4000:\n%s", raw)
	}
}

func TestSaveNodeRejectsNonNumericNumber(t *testing.T) {
	// A non-numeric number would silently vanish from ListNodes (which
	// only recognizes purely-numeric section names as nodes), so this
	// must be rejected up front rather than saved and then hidden.
	s := newTestStore(t)
	n := &Node{Number: "abc123", RXChannel: "USBRADIO/usb"}
	if err := s.SaveNode(n); err == nil {
		t.Fatalf("SaveNode with non-numeric number should have failed")
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
	if len(nodes) != 1 || nodes[0] != "2001" {
		t.Fatalf("ListNodes after delete = %v, want [2001]", nodes)
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
