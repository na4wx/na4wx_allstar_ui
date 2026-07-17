package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const standardCommandsFixture = `[nodes]

[68536]
rxchannel = SimpleUSB/usb
duplex = 4
`

func newStandardCommandsTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RptConfFile), []byte(standardCommandsFixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return NewStore(dir)
}

func TestApplyStandardCommandSetGivesNodeAWorkingSet(t *testing.T) {
	s := newStandardCommandsTestStore(t)
	if err := s.ApplyStandardCommandSet("68536"); err != nil {
		t.Fatalf("ApplyStandardCommandSet: %v", err)
	}

	n, err := s.LoadNode("68536")
	if err != nil {
		t.Fatalf("LoadNode: %v", err)
	}
	if n.Functions != "functions68536" || n.Macro != "macro68536" ||
		n.Telemetry != "telemetry68536" || n.Morse != "morse68536" {
		t.Fatalf("68536 companion fields = %+v, want all *68536", n)
	}

	raw, _ := os.ReadFile(filepath.Join(s.dir, RptConfFile))
	out := string(raw)
	for _, want := range []string{
		"[functions68536]",
		"1 = ilink,1",
		"3 = ilink,3",
		"[telemetry68536]",
		"ct1 = |t(350,0,100,2048)(500,0,100,2048)(660,0,100,2048)",
		"patchdown = rpt/callterminated",
		"[morse68536]",
		"speed = 20",
		"idamplitude = 1024",
		"[macro68536]",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestApplyStandardCommandSetIsResyncable(t *testing.T) {
	s := newStandardCommandsTestStore(t)
	if err := s.ApplyStandardCommandSet("68536"); err != nil {
		t.Fatalf("first ApplyStandardCommandSet: %v", err)
	}
	if err := s.ApplyStandardCommandSet("68536"); err != nil {
		t.Fatalf("second ApplyStandardCommandSet: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(s.dir, RptConfFile))
	if strings.Count(string(raw), "[functions68536]") != 1 {
		t.Fatalf("ApplyStandardCommandSet duplicated the section:\n%s", raw)
	}
	// Newline-anchored so "71 = ilink,11" (a real, different line) isn't
	// miscounted as a second match of "1 = ilink,1".
	if strings.Count(string(raw), "\n1 = ilink,1\n") != 1 {
		t.Fatalf("re-running ApplyStandardCommandSet duplicated entries:\n%s", raw)
	}
}

func TestApplyStandardCommandSetRejectsUnknownNode(t *testing.T) {
	s := newStandardCommandsTestStore(t)
	if err := s.ApplyStandardCommandSet("99999"); err == nil {
		t.Fatalf("expected error for unknown destination node")
	}
}

func TestApplyStandardCommandSetDoesNotEmbedNodeNumberInScripts(t *testing.T) {
	// Regression guard: the real stanza this was modeled on has entries
	// like "cmd,/usr/local/sbin/saytime.pl 68536" that embed the node
	// number as a script argument. Applying this to a *different* node
	// number must never produce a stale/wrong node number in any value.
	s := newStandardCommandsTestStore(t)
	if err := s.ApplyStandardCommandSet("68536"); err != nil {
		t.Fatalf("ApplyStandardCommandSet: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(s.dir, RptConfFile))
	if strings.Contains(string(raw), "saytime") || strings.Contains(string(raw), "/usr/local/sbin/") {
		t.Fatalf("standard command set should not reference site-specific scripts:\n%s", raw)
	}
}
