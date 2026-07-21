package server

import (
	"net/http"
	"strconv"
	"strings"

	"hamvoipconfiggui/internal/config"
)

// soundUploadMaxBytes bounds one upload — generous for a short voice
// recording as an uncompressed WAV, small enough that one request can't
// meaningfully fill the disk.
const soundUploadMaxBytes = 20 << 20

// telemetryRow is one entry in the "Tones & Audio" editor. Mode is
// decided from the entry's actual current value, not its key name —
// app_rpt accepts either a tone-generator string or a sound file
// reference in any telemetry field, so e.g. "patchup" isn't
// hardcoded as a sound field, it's just that its real value never
// happens to parse as a tone.
type telemetryRow struct {
	Key   string
	Value string
	// Mode is "tone" (single-segment — friendly Hz/duration/amplitude
	// fields), "tone-raw" (a real tone this app doesn't offer per-field
	// editing for, because it has more than one segment), or "sound"
	// (not tone syntax at all — edited as a sound-file reference).
	Mode string
	Tone config.ToneSpec // populated only when Mode == "tone"
}

func buildTelemetryRows(entries []config.TelemetryEntry) []telemetryRow {
	rows := make([]telemetryRow, 0, len(entries))
	for _, e := range entries {
		row := telemetryRow{Key: e.Key, Value: e.Value}
		if spec, ok := config.ParseSingleTone(e.Value); ok {
			row.Mode = "tone"
			row.Tone = spec
		} else if config.IsToneValue(e.Value) {
			row.Mode = "tone-raw"
		} else {
			row.Mode = "sound"
		}
		rows = append(rows, row)
	}
	return rows
}

// populateNodeTelemetry fills data's "Tones & Audio" fields: the node's
// telemetry entries as friendly rows (see buildTelemetryRows) and the
// combined custom+stock sound list for the picker. Best-effort, like the
// rest of this page's supplementary data — a read failure just leaves
// the section looking empty rather than failing the whole page.
func (s *Server) populateNodeTelemetry(data *nodeFormData) {
	node := data.Node
	if node == nil || node.Number == "" {
		return
	}
	section := node.Telemetry
	if section == "" {
		section = "telemetry"
	}
	data.TelemetrySect = section
	if entries, err := s.store.ListTelemetryEntries(section); err == nil {
		data.TelemetryRows = buildTelemetryRows(entries)
	}
	if files, err := s.sounds.ListAll(); err == nil {
		data.SoundFiles = files
	}
}

// atoiField parses a submitted numeric field, reporting ok=false for
// anything that doesn't fully parse (missing, blank, non-numeric) — used
// so a malformed or incomplete submission leaves a tone entry untouched
// rather than silently writing over it with zeroed-out frequencies.
func atoiField(v string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	return n, err == nil
}

// handleNodeTelemetrySave saves every row of the "Tones & Audio" editor
// in one submission, matching the Node identity section's own "one
// form, one Save button" pattern rather than a form per row — there's
// nothing to add or delete here, since telemetry keys are fixed by
// app_rpt itself, not user-invented like DTMF digits, only to edit.
//
// The current key list and each entry's tone-vs-sound classification are
// both read fresh from disk rather than trusted from the submitted
// form, so a stale or manually-crafted request can't resurrect a key
// that no longer exists or force a sound-reference field to be
// (mis)parsed as a tone.
func (s *Server) handleNodeTelemetrySave(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	node, err := s.store.LoadNode(number)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	section := node.Telemetry
	if section == "" {
		section = "telemetry"
	}
	entries, err := s.store.ListTelemetryEntries(section)
	if err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}

	var failed []string
	for _, e := range entries {
		key := e.Key
		var value string
		if _, isTone := config.ParseSingleTone(e.Value); isTone {
			f1, ok1 := atoiField(r.FormValue("tone_" + key + "_freq1"))
			f2, ok2 := atoiField(r.FormValue("tone_" + key + "_freq2"))
			dur, ok3 := atoiField(r.FormValue("tone_" + key + "_duration"))
			amp, ok4 := atoiField(r.FormValue("tone_" + key + "_amplitude"))
			if !ok1 || !ok2 || !ok3 || !ok4 {
				continue // incomplete submission for this row -- leave it as-is
			}
			value = config.ToneSpec{Freq1: f1, Freq2: f2, DurationMS: dur, Amplitude: amp}.String()
		} else {
			value = strings.TrimSpace(r.FormValue("raw_" + key))
			if value == "" {
				continue // never blank out a real value
			}
		}
		if value == e.Value {
			continue
		}
		if err := s.store.SetTelemetryEntry(section, key, value); err != nil {
			failed = append(failed, key+": "+err.Error())
		}
	}

	if len(failed) > 0 {
		s.renderNodeEditPage(w, r, number, flash("error", "Some tones/audio couldn't be saved: "+strings.Join(failed, "; ")))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeSoundUpload handles an uploaded audio file (typically a
// WAV), transcodes it via sox, and saves it as a custom sound the
// operator can then pick for idrecording or any telemetry entry.
// Node-agnostic underneath — sound files are shared across the whole
// box, not per-node — but reached from a node's own page since that's
// where an operator actually needs one, and errors/success land back on
// that same page rather than a separate one.
func (s *Server) handleNodeSoundUpload(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseMultipartForm(soundUploadMaxBytes); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Upload too large or malformed: "+err.Error()))
		return
	}
	name := strings.TrimSpace(r.FormValue("sound_name"))
	if name == "" {
		s.renderNodeEditPage(w, r, number, flash("error", "Enter a name for this sound file"))
		return
	}
	file, _, err := r.FormFile("sound_file")
	if err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Choose an audio file to upload"))
		return
	}
	defer file.Close()

	if _, err := s.sounds.Upload(r.Context(), name, file); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Couldn't convert the uploaded file: "+err.Error()))
		return
	}
	s.renderNodeEditPage(w, r, number, flash("ok", "Uploaded \""+name+"\" — pick it from any sound field below."))
}

// handleNodeSoundDelete removes one of the operator's own custom sound
// files. Never reachable for a stock library file — see
// sounds.Store.DeleteCustom, which only ever touches the custom
// directory.
func (s *Server) handleNodeSoundDelete(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	if err := s.sounds.DeleteCustom(name); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	s.renderNodeEditPage(w, r, number, flash("ok", "Deleted sound \""+name+"\"."))
}
