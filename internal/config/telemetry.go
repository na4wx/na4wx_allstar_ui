package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ToneSpec is one single-segment app_rpt tone-generator instruction —
// the value format used throughout a real node's [telemetryNNNN] stanza
// for courtesy tones etc., e.g. "|t(660,880,150,2048)". Freq2 is 0 for a
// single-frequency tone, or a second frequency for a dual-tone sound.
// DurationMS and Amplitude are exactly what app_rpt itself calls them in
// its own tone-generator syntax.
type ToneSpec struct {
	Freq1, Freq2, DurationMS, Amplitude int
}

// String renders back to app_rpt's own "|t(f1,f2,dur,amp)" syntax.
func (t ToneSpec) String() string {
	return fmt.Sprintf("|t(%d,%d,%d,%d)", t.Freq1, t.Freq2, t.DurationMS, t.Amplitude)
}

// singleToneRe matches exactly one tone-generator segment and nothing
// else — anchored at both ends, so a multi-segment value (several
// "(...)" groups back to back, e.g. a 3-part courtesy tone) does not
// match and is left for ParseSingleTone to reject.
var singleToneRe = regexp.MustCompile(`^\|t\((-?\d+),(-?\d+),(\d+),(\d+)\)$`)

// anyToneSegmentRe matches the general tone-generator shape, one or more
// "(...)" segments, used only to tell "a tone this app doesn't offer a
// friendly per-field editor for yet" apart from "not a tone at all,
// probably a sound file reference" — see IsToneValue.
var anyToneSegmentRe = regexp.MustCompile(`^\|t(\((-?\d+),(-?\d+),(\d+),(\d+)\))+$`)

// ParseSingleTone parses value as exactly one "|t(f1,f2,dur,amp)"
// segment. ok is false for anything else: no segments, more than one
// segment (a real, multi-part courtesy tone like
// "|t(350,0,100,2048)(500,0,100,2048)(660,0,100,2048)"), or not tone
// syntax at all (e.g. a sound file reference like "rpt/callproceeding").
// A multi-segment tone is still a valid, working value — this just
// means the caller should fall back to editing it as raw text rather
// than silently truncating it to one segment.
func ParseSingleTone(value string) (ToneSpec, bool) {
	m := singleToneRe.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return ToneSpec{}, false
	}
	f1, _ := strconv.Atoi(m[1])
	f2, _ := strconv.Atoi(m[2])
	dur, _ := strconv.Atoi(m[3])
	amp, _ := strconv.Atoi(m[4])
	return ToneSpec{Freq1: f1, Freq2: f2, DurationMS: dur, Amplitude: amp}, true
}

// IsToneValue reports whether value is any app_rpt tone-generator string
// (one or more "|t(...)" segments) at all, regardless of whether
// ParseSingleTone can break it into friendly fields.
func IsToneValue(value string) bool {
	return anyToneSegmentRe.MatchString(strings.TrimSpace(value))
}

// TelemetryEntry is one key/value pair from a node's [telemetryNNNN]
// stanza — a courtesy tone (e.g. "ct1"), a named event tone ("cmdmode",
// "remotetx"), or a sound-file playback reference ("patchup",
// "patchdown"). Which of those a given key is isn't fixed by the key
// name — app_rpt accepts either a tone-generator string or a sound file
// name in any of these fields — so this only carries the raw value;
// callers decide how to render it by trying ParseSingleTone/IsToneValue
// on Value, not by switching on Key.
type TelemetryEntry struct {
	Key   string
	Value string
}

// ListTelemetryEntries returns a telemetry stanza's key/value pairs in
// file order. Structurally identical to ListFunctionMacros (both just
// list a section's keys), kept as a separate, telemetry-named method
// rather than reusing FunctionMacro's Digits/Command field names, which
// would read strangely for callers working with telemetry keys.
func (s *Store) ListTelemetryEntries(section string) ([]TelemetryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return nil, err
	}
	kv := f.SectionKeys(section)
	out := make([]TelemetryEntry, 0, len(kv))
	for _, p := range kv {
		out = append(out, TelemetryEntry{Key: p.Key, Value: p.Value})
	}
	return out, nil
}

// SetTelemetryEntry adds or updates one key in a telemetry stanza,
// creating the section if it doesn't exist yet.
func (s *Store) SetTelemetryEntry(section, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	f.EnsureSection(section)
	f.Set(section, key, value)
	return s.save(RptConfFile, f)
}
