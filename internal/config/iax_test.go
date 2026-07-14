package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testIaxConf = `[general]
bindport = 4569
disallow = all
allow = ulaw

register => 2000:secretpass@register.allstarlink.org

[2000]
type = friend
context = radio-secure
host = dynamic
secret = secretpass
auth = md5
`

func newIaxTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, IaxConfFile), []byte(testIaxConf), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return NewStore(dir)
}

func TestParseRegistration(t *testing.T) {
	cases := []struct {
		value string
		want  Registration
		ok    bool
	}{
		{"2000:secret@register.allstarlink.org", Registration{"2000", "secret", "register.allstarlink.org", ""}, true},
		{"2000:secret@register.allstarlink.org:4569", Registration{"2000", "secret", "register.allstarlink.org", "4569"}, true},
		{"garbage", Registration{}, false},
		{"noatsign:secret", Registration{}, false},
	}
	for _, c := range cases {
		got, ok := parseRegistration(c.value)
		if ok != c.ok {
			t.Errorf("parseRegistration(%q) ok = %v, want %v", c.value, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("parseRegistration(%q) = %+v, want %+v", c.value, got, c.want)
		}
	}
}

func TestListRegistrations(t *testing.T) {
	s := newIaxTestStore(t)
	regs, err := s.ListRegistrations()
	if err != nil {
		t.Fatalf("ListRegistrations: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("ListRegistrations = %v, want 1", regs)
	}
	if regs[0].Node != "2000" || regs[0].Password != "secretpass" || regs[0].Host != "register.allstarlink.org" {
		t.Fatalf("regs[0] = %+v", regs[0])
	}
}

func TestLoadRegistrationNotFound(t *testing.T) {
	s := newIaxTestStore(t)
	reg, err := s.LoadRegistration("9999")
	if err != nil {
		t.Fatalf("LoadRegistration: %v", err)
	}
	if reg != nil {
		t.Fatalf("expected nil for missing registration, got %+v", reg)
	}
}

func TestSaveRegistrationUpdatesExisting(t *testing.T) {
	s := newIaxTestStore(t)
	err := s.SaveRegistration(Registration{Node: "2000", Password: "newpass", Host: "register.allstarlink.org", Port: "4569"})
	if err != nil {
		t.Fatalf("SaveRegistration: %v", err)
	}
	reg, err := s.LoadRegistration("2000")
	if err != nil {
		t.Fatalf("LoadRegistration: %v", err)
	}
	if reg.Password != "newpass" || reg.Port != "4569" {
		t.Fatalf("reg after update = %+v", reg)
	}

	// Must still be exactly one registration, and unrelated content
	// (the [2000] peer stanza) must survive.
	regs, _ := s.ListRegistrations()
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration after update, got %d: %v", len(regs), regs)
	}
	raw, _ := os.ReadFile(filepath.Join(s.dir, IaxConfFile))
	if !strings.Contains(string(raw), "context = radio-secure") {
		t.Fatalf("peer stanza lost:\n%s", raw)
	}
}

func TestSaveRegistrationAddsNew(t *testing.T) {
	s := newIaxTestStore(t)
	err := s.SaveRegistration(Registration{Node: "3000", Password: "pw3000", Host: "register.allstarlink.org"})
	if err != nil {
		t.Fatalf("SaveRegistration: %v", err)
	}
	regs, err := s.ListRegistrations()
	if err != nil {
		t.Fatalf("ListRegistrations: %v", err)
	}
	if len(regs) != 2 {
		t.Fatalf("ListRegistrations = %v, want 2", regs)
	}
}

func TestSaveRegistrationRejectsIncomplete(t *testing.T) {
	s := newIaxTestStore(t)
	err := s.SaveRegistration(Registration{Node: "2000"})
	if err == nil {
		t.Fatalf("expected error for incomplete registration")
	}
}

func TestDeleteRegistration(t *testing.T) {
	s := newIaxTestStore(t)
	if err := s.DeleteRegistration("2000"); err != nil {
		t.Fatalf("DeleteRegistration: %v", err)
	}
	regs, err := s.ListRegistrations()
	if err != nil {
		t.Fatalf("ListRegistrations: %v", err)
	}
	if len(regs) != 0 {
		t.Fatalf("ListRegistrations after delete = %v", regs)
	}
	// Peer stanza is untouched by DeleteRegistration.
	raw, _ := os.ReadFile(filepath.Join(s.dir, IaxConfFile))
	if !strings.Contains(string(raw), "[2000]") {
		t.Fatalf("peer stanza should survive registration deletion:\n%s", raw)
	}
}

func TestLoadPeer(t *testing.T) {
	s := newIaxTestStore(t)
	p, err := s.LoadPeer("2000")
	if err != nil {
		t.Fatalf("LoadPeer: %v", err)
	}
	if p == nil {
		t.Fatalf("expected peer, got nil")
	}
	if p.Type != "friend" || p.Context != "radio-secure" || p.Secret != "secretpass" {
		t.Fatalf("peer = %+v", p)
	}
}

func TestLoadPeerNotFound(t *testing.T) {
	s := newIaxTestStore(t)
	p, err := s.LoadPeer("9999")
	if err != nil {
		t.Fatalf("LoadPeer: %v", err)
	}
	if p != nil {
		t.Fatalf("expected nil peer, got %+v", p)
	}
}

func TestSavePeerCreatesNew(t *testing.T) {
	s := newIaxTestStore(t)
	p := &Peer{Node: "3000", Type: "friend", Context: "radio-secure", Host: "dynamic", Secret: "pw3000", Auth: "md5"}
	if err := s.SavePeer(p); err != nil {
		t.Fatalf("SavePeer: %v", err)
	}
	got, err := s.LoadPeer("3000")
	if err != nil {
		t.Fatalf("LoadPeer: %v", err)
	}
	if got.Secret != "pw3000" {
		t.Fatalf("got = %+v", got)
	}
}

func TestSavePeerUpdatesAndClearsFields(t *testing.T) {
	s := newIaxTestStore(t)
	p, err := s.LoadPeer("2000")
	if err != nil {
		t.Fatalf("LoadPeer: %v", err)
	}
	p.Auth = ""
	p.Secret = "rotatedpass"
	if err := s.SavePeer(p); err != nil {
		t.Fatalf("SavePeer: %v", err)
	}
	got, err := s.LoadPeer("2000")
	if err != nil {
		t.Fatalf("LoadPeer: %v", err)
	}
	if got.Auth != "" {
		t.Fatalf("Auth should have been cleared, got %q", got.Auth)
	}
	if got.Secret != "rotatedpass" {
		t.Fatalf("Secret = %q", got.Secret)
	}
	if got.Context != "radio-secure" {
		t.Fatalf("untouched Context = %q", got.Context)
	}
}
