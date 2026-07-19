package server

import (
	"testing"

	"hamvoipconfiggui/internal/config"
)

// TestShariPresetFixesCarrierFrom pins down the specific field that
// causes a freshly created SHARI node to transmit continuously. The
// generic placeholder uses carrierfrom=usb, but a SHARI's carrier-detect
// line is inverted, so with "usb" the node reads "receiving" permanently
// and holds the transmitter keyed. If this assertion ever fails, newly
// created SHARI nodes are jamming their frequency.
func TestShariPresetFixesCarrierFrom(t *testing.T) {
	generic := placeholderRadioDevice("Device")
	if generic.CarrierFrom != "usb" {
		t.Fatalf("precondition: generic default carrierfrom = %q, want usb", generic.CarrierFrom)
	}

	shari := placeholderRadioDevice("Device")
	config.ApplyShariUSBPreset(shari)
	if shari.CarrierFrom != "usbinvert" {
		t.Errorf("SHARI carrierfrom = %q, want usbinvert — a SHARI node with %q transmits continuously", shari.CarrierFrom, generic.CarrierFrom)
	}
}

// TestShariPresetKeepsDeviceName guards against the preset clobbering
// the device name, which would leave the node's rxchannel pointing at a
// stanza that doesn't exist — the failure mode that took a real node
// offline during this app's development.
func TestShariPresetKeepsDeviceName(t *testing.T) {
	d := placeholderRadioDevice("Device")
	config.ApplyShariUSBPreset(d)
	if d.Name != "Device" {
		t.Errorf("Name = %q, want Device", d.Name)
	}
	if d.RXMixerSet != "500" || d.TXMixerSet != "500" {
		t.Errorf("preset should leave audio levels alone, got rx=%q tx=%q", d.RXMixerSet, d.TXMixerSet)
	}
}
