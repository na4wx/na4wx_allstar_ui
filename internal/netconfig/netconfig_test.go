package netconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baseDhcpcdConf = `# dhcpcd.conf
hostname
clientid
persistent
option rapid_commit

interface wlan0
nohook wpa_supplicant
`

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "dhcpcd.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestReadManagedBlockAbsent(t *testing.T) {
	path := writeFixture(t, baseDhcpcdConf)
	cfg, err := ReadManagedBlock(path)
	if err != nil {
		t.Fatalf("ReadManagedBlock: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config (DHCP), got %+v", cfg)
	}
}

func TestReadManagedBlockMissingFile(t *testing.T) {
	cfg, err := ReadManagedBlock(filepath.Join(t.TempDir(), "nope.conf"))
	if err != nil {
		t.Fatalf("ReadManagedBlock on missing file: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config for missing file, got %+v", cfg)
	}
}

func TestWriteThenReadRoundTrip(t *testing.T) {
	path := writeFixture(t, baseDhcpcdConf)
	want := &StaticConfig{
		Interface: "eth0",
		Address:   "192.168.1.10/24",
		Router:    "192.168.1.1",
		DNS:       "8.8.8.8 8.8.4.4",
	}
	if err := WriteManagedBlock(path, want); err != nil {
		t.Fatalf("WriteManagedBlock: %v", err)
	}

	got, err := ReadManagedBlock(path)
	if err != nil {
		t.Fatalf("ReadManagedBlock: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil config after write")
	}
	if *got != *want {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	// Original, unrelated content must survive untouched.
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "nohook wpa_supplicant") {
		t.Fatalf("unrelated dhcpcd.conf content was lost:\n%s", raw)
	}
}

func TestWriteOverwritesPreviousBlock(t *testing.T) {
	path := writeFixture(t, baseDhcpcdConf)
	first := &StaticConfig{Interface: "eth0", Address: "10.0.0.5/24"}
	if err := WriteManagedBlock(path, first); err != nil {
		t.Fatalf("WriteManagedBlock (first): %v", err)
	}
	second := &StaticConfig{Interface: "eth0", Address: "10.0.0.9/24", Router: "10.0.0.1"}
	if err := WriteManagedBlock(path, second); err != nil {
		t.Fatalf("WriteManagedBlock (second): %v", err)
	}

	got, err := ReadManagedBlock(path)
	if err != nil {
		t.Fatalf("ReadManagedBlock: %v", err)
	}
	if got.Address != "10.0.0.9/24" || got.Router != "10.0.0.1" {
		t.Fatalf("got %+v, want the second config", got)
	}

	raw, _ := os.ReadFile(path)
	if strings.Count(string(raw), beginMarker) != 1 {
		t.Fatalf("expected exactly one managed block, got:\n%s", raw)
	}
}

func TestWriteNilRemovesBlockRevertsToDHCP(t *testing.T) {
	path := writeFixture(t, baseDhcpcdConf)
	if err := WriteManagedBlock(path, &StaticConfig{Interface: "eth0", Address: "10.0.0.5/24"}); err != nil {
		t.Fatalf("WriteManagedBlock: %v", err)
	}
	if err := WriteManagedBlock(path, nil); err != nil {
		t.Fatalf("WriteManagedBlock(nil): %v", err)
	}

	cfg, err := ReadManagedBlock(path)
	if err != nil {
		t.Fatalf("ReadManagedBlock: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected DHCP (nil) after removing block, got %+v", cfg)
	}

	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "nohook wpa_supplicant") {
		t.Fatalf("unrelated content lost after block removal:\n%s", raw)
	}
	if strings.Contains(string(raw), beginMarker) {
		t.Fatalf("marker still present after removal:\n%s", raw)
	}
}

func TestWriteToMissingFileCreatesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dhcpcd.conf")
	cfg := &StaticConfig{Interface: "eth0", Address: "192.168.1.10/24"}
	if err := WriteManagedBlock(path, cfg); err != nil {
		t.Fatalf("WriteManagedBlock: %v", err)
	}
	got, err := ReadManagedBlock(path)
	if err != nil {
		t.Fatalf("ReadManagedBlock: %v", err)
	}
	if got == nil || got.Address != "192.168.1.10/24" {
		t.Fatalf("got %+v", got)
	}
}
