package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// extensionsFixture mirrors the relevant shape of a real HamVoIP
// node-config.sh generated extensions.conf: one node (68536) already
// has entries in all three managed contexts, plus decoy exten lines in
// unrelated contexts that must never be touched.
const extensionsFixture = `[radio-secure]
exten => 68536,1,rpt,68536
exten => 1999,1,rpt,1999

[radio-secure-proxy]
exten => 68536,1,rpt,68536|X
exten => _0X.,1,Goto(allstar-sys|${EXTEN:1}|1)

[radio-iaxrpt]
exten=68536,1,Rpt,68536|X
exten=1999,1,Rpt,1999|X

[allstar-sys]
exten => _x.,1,Ringing
`

func newExtensionsTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ExtensionsConfFile), []byte(extensionsFixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return NewStore(dir)
}

func TestEnsureNodeExtensionsAddsMissingNode(t *testing.T) {
	s := newExtensionsTestStore(t)
	if err := s.EnsureNodeExtensions("52829"); err != nil {
		t.Fatalf("EnsureNodeExtensions: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(s.dir, ExtensionsConfFile))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out := string(raw)

	for _, want := range []string{
		"exten => 52829,1,rpt,52829",
		"exten => 52829,1,rpt,52829|X",
		// AppendKeyValue always renders "key op value" with spaces around
		// the operator, even in radio-iaxrpt where the template's
		// existing lines use "exten=..." with none — that's a cosmetic
		// difference Asterisk doesn't care about, not a bug.
		"exten = 52829,1,Rpt,52829|X",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, out)
		}
	}

	// Existing node 68536's lines and the unrelated [allstar-sys]
	// section must be untouched.
	if !strings.Contains(out, "exten => 68536,1,rpt,68536\n") {
		t.Fatalf("existing 68536 radio-secure line disturbed:\n%s", out)
	}
	if !strings.Contains(out, "[allstar-sys]\nexten => _x.,1,Ringing") {
		t.Fatalf("unrelated section disturbed:\n%s", out)
	}
}

func TestEnsureNodeExtensionsIsIdempotent(t *testing.T) {
	s := newExtensionsTestStore(t)
	if err := s.EnsureNodeExtensions("68536"); err != nil {
		t.Fatalf("EnsureNodeExtensions: %v", err)
	}
	// 68536 already has entries in all three contexts, so nothing should
	// have changed at all.
	raw, _ := os.ReadFile(filepath.Join(s.dir, ExtensionsConfFile))
	if string(raw) != extensionsFixture {
		t.Fatalf("EnsureNodeExtensions changed a file where nothing was missing:\n%s", raw)
	}
}

func TestRemoveNodeExtensionsCleansUpAllContexts(t *testing.T) {
	s := newExtensionsTestStore(t)
	if err := s.RemoveNodeExtensions("68536"); err != nil {
		t.Fatalf("RemoveNodeExtensions: %v", err)
	}

	raw, _ := os.ReadFile(filepath.Join(s.dir, ExtensionsConfFile))
	out := string(raw)
	if strings.Contains(out, "68536") {
		t.Fatalf("68536 should be fully removed, got:\n%s", out)
	}
	// The other node (1999) and unrelated section must survive.
	if !strings.Contains(out, "exten => 1999,1,rpt,1999") {
		t.Fatalf("unrelated node 1999 disturbed:\n%s", out)
	}
	if !strings.Contains(out, "[allstar-sys]") {
		t.Fatalf("unrelated section disturbed:\n%s", out)
	}
}

func TestRemoveNodeExtensionsNoOpForUnknownNode(t *testing.T) {
	s := newExtensionsTestStore(t)
	before, _ := os.ReadFile(filepath.Join(s.dir, ExtensionsConfFile))
	if err := s.RemoveNodeExtensions("99999"); err != nil {
		t.Fatalf("RemoveNodeExtensions: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(s.dir, ExtensionsConfFile))
	if string(before) != string(after) {
		t.Fatalf("file changed for a node with no entries:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
