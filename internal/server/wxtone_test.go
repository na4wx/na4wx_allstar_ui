package server

import (
	"os"
	"path/filepath"
	"testing"

	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/sounds"
	"hamvoipconfiggui/internal/wxtone"
)

// newWXToneTestServer builds a minimal *Server (bypassing server.New, which
// needs real embedded templates) with just the fields resolveCTDestPath/
// applyWXTone actually touch: a config.Store backed by a real rpt.conf
// fixture, and a sounds.Store backed by a real custom sound directory.
func newWXToneTestServer(t *testing.T, ct1Value string) (*Server, string) {
	t.Helper()
	asteriskDir := t.TempDir()
	customDir := t.TempDir()

	fixture := "[telemetry2000]\n" +
		"ct1=" + ct1Value + "\n" +
		"cmdmode=|t(1000,0,100,2048)\n" +
		"\n" +
		"[2000]\n" +
		"rxchannel = SimpleUSB/usb\n" +
		"duplex = 4\n" +
		"telemetry = telemetry2000\n"
	if err := os.WriteFile(filepath.Join(asteriskDir, config.RptConfFile), []byte(fixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	store := config.NewStore(asteriskDir)
	soundsStore := sounds.New(customDir, filepath.Join(t.TempDir(), "stock-does-not-exist"), "sox")
	return &Server{store: store, sounds: soundsStore, wxTones: wxtone.New(filepath.Join(t.TempDir(), "wx-tones.json"))}, customDir
}

func writeCustomSound(t *testing.T, customDir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(customDir, name+".ulaw"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveCTDestPathSoundMode(t *testing.T) {
	// customDir isn't known until newWXToneTestServer creates it, so
	// build the fixture with a placeholder ct1 first, then rewrite it
	// once we know the real path -- simplest way to keep the fixture and
	// the directory in sync without a chicken-and-egg dependency.
	s, customDir := newWXToneTestServer(t, "PLACEHOLDER")
	writeCustomSound(t, customDir, "ct1", "dest-bytes")
	ct1Ref := filepath.Join(customDir, "ct1")
	rewriteFixtureCT1(t, s, ct1Ref)

	node, err := s.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.resolveCTDestPath(node, "ct1")
	if err != nil {
		t.Fatalf("resolveCTDestPath() error = %v", err)
	}
	if path != filepath.Join(customDir, "ct1.ulaw") {
		t.Errorf("path = %q, want %q", path, filepath.Join(customDir, "ct1.ulaw"))
	}
}

// rewriteFixtureCT1 patches ct1's value in the already-written rpt.conf
// fixture to ct1Ref, using the store's own telemetry setter rather than
// re-writing the file by hand, so this exercises the same code path a
// real save would.
func rewriteFixtureCT1(t *testing.T, s *Server, ct1Ref string) {
	t.Helper()
	if err := s.store.SetTelemetryEntry("telemetry2000", "ct1", ct1Ref); err != nil {
		t.Fatal(err)
	}
}

func TestResolveCTDestPathRejectsToneValue(t *testing.T) {
	s, _ := newWXToneTestServer(t, "unused")
	node, err := s.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveCTDestPath(node, "cmdmode"); err == nil {
		t.Fatal("resolveCTDestPath() error = nil, want rejection of a tone-generator value")
	}
}

func TestResolveCTDestPathRejectsUnrelatedPath(t *testing.T) {
	s, _ := newWXToneTestServer(t, "/etc/asterisk/local/not-a-real-custom-sound")
	node, err := s.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveCTDestPath(node, "ct1"); err == nil {
		t.Fatal("resolveCTDestPath() error = nil, want rejection of a path that isn't one of this app's own custom sounds")
	}
}

func TestApplyWXToneSwapsFileContent(t *testing.T) {
	s, customDir := newWXToneTestServer(t, "unused")
	writeCustomSound(t, customDir, "ct1", "dest-original")
	writeCustomSound(t, customDir, "normal-tone", "NORMAL")
	writeCustomSound(t, customDir, "wx-tone", "WX")
	ct1Ref := filepath.Join(customDir, "ct1")
	rewriteFixtureCT1(t, s, ct1Ref)

	entry := wxtone.Entry{Node: "2000", CTKey: "ct1", NormalSound: "normal-tone", WXSound: "wx-tone"}

	if err := s.applyWXTone(entry, wxtone.ModeWX); err != nil {
		t.Fatalf("applyWXTone(WX) error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(customDir, "ct1.ulaw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "WX" {
		t.Errorf("after WX swap, dest content = %q, want %q", got, "WX")
	}

	if err := s.applyWXTone(entry, wxtone.ModeNormal); err != nil {
		t.Fatalf("applyWXTone(Normal) error = %v", err)
	}
	got, err = os.ReadFile(filepath.Join(customDir, "ct1.ulaw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NORMAL" {
		t.Errorf("after normal swap, dest content = %q, want %q", got, "NORMAL")
	}
}

func TestApplyWXToneNoopWhenSourceIsDestination(t *testing.T) {
	s, customDir := newWXToneTestServer(t, "unused")
	// ct1's own file IS the "normal" source -- the expected steady state
	// before any alert has ever fired.
	writeCustomSound(t, customDir, "ct1", "steady-state")
	ct1Ref := filepath.Join(customDir, "ct1")
	rewriteFixtureCT1(t, s, ct1Ref)

	entry := wxtone.Entry{Node: "2000", CTKey: "ct1", NormalSound: "ct1", WXSound: "does-not-matter"}
	if err := s.applyWXTone(entry, wxtone.ModeNormal); err != nil {
		t.Fatalf("applyWXTone() error = %v, want no error when source equals destination", err)
	}
	got, err := os.ReadFile(filepath.Join(customDir, "ct1.ulaw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "steady-state" {
		t.Errorf("content changed unexpectedly: %q", got)
	}
}
