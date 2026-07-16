package server

import "testing"

func TestParseRadioChannel(t *testing.T) {
	cases := []struct {
		in       string
		wantFile string
		wantName string
		wantOK   bool
	}{
		{"SimpleUSB/usb", "simpleusb.conf", "usb", true},
		{"USBRADIO/usb", "usbradio.conf", "usb", true},
		{"USBRADIO/usb1", "usbradio.conf", "usb1", true},
		{"Voter/125", "", "", false},
		{"garbage", "", "", false},
		{"", "", "", false},
		{"SimpleUSB/", "", "", false},
	}
	for _, c := range cases {
		ref, ok := parseRadioChannel(c.in)
		if ok != c.wantOK {
			t.Errorf("parseRadioChannel(%q) ok = %v, want %v", c.in, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if ref.File != c.wantFile || ref.Name != c.wantName {
			t.Errorf("parseRadioChannel(%q) = %+v, want {%s %s}", c.in, ref, c.wantFile, c.wantName)
		}
	}
}
