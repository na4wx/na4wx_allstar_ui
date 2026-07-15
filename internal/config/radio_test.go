package config

import (
	"os"
	"path/filepath"
	"testing"
)

const testUsbradioConf = `[usb]
carrierfrom = usbinvert
ctcssfrom = no
rxdemod = speaker
txprelim = yes
txmixa = voice
invertptt = 0
rxmixerset = 500
txmixerset = 500
hdwtype = 0
`

func newRadioTestStore(t *testing.T, filename, content string) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return NewStore(dir)
}

func TestListRadioDevices(t *testing.T) {
	s := newRadioTestStore(t, UsbradioConfFile, testUsbradioConf)
	devices, err := s.ListRadioDevices(UsbradioConfFile)
	if err != nil {
		t.Fatalf("ListRadioDevices: %v", err)
	}
	if len(devices) != 1 || devices[0] != "usb" {
		t.Fatalf("ListRadioDevices = %v, want [usb]", devices)
	}
}

// Regression test for a real deployment finding: a stock HamVoIP node's
// simpleusb.conf has an empty [general] defaults section (standard
// Asterisk driver-config convention) alongside real device sections —
// [general] must not show up as a fake "device".
const testSimpleusbConfWithGeneral = `[general]

[usb]
carrierfrom = usbinvert
ctcssfrom = no
invertptt = 0
`

func TestListRadioDevicesExcludesGeneral(t *testing.T) {
	s := newRadioTestStore(t, SimpleusbConfFile, testSimpleusbConfWithGeneral)
	devices, err := s.ListRadioDevices(SimpleusbConfFile)
	if err != nil {
		t.Fatalf("ListRadioDevices: %v", err)
	}
	if len(devices) != 1 || devices[0] != "usb" {
		t.Fatalf("ListRadioDevices = %v, want [usb] (general excluded)", devices)
	}
}

func TestLoadRadioDeviceRejectsGeneral(t *testing.T) {
	s := newRadioTestStore(t, SimpleusbConfFile, testSimpleusbConfWithGeneral)
	if _, err := s.LoadRadioDevice(SimpleusbConfFile, "general"); err == nil {
		t.Fatalf("expected error loading [general] as a device")
	}
}

func TestSaveRadioDeviceRejectsGeneral(t *testing.T) {
	s := newRadioTestStore(t, SimpleusbConfFile, testSimpleusbConfWithGeneral)
	err := s.SaveRadioDevice(SimpleusbConfFile, &RadioDevice{Name: "general", CarrierFrom: "usb"})
	if err == nil {
		t.Fatalf("expected error saving a device named \"general\"")
	}
}

func TestListRadioDevicesRejectsWrongFile(t *testing.T) {
	s := newRadioTestStore(t, UsbradioConfFile, testUsbradioConf)
	if _, err := s.ListRadioDevices("rpt.conf"); err == nil {
		t.Fatalf("expected error for non-radio file")
	}
}

func TestLoadRadioDevice(t *testing.T) {
	s := newRadioTestStore(t, UsbradioConfFile, testUsbradioConf)
	d, err := s.LoadRadioDevice(UsbradioConfFile, "usb")
	if err != nil {
		t.Fatalf("LoadRadioDevice: %v", err)
	}
	if d.CarrierFrom != "usbinvert" {
		t.Fatalf("CarrierFrom = %q", d.CarrierFrom)
	}
	if d.RXMixerSet != "500" {
		t.Fatalf("RXMixerSet = %q", d.RXMixerSet)
	}
	if d.HdwType != "0" {
		t.Fatalf("HdwType = %q", d.HdwType)
	}
}

func TestSaveRadioDeviceUpdatesExisting(t *testing.T) {
	s := newRadioTestStore(t, UsbradioConfFile, testUsbradioConf)
	d, err := s.LoadRadioDevice(UsbradioConfFile, "usb")
	if err != nil {
		t.Fatalf("LoadRadioDevice: %v", err)
	}
	d.RXMixerSet = "700"
	d.TXCTCSSDefault = "100.0" // previously unset
	if err := s.SaveRadioDevice(UsbradioConfFile, d); err != nil {
		t.Fatalf("SaveRadioDevice: %v", err)
	}

	d2, err := s.LoadRadioDevice(UsbradioConfFile, "usb")
	if err != nil {
		t.Fatalf("LoadRadioDevice after save: %v", err)
	}
	if d2.RXMixerSet != "700" {
		t.Fatalf("RXMixerSet after save = %q", d2.RXMixerSet)
	}
	if d2.TXCTCSSDefault != "100.0" {
		t.Fatalf("TXCTCSSDefault after save = %q", d2.TXCTCSSDefault)
	}
	if d2.CarrierFrom != "usbinvert" {
		t.Fatalf("untouched CarrierFrom = %q", d2.CarrierFrom)
	}
}

func TestSaveRadioDeviceCreatesNew(t *testing.T) {
	s := newRadioTestStore(t, UsbradioConfFile, testUsbradioConf)
	d := &RadioDevice{Name: "usb1", CarrierFrom: "dsp", RXMixerSet: "600"}
	if err := s.SaveRadioDevice(UsbradioConfFile, d); err != nil {
		t.Fatalf("SaveRadioDevice: %v", err)
	}
	devices, err := s.ListRadioDevices(UsbradioConfFile)
	if err != nil {
		t.Fatalf("ListRadioDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("ListRadioDevices = %v, want 2", devices)
	}
}

func TestDeleteRadioDevice(t *testing.T) {
	s := newRadioTestStore(t, UsbradioConfFile, testUsbradioConf)
	if err := s.DeleteRadioDevice(UsbradioConfFile, "usb"); err != nil {
		t.Fatalf("DeleteRadioDevice: %v", err)
	}
	devices, err := s.ListRadioDevices(UsbradioConfFile)
	if err != nil {
		t.Fatalf("ListRadioDevices: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("ListRadioDevices after delete = %v", devices)
	}
}

func TestSimpleusbConfSharesFieldMapping(t *testing.T) {
	s := newRadioTestStore(t, SimpleusbConfFile, testUsbradioConf)
	d, err := s.LoadRadioDevice(SimpleusbConfFile, "usb")
	if err != nil {
		t.Fatalf("LoadRadioDevice: %v", err)
	}
	if d.CarrierFrom != "usbinvert" {
		t.Fatalf("CarrierFrom = %q", d.CarrierFrom)
	}
}
