package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// migratedRptConf reproduces the exact real-world breakage this repair
// exists for: node 52829's own section was created by renaming the
// shipped template's [1998] header, so it still points at that template's
// functions1998/morse1998/etc. sections instead of its own. macro is
// left blank, exercising the bare-fallback path.
const migratedRptConf = `[52829]
rxchannel = SimpleUSB/Device
duplex = 0
functions = functions1998
telemetry = telemetry1998
morse = morse1998

[functions1998]
1=ilink,1
2=ilink,2
82=cmd,/usr/local/sbin/say24time.pl 1998

[telemetry1998]
ct1=|t(350,0,100,2048)

[morse1998]
speed=20
frequency=800
`

func newMigratedStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RptConfFile), []byte(migratedRptConf), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return NewStore(dir)
}

func TestNormalizeRepointsAndCopies(t *testing.T) {
	s := newMigratedStore(t)
	changed, err := s.NormalizeNodeConfig("52829")
	if err != nil {
		t.Fatalf("NormalizeNodeConfig: %v", err)
	}
	// functions, telemetry and morse all pointed at *1998 sections;
	// macro was blank (bare "macro" fallback). All four differ from the
	// desired <prefix>52829 name, so all four are repaired.
	if len(changed) != 4 {
		t.Fatalf("changed = %v, want all four fields", changed)
	}

	node, err := s.LoadNode("52829")
	if err != nil {
		t.Fatal(err)
	}
	if node.Functions != "functions52829" {
		t.Errorf("functions = %q, want functions52829", node.Functions)
	}
	if node.Telemetry != "telemetry52829" {
		t.Errorf("telemetry = %q, want telemetry52829", node.Telemetry)
	}
	if node.Morse != "morse52829" {
		t.Errorf("morse = %q, want morse52829", node.Morse)
	}
	if node.Macro != "macro52829" {
		t.Errorf("macro = %q, want macro52829", node.Macro)
	}

	raw, err := os.ReadFile(filepath.Join(s.dir, RptConfFile))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)

	// The command set must have been carried across verbatim into the
	// correctly-named section — losing it would silently disable DTMF.
	if !strings.Contains(got, "[functions52829]") {
		t.Error("functions52829 section was not created")
	}
	if !strings.Contains(got, "1=ilink,1") || !strings.Contains(got, "2=ilink,2") {
		t.Error("command entries were not copied into the new section")
	}
	// A blank field's bare fallback yields an empty but correctly-named
	// section (there was no [macro] to copy from).
	if !strings.Contains(got, "[macro52829]") {
		t.Error("macro52829 section was not created for the blank field")
	}
	// The old sections are deliberately left in place.
	if !strings.Contains(got, "[functions1998]") {
		t.Error("source section functions1998 should be left untouched")
	}
}

// TestNormalizeDoesNotRewriteEmbeddedNumbers pins the deliberate
// conservatism: a node number embedded as a literal command argument is
// copied as-is, not substituted, since guessing which numbers are safe
// to rewrite could corrupt a command referring to a different node.
func TestNormalizeDoesNotRewriteEmbeddedNumbers(t *testing.T) {
	s := newMigratedStore(t)
	if _, err := s.NormalizeNodeConfig("52829"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(s.dir, RptConfFile))
	if !strings.Contains(string(raw), "say24time.pl 1998") {
		t.Error("embedded node number was rewritten; it should be copied verbatim")
	}
}

// TestNormalizeIdempotent is the safety property: running repair on an
// already-correct node changes nothing and reports no changes.
func TestNormalizeIdempotent(t *testing.T) {
	s := newMigratedStore(t)
	if _, err := s.NormalizeNodeConfig("52829"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(s.dir, RptConfFile))

	changed, err := s.NormalizeNodeConfig("52829")
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Errorf("second run changed %v, want nothing", changed)
	}
	after, _ := os.ReadFile(filepath.Join(s.dir, RptConfFile))
	if string(before) != string(after) {
		t.Error("second normalize modified the file despite reporting no changes")
	}
}

func TestNormalizeMissingNode(t *testing.T) {
	s := newMigratedStore(t)
	if _, err := s.NormalizeNodeConfig("99999"); err == nil {
		t.Error("expected an error for a node that doesn't exist")
	}
}
