package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression test for a real deployment bug: on a stock HamVoIP node,
// usbradio.conf and simpleusb.conf are mutually exclusive — only one
// exists, depending on which driver the node uses. Visiting the Radio
// page for the one that doesn't exist must not be a hard error.
func TestLoadMissingFileIsEmptyNotError(t *testing.T) {
	dir := t.TempDir() // deliberately contains no config files at all
	s := NewStore(dir)

	f, err := s.load("usbradio.conf")
	if err != nil {
		t.Fatalf("load of missing file should not error, got: %v", err)
	}
	if f == nil {
		t.Fatalf("load of missing file returned nil *File")
	}
	if len(f.Sections()) != 0 {
		t.Fatalf("expected no sections from a missing file, got %v", f.Sections())
	}
}

func TestListRadioDevicesOnCompletelyMissingFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	devices, err := s.ListRadioDevices(UsbradioConfFile)
	if err != nil {
		t.Fatalf("ListRadioDevices on missing file should not error, got: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected no devices, got %v", devices)
	}
}

func TestSaveRadioDeviceCreatesCompletelyMissingFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	d := &RadioDevice{Name: "usb", CarrierFrom: "usbinvert", RXMixerSet: "500"}
	if err := s.SaveRadioDevice(UsbradioConfFile, d); err != nil {
		t.Fatalf("SaveRadioDevice: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, UsbradioConfFile)); err != nil {
		t.Fatalf("expected usbradio.conf to be created: %v", err)
	}
	got, err := s.LoadRadioDevice(UsbradioConfFile, "usb")
	if err != nil {
		t.Fatalf("LoadRadioDevice: %v", err)
	}
	if got.CarrierFrom != "usbinvert" {
		t.Fatalf("got = %+v", got)
	}
}

func TestListNodesOnMissingRptConf(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	nodes, err := s.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes on missing rpt.conf should not error, got: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected no nodes, got %v", nodes)
	}
	// Non-nil even with zero nodes -- a nil slice marshals to JSON
	// null, and the cloud relay's config.listNodes action sends this
	// straight to the browser as JSON.
	if nodes == nil {
		t.Error("ListNodes() = nil, want a non-nil empty slice")
	}
}

// TestChangeHookFiresOnSave covers the mechanism the server package uses
// to show its "Asterisk must be restarted" bar: every write to any
// Asterisk config file must call the installed hook exactly once, naming
// the file that changed, so the server never has to remember to flag it
// per handler.
func TestChangeHookFiresOnSave(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	var got []string
	s.SetChangeHook(func(file string) { got = append(got, file) })

	if err := s.SetTelemetryEntry("telemetry2000", "ct1", "|t(660,0,150,2048)"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != RptConfFile {
		t.Fatalf("change hook calls = %v, want one call with %q", got, RptConfFile)
	}
}

func TestChangeHookNotRequired(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	// No SetChangeHook call at all -- must not panic.
	if err := s.SetTelemetryEntry("telemetry2000", "ct1", "|t(660,0,150,2048)"); err != nil {
		t.Fatal(err)
	}
}
