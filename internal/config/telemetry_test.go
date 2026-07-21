package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseSingleToneRealSingleSegmentValues covers every single-segment
// tone confirmed present verbatim in a real HamVoIP node's
// [telemetryNNNN] stanza (see standardTelemetry) -- these are exactly
// the entries a "Tones & Audio" editor should be able to render as
// friendly Hz/duration/amplitude fields.
func TestParseSingleToneRealSingleSegmentValues(t *testing.T) {
	cases := []struct {
		key, value string
		want       ToneSpec
	}{
		{"ct2", "|t(660,880,150,2048)", ToneSpec{660, 880, 150, 2048}},
		{"ct3", "|t(440,0,150,4096)", ToneSpec{440, 0, 150, 4096}},
		{"ct4", "|t(550,0,150,2048)", ToneSpec{550, 0, 150, 2048}},
		{"ct5", "|t(660,0,150,2048)", ToneSpec{660, 0, 150, 2048}},
		{"ct6", "|t(880,0,150,2048)", ToneSpec{880, 0, 150, 2048}},
		{"ct7", "|t(660,440,150,2048)", ToneSpec{660, 440, 150, 2048}},
		{"ct8", "|t(700,1100,150,2048)", ToneSpec{700, 1100, 150, 2048}},
		{"remotemon", "|t(1209,0,50,2048)", ToneSpec{1209, 0, 50, 2048}},
		{"cmdmode", "|t(900,903,200,2048)", ToneSpec{900, 903, 200, 2048}},
	}
	for _, c := range cases {
		got, ok := ParseSingleTone(c.value)
		if !ok {
			t.Errorf("%s: ParseSingleTone(%q) ok = false, want true", c.key, c.value)
			continue
		}
		if got != c.want {
			t.Errorf("%s: ParseSingleTone(%q) = %+v, want %+v", c.key, c.value, got, c.want)
		}
	}
}

// TestParseSingleToneRejectsMultiSegment covers the real multi-part
// courtesy tones (ct1, remotetx, functcomplete) -- these are still
// working, valid app_rpt values, but ParseSingleTone must reject them
// (not silently keep only the first segment) so the caller falls back
// to raw-text editing instead of corrupting the value on save.
func TestParseSingleToneRejectsMultiSegment(t *testing.T) {
	multi := []string{
		"|t(350,0,100,2048)(500,0,100,2048)(660,0,100,2048)", // ct1
		"|t(1633,0,50,3000)(0,0,80,0)(1209,0,50,3000)",       // remotetx
		"|t(1000,0,100,2048)(0,0,100,0)(1000,0,100,2048)",    // functcomplete
	}
	for _, v := range multi {
		if _, ok := ParseSingleTone(v); ok {
			t.Errorf("ParseSingleTone(%q) ok = true, want false (multi-segment)", v)
		}
		// Still a real tone value, just not a single-segment one -- must
		// be recognized by IsToneValue so it's labeled "tone, edit as raw
		// text" rather than mistaken for a sound file reference.
		if !IsToneValue(v) {
			t.Errorf("IsToneValue(%q) = false, want true", v)
		}
	}
}

// TestIsToneValueRejectsSoundReferences covers the real sound-file
// reference values (patchup/patchdown) -- these must NOT be recognized
// as tones at all, so the UI offers a sound-file picker for them instead
// of trying to parse frequencies out of a filename.
func TestIsToneValueRejectsSoundReferences(t *testing.T) {
	refs := []string{"rpt/callproceeding", "rpt/callterminated", "node-id", ""}
	for _, v := range refs {
		if IsToneValue(v) {
			t.Errorf("IsToneValue(%q) = true, want false", v)
		}
		if _, ok := ParseSingleTone(v); ok {
			t.Errorf("ParseSingleTone(%q) ok = true, want false", v)
		}
	}
}

func TestToneSpecStringRoundTrip(t *testing.T) {
	cases := []string{
		"|t(660,880,150,2048)",
		"|t(440,0,150,4096)",
		"|t(900,903,200,2048)",
	}
	for _, v := range cases {
		spec, ok := ParseSingleTone(v)
		if !ok {
			t.Fatalf("ParseSingleTone(%q) failed", v)
		}
		if got := spec.String(); got != v {
			t.Errorf("round trip: String() = %q, want %q", got, v)
		}
	}
}

// realTelemetrySection mirrors a real [telemetryNNNN] stanza verbatim
// (see standardTelemetry), for ListTelemetryEntries/SetTelemetryEntry
// tests.
const realTelemetrySection = `[telemetry68536]
ct1=|t(350,0,100,2048)(500,0,100,2048)(660,0,100,2048)
ct2=|t(660,880,150,2048)
ct8=|t(700,1100,150,2048)
patchup=rpt/callproceeding
patchdown=rpt/callterminated
`

func newTelemetryTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RptConfFile), []byte(realTelemetrySection), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return NewStore(dir)
}

func TestListTelemetryEntriesRealSection(t *testing.T) {
	s := newTelemetryTestStore(t)
	entries, err := s.ListTelemetryEntries("telemetry68536")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5: %+v", len(entries), entries)
	}
	want := map[string]string{
		"ct1":       "|t(350,0,100,2048)(500,0,100,2048)(660,0,100,2048)",
		"ct2":       "|t(660,880,150,2048)",
		"ct8":       "|t(700,1100,150,2048)",
		"patchup":   "rpt/callproceeding",
		"patchdown": "rpt/callterminated",
	}
	for _, e := range entries {
		if want[e.Key] != e.Value {
			t.Errorf("entry %s = %q, want %q", e.Key, e.Value, want[e.Key])
		}
	}
}

func TestSetTelemetryEntryUpdatesExisting(t *testing.T) {
	s := newTelemetryTestStore(t)
	if err := s.SetTelemetryEntry("telemetry68536", "ct2", "|t(700,0,200,3000)"); err != nil {
		t.Fatal(err)
	}
	entries, err := s.ListTelemetryEntries("telemetry68536")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Key == "ct2" {
			found = true
			if e.Value != "|t(700,0,200,3000)" {
				t.Errorf("ct2 = %q, want updated value", e.Value)
			}
		}
	}
	if !found {
		t.Fatal("ct2 missing after update")
	}
	if len(entries) != 5 {
		t.Errorf("got %d entries after update, want still 5 (update, not append)", len(entries))
	}
}

func TestSetTelemetryEntryCreatesSection(t *testing.T) {
	s := newTelemetryTestStore(t)
	if err := s.SetTelemetryEntry("telemetry99999", "ct1", "|t(660,0,150,2048)"); err != nil {
		t.Fatal(err)
	}
	entries, err := s.ListTelemetryEntries("telemetry99999")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Value != "|t(660,0,150,2048)" {
		t.Fatalf("got %+v", entries)
	}
}
