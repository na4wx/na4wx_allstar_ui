package server

import (
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
	rows := buildTelemetryRows(entries)
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
