package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// fakeAsterisk writes a fake "asterisk" binary to a temp dir that logs
// every "-rx <cmd>" it's called with (one per line) to logPath and
// exits 0 unconditionally -- mirrors internal/system's own fakeAsterisk
// test double, simplified since these tests only need to observe that
// AsteriskReloadRpt's plain "rpt reload" form was actually invoked, not
// exercise its own fallback logic (already covered in internal/system).
func fakeAsterisk(t *testing.T, logPath string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "asterisk")
	script := "#!/bin/sh\necho \"$2\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake asterisk: %v", err)
	}
	return path
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

	entry := wxtone.Entry{
		Node: "2000", CTKey: "ct1",
		NormalType: wxtone.TypeSound, NormalSound: "normal-tone",
		WXType: wxtone.TypeSound, WXSound: "wx-tone",
	}

	if err := s.applyWXTone(context.Background(), entry, wxtone.ModeWX); err != nil {
		t.Fatalf("applyWXTone(WX) error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(customDir, "ct1.ulaw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "WX" {
		t.Errorf("after WX swap, dest content = %q, want %q", got, "WX")
	}

	if err := s.applyWXTone(context.Background(), entry, wxtone.ModeNormal); err != nil {
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

	entry := wxtone.Entry{
		Node: "2000", CTKey: "ct1",
		NormalType: wxtone.TypeSound, NormalSound: "ct1",
		WXType: wxtone.TypeSound, WXSound: "does-not-matter",
	}
	if err := s.applyWXTone(context.Background(), entry, wxtone.ModeNormal); err != nil {
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

// TestPopulateNodeWXTonesOffersEveryCTKey confirms the picker no longer
// filters down to sound-mode-only keys (see this package's git history
// for the short-lived SoundCTKeys restriction) -- once a tone-type
// state is possible, any existing ctX key is a valid starting point,
// since its value is only ever read to build the friendly "current
// value" display, never required to already be a sound file.
func TestPopulateNodeWXTonesOffersEveryCTKey(t *testing.T) {
	s, _ := newWXToneTestServer(t, "unused")
	node, err := s.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	data := nodeFormData{Node: node, SkywarnInstalled: true, CTKeys: []string{"ct1", "cmdmode"}}
	s.populateNodeWXTones(&data)

	if len(data.CTKeys) != 2 {
		t.Fatalf("CTKeys = %v, want unchanged (populateNodeWXTones must not filter it)", data.CTKeys)
	}
}

// TestApplyWXToneToneTypeRewritesRptConfAndReloads covers a pure
// tone-type entry: applying WX must rewrite ct1's rpt.conf value to the
// WX tone's raw "|t(...)" string and call AsteriskReloadRpt's plain
// "rpt reload" form -- never touching any sound file at all.
func TestApplyWXToneToneTypeRewritesRptConfAndReloads(t *testing.T) {
	s, _ := newWXToneTestServer(t, "|t(660,0,150,2048)")
	logPath := filepath.Join(t.TempDir(), "calls.log")
	s.asteriskBin = fakeAsterisk(t, logPath)

	entry := wxtone.Entry{
		Node: "2000", CTKey: "ct1",
		NormalType: wxtone.TypeTone, NormalTone: "|t(660,0,150,2048)",
		WXType: wxtone.TypeTone, WXTone: "|t(650,0,100,2048)(770,0,100,2048)",
	}
	if err := s.applyWXTone(context.Background(), entry, wxtone.ModeWX); err != nil {
		t.Fatalf("applyWXTone(WX) error = %v", err)
	}

	node, err := s.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.store.ListTelemetryEntries(node.Telemetry)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, te := range entries {
		if te.Key == "ct1" {
			got = te.Value
		}
	}
	if got != "|t(650,0,100,2048)(770,0,100,2048)" {
		t.Fatalf("ct1 = %q, want the WX tone value", got)
	}

	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected AsteriskReloadRpt to have been called: %v", err)
	}
	if got := strings.TrimSpace(string(calls)); got != "rpt reload" {
		t.Fatalf("calls = %q, want \"rpt reload\"", got)
	}
}

// TestApplyWXToneMixedTypeSwitchesBetweenSoundAndTone covers an entry
// where Normal is a sound file and WX is a tone -- the case that broke
// the original resolveCTDestPath-based sound swap, since ct1's rpt.conf
// value stops being a fixed sound-file reference once a tone apply has
// run. Applying WX must write the tone's raw value; applying Normal
// afterward must write the sound file's own rpt.conf-ready reference
// (sounds.File.Ref), not attempt a byte-level file copy.
func TestApplyWXToneMixedTypeSwitchesBetweenSoundAndTone(t *testing.T) {
	s, customDir := newWXToneTestServer(t, "unused")
	writeCustomSound(t, customDir, "normal-tone", "NORMAL")
	ct1Ref := filepath.Join(customDir, "normal-tone")
	rewriteFixtureCT1(t, s, ct1Ref) // arbitrary starting value, about to be overwritten either way
	logPath := filepath.Join(t.TempDir(), "calls.log")
	s.asteriskBin = fakeAsterisk(t, logPath)

	entry := wxtone.Entry{
		Node: "2000", CTKey: "ct1",
		NormalType: wxtone.TypeSound, NormalSound: "normal-tone",
		WXType: wxtone.TypeTone, WXTone: "|t(650,0,100,2048)",
	}

	if err := s.applyWXTone(context.Background(), entry, wxtone.ModeWX); err != nil {
		t.Fatalf("applyWXTone(WX) error = %v", err)
	}
	node, err := s.store.LoadNode("2000")
	if err != nil {
		t.Fatal(err)
	}
	value := func() string {
		entries, err := s.store.ListTelemetryEntries(node.Telemetry)
		if err != nil {
			t.Fatal(err)
		}
		for _, te := range entries {
			if te.Key == "ct1" {
				return te.Value
			}
		}
		return ""
	}
	if got := value(); got != "|t(650,0,100,2048)" {
		t.Fatalf("after WX apply, ct1 = %q, want the tone value", got)
	}

	if err := s.applyWXTone(context.Background(), entry, wxtone.ModeNormal); err != nil {
		t.Fatalf("applyWXTone(Normal) error = %v", err)
	}
	if got := value(); got != ct1Ref {
		t.Fatalf("after Normal apply, ct1 = %q, want the sound file's own reference %q", got, ct1Ref)
	}

	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected AsteriskReloadRpt to have been called: %v", err)
	}
	if got := strings.TrimSpace(string(calls)); got != "rpt reload\nrpt reload" {
		t.Fatalf("calls = %q, want two reloads (one per apply)", got)
	}
}
