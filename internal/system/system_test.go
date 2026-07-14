package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogTailShortFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	lines, err := LogTail(path, 3)
	if err != nil {
		t.Fatalf("LogTail: %v", err)
	}
	want := []string{"line3", "line4", "line5"}
	if len(lines) != len(want) {
		t.Fatalf("LogTail = %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("LogTail[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestLogTailMoreLinesThanFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte("only\ntwo\n"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	lines, err := LogTail(path, 50)
	if err != nil {
		t.Fatalf("LogTail: %v", err)
	}
	if len(lines) != 2 || lines[0] != "only" || lines[1] != "two" {
		t.Fatalf("LogTail = %v", lines)
	}
}

func TestLogTailLargeFileSeeksNearEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	var b strings.Builder
	for i := 0; i < 100000; i++ {
		b.WriteString("filler line to pad the file out past the seek window\n")
	}
	b.WriteString("THE-LAST-LINE\n")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	lines, err := LogTail(path, 5)
	if err != nil {
		t.Fatalf("LogTail: %v", err)
	}
	if len(lines) == 0 || lines[len(lines)-1] != "THE-LAST-LINE" {
		t.Fatalf("LogTail last line = %v, want THE-LAST-LINE", lines)
	}
}

const testProcAsoundCards = ` 0 [Device         ]: USB-Audio - USB PnP Sound Device
                      C-Media Electronics Inc. USB PnP Sound Device at usb-3f980000.usb-1.4, full speed
 1 [usb1           ]: USB-Audio - USB PnP Sound Device
                      C-Media Electronics Inc. USB PnP Sound Device at usb-3f980000.usb-1.5, full speed
 2 [Headphones     ]: bcm2835 Headphones - bcm2835 Headphones
                      bcm2835 Headphones
`

func TestParseSoundCards(t *testing.T) {
	cards := parseSoundCards(testProcAsoundCards)
	if len(cards) != 3 {
		t.Fatalf("parseSoundCards = %v, want 3 entries", cards)
	}
	if cards[0].Index != 0 || cards[0].ID != "Device" || !cards[0].IsUSB {
		t.Fatalf("cards[0] = %+v", cards[0])
	}
	if cards[1].ID != "usb1" || !cards[1].IsUSB {
		t.Fatalf("cards[1] = %+v", cards[1])
	}
	if cards[2].ID != "Headphones" || cards[2].IsUSB {
		t.Fatalf("cards[2] = %+v, want IsUSB=false", cards[2])
	}
}

func TestParseSoundCardsEmpty(t *testing.T) {
	cards := parseSoundCards("")
	if len(cards) != 0 {
		t.Fatalf("parseSoundCards(\"\") = %v, want empty", cards)
	}
}

func TestListSoundCardsMissingFileIsNotAnError(t *testing.T) {
	// /proc/asound/cards doesn't exist on this dev machine (macOS) —
	// this must degrade to an empty result, not an error, since the
	// caller's fallback is "let the user type it in manually."
	cards, err := ListSoundCards()
	if err != nil {
		t.Fatalf("ListSoundCards: %v", err)
	}
	if cards != nil {
		t.Fatalf("expected nil cards on a system with no /proc/asound, got %v", cards)
	}
}

func TestListNetworkInterfacesExcludesLoopback(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"lo", "eth0", "wlan0"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	names, err := listNetworkInterfaces(dir)
	if err != nil {
		t.Fatalf("listNetworkInterfaces: %v", err)
	}
	want := []string{"eth0", "wlan0"}
	if len(names) != len(want) {
		t.Fatalf("listNetworkInterfaces = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("listNetworkInterfaces = %v, want %v", names, want)
		}
	}
}

func TestListNetworkInterfacesMissingDirIsNotAnError(t *testing.T) {
	names, err := listNetworkInterfaces(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("listNetworkInterfaces: %v", err)
	}
	if names != nil {
		t.Fatalf("expected nil, got %v", names)
	}
}

func TestSetHostnameRejectsInvalid(t *testing.T) {
	bad := []string{"", "-bad", "bad-", "bad_host", "has spaces", strings.Repeat("a", 64)}
	for _, h := range bad {
		if hostnameRe.MatchString(h) {
			t.Errorf("hostnameRe unexpectedly matched invalid hostname %q", h)
		}
	}
	good := []string{"hamvoip", "node-2000", "a", strings.Repeat("a", 63)}
	for _, h := range good {
		if !hostnameRe.MatchString(h) {
			t.Errorf("hostnameRe unexpectedly rejected valid hostname %q", h)
		}
	}
}
