package server

import (
	"strings"
	"testing"

	"hamvoipconfiggui/internal/config"
)

// TestBuildTelemetryRowsRealEntries mirrors a real [telemetryNNNN]
// stanza (see config.standardTelemetry) and pins the three-way mode
// classification a "Tones & Audio" editor depends on.
func TestBuildTelemetryRowsRealEntries(t *testing.T) {
	entries := []config.TelemetryEntry{
		{Key: "ct1", Value: "|t(350,0,100,2048)(500,0,100,2048)(660,0,100,2048)"},
		{Key: "ct2", Value: "|t(660,880,150,2048)"},
		{Key: "patchup", Value: "rpt/callproceeding"},
	}
	rows := buildTelemetryRows(entries, nil)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}

	if rows[0].Mode != "tone-raw" {
		t.Errorf("ct1 Mode = %q, want tone-raw (multi-segment)", rows[0].Mode)
	}

	if rows[1].Mode != "tone" {
		t.Errorf("ct2 Mode = %q, want tone", rows[1].Mode)
	}
	want := config.ToneSpec{Freq1: 660, Freq2: 880, DurationMS: 150, Amplitude: 2048}
	if rows[1].Tone != want {
		t.Errorf("ct2 Tone = %+v, want %+v", rows[1].Tone, want)
	}

	if rows[2].Mode != "sound" {
		t.Errorf("patchup Mode = %q, want sound", rows[2].Mode)
	}
}

func TestAtoiField(t *testing.T) {
	if n, ok := atoiField("2048"); !ok || n != 2048 {
		t.Errorf("atoiField(2048) = %d, %v", n, ok)
	}
	if n, ok := atoiField(" 150 "); !ok || n != 150 {
		t.Errorf("atoiField( 150 ) = %d, %v, want 150, true", n, ok)
	}
	for _, bad := range []string{"", "abc", "12.5"} {
		if _, ok := atoiField(bad); ok {
			t.Errorf("atoiField(%q) ok = true, want false", bad)
		}
	}
}

// TestTelemetryKeyDescriptionRealAssignments uses the exact courtesy-
// tone assignments confirmed present in a real node's own rpt.conf
// (unlinkedct=ct2, remotect=ct3, linkunkeyct=ct8) -- this is the crux of
// making ct1/ct2/etc. readable: the meaning comes from these fields, not
// from the ctN number itself.
func TestTelemetryKeyDescriptionRealAssignments(t *testing.T) {
	node := &config.Node{UnlinkedCT: "ct2", RemoteCT: "ct3", LinkUnkeyCT: "ct8"}

	if got := telemetryKeyDescription("ct2", node); !contains(got, "isn't connected") {
		t.Errorf("ct2 description = %q, want it to explain the unlinked role", got)
	}
	if got := telemetryKeyDescription("ct3", node); !contains(got, "remote base") {
		t.Errorf("ct3 description = %q, want it to explain the remote-base role", got)
	}
	if got := telemetryKeyDescription("ct8", node); !contains(got, "unkeys") {
		t.Errorf("ct8 description = %q, want it to explain the link-unkey role", got)
	}
	// ct1 isn't assigned to any of the three roles on this node -- must
	// say so rather than inventing a meaning or silently matching ct2's.
	if got := telemetryKeyDescription("ct1", node); contains(got, "isn't connected") || contains(got, "remote base") || contains(got, "unkeys") {
		t.Errorf("ct1 description = %q, want it to NOT claim any of the assigned roles", got)
	}
}

func TestTelemetryKeyDescriptionFixedKeys(t *testing.T) {
	cases := map[string]string{
		"cmdmode":       "touch-tone command",
		"functcomplete": "finishes successfully",
		"patchup":       "connects",
		"patchdown":     "ends",
		"remotetx":      "remote base radio",
		"remotemon":     "remote base radio",
	}
	for key, want := range cases {
		if got := telemetryKeyDescription(key, nil); !contains(got, want) {
			t.Errorf("telemetryKeyDescription(%q) = %q, want it to contain %q", key, got, want)
		}
	}
}

func TestTelemetryKeyLabel(t *testing.T) {
	if got := telemetryKeyLabel("ct4"); got != "Courtesy tone" {
		t.Errorf("label(ct4) = %q, want %q", got, "Courtesy tone")
	}
	if got := telemetryKeyLabel("cmdmode"); got == "" {
		t.Error("label(cmdmode) is empty, want a real label")
	}
	if got := telemetryKeyLabel("some_custom_key"); got != "" {
		t.Errorf("label(some_custom_key) = %q, want empty for an unrecognized key", got)
	}
}

func TestCourtesyToneKeys(t *testing.T) {
	entries := []config.TelemetryEntry{
		{Key: "ct1", Value: "x"},
		{Key: "ct2", Value: "x"},
		{Key: "patchup", Value: "x"},
		{Key: "ct8", Value: "x"},
	}
	got := courtesyToneKeys(entries)
	want := []string{"ct1", "ct2", "ct8"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
