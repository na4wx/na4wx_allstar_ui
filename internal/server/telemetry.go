package server

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"strings"

	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/tts"
)

// soundUploadMaxBytes bounds one upload — generous for a short voice
// recording as an uncompressed WAV, small enough that one request can't
// meaningfully fill the disk.
const soundUploadMaxBytes = 20 << 20

// espeakFallbackTool is used when Piper cannot run on older system
// libraries but text-to-speech is still desired.
const espeakFallbackTool = "espeak-ng"

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

	// Label and Description explain what this entry is *for*, since a
	// bare key like "ct1" means nothing to someone who didn't write
	// app_rpt. See telemetryKeyLabel/telemetryKeyDescription.
	Label       string
	Description string
}

// telemetryKeyLabel gives a plain-English name for a telemetry key,
// sourced from AllStarLink's own rpt.conf documentation
// (allstarlink.github.io/config/rpt_conf) rather than guessed. Courtesy
// tones (ct1-ct8) don't get a fixed label here — what each one is
// *for*, if anything, depends on this node's own unlinkedct/remotect/
// linkunkeyct fields, which telemetryKeyDescription reads directly
// instead of assuming a universal meaning that could be wrong for a
// node that assigns them differently.
func telemetryKeyLabel(key string) string {
	switch key {
	case "cmdmode":
		return "Command-mode beep"
	case "functcomplete":
		return "Command-complete tone"
	case "patchup":
		return "Autopatch connected"
	case "patchdown":
		return "Autopatch ended"
	case "remotetx":
		return "Remote base transmitting"
	case "remotemon":
		return "Remote base monitoring"
	default:
		if isCourtesyToneKey(key) {
			return "Courtesy tone"
		}
		return ""
	}
}

// telemetryKeyDescription explains what a telemetry entry is for. For
// the fixed-meaning keys this is a static, sourced description; for a
// courtesy tone (ct1-ct8) it's built from node's own
// UnlinkedCT/RemoteCT/LinkUnkeyCT fields, since app_rpt doesn't give
// ctN a fixed meaning by number — the node's own settings decide which
// one plays in which situation (confirmed both in AllStarLink's rpt.conf
// docs and in a real node's own inline comments: unlinkedct=ct2,
// remotect=ct3, linkunkeyct=ct8 on that node — a different node could
// assign the exact same three roles to entirely different numbers).
func telemetryKeyDescription(key string, node *config.Node) string {
	switch key {
	case "cmdmode":
		return "Plays when you start entering a touch-tone command, confirming the node is listening for it."
	case "functcomplete":
		return "Plays right after a touch-tone command finishes successfully."
	case "patchup":
		return "Plays when an autopatch (phone) call connects."
	case "patchdown":
		return "Plays when an autopatch (phone) call ends."
	case "remotetx":
		return "Only used if this node controls a remote base radio: plays when that remote radio starts transmitting."
	case "remotemon":
		return "Only used if this node controls a remote base radio: plays while monitoring that remote radio."
	}
	if !isCourtesyToneKey(key) {
		return ""
	}
	var roles []string
	if node != nil {
		if key == node.UnlinkedCT && key != "" {
			roles = append(roles, "this node isn't connected to any other node")
		}
		if key == node.RemoteCT && key != "" {
			roles = append(roles, "a remote base radio is connected locally")
		}
		if key == node.LinkUnkeyCT && key != "" {
			roles = append(roles, "a connected node unkeys")
		}
	}
	if len(roles) == 0 {
		return "One of this node's courtesy tones. It isn't currently assigned to unlinked/remote-base/link-unkey below, so check what uses it before changing it."
	}
	return "Plays when " + strings.Join(roles, ", and also when ") + "."
}

// isCourtesyToneKey reports whether key is one of app_rpt's fixed
// courtesy-tone slots (ct1 through ct8, per its own documentation — this
// isn't an arbitrary "ct"-prefixed match, it's exactly that set).
func isCourtesyToneKey(key string) bool {
	switch key {
	case "ct1", "ct2", "ct3", "ct4", "ct5", "ct6", "ct7", "ct8":
		return true
	default:
		return false
	}
}

func buildTelemetryRows(entries []config.TelemetryEntry, node *config.Node) []telemetryRow {
	rows := make([]telemetryRow, 0, len(entries))
	for _, e := range entries {
		row := telemetryRow{
			Key:         e.Key,
			Value:       e.Value,
			Label:       telemetryKeyLabel(e.Key),
			Description: telemetryKeyDescription(e.Key, node),
		}
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

// courtesyToneKeys returns the courtesy-tone keys (ct1-ct8) actually
// present in entries, in file order — the valid choices for this node's
// unlinkedct/remotect/linkunkeyct assignment fields. Built from what's
// really there rather than always offering all 8, since a node's
// telemetry section might only define the ones it actually uses.
func courtesyToneKeys(entries []config.TelemetryEntry) []string {
	var keys []string
	for _, e := range entries {
		if isCourtesyToneKey(e.Key) {
			keys = append(keys, e.Key)
		}
	}
	return keys
}

func formatTTSError(prefix string, output string) string {
	msg := prefix
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		msg += " — " + trimmed
	}
	return msg
}

func findVoiceByName(voices []tts.Voice, name string) (tts.Voice, bool) {
	for _, v := range voices {
		if v.Name == name {
			return v, true
		}
	}
	return tts.Voice{}, false
}

// resolveTTSBackend chooses which engine this request/page should use:
// Piper first when healthy and voices exist, otherwise espeak-ng.
func (s *Server) resolveTTSBackend(ctx context.Context) (engine string, voices []tts.Voice, note string, errMsg string) {
	var piperErr string
	if piperVoices, err := tts.ListVoices(s.ttsVoicesDir); err == nil && len(piperVoices) > 0 {
		if output, checkErr := tts.CheckTool(ctx, s.ttsTool, "--help"); checkErr == nil {
			return "piper", piperVoices, "", ""
		} else {
			piperErr = formatTTSError("Piper couldn't start", output)
		}
	}

	espeakCheckOut, espeakCheckErr := tts.CheckTool(ctx, espeakFallbackTool, "--version")
	if espeakCheckErr == nil {
		espeakVoices, espeakVoiceOut, espeakVoicesErr := tts.ListESpeakVoices(ctx, espeakFallbackTool)
		if espeakVoicesErr == nil {
			note := ""
			if piperErr != "" {
				note = "Using espeak-ng fallback because Piper is unavailable on this system."
			}
			return "espeak-ng", espeakVoices, note, ""
		}
		return "", nil, "", formatTTSError("Text-to-speech is unavailable because espeak-ng voices could not be listed", espeakVoiceOut)
	}

	if piperErr != "" {
		return "", nil, "", "Text-to-speech is unavailable. " + piperErr + " " + formatTTSError("espeak-ng also couldn't start", espeakCheckOut)
	}
	return "", nil, "", formatTTSError("Text-to-speech is unavailable because espeak-ng couldn't start", espeakCheckOut)
}

func (s *Server) synthesizeWithEngine(ctx context.Context, engine, voiceName, text string) (wav []byte, output string, userErr string, err error) {
	switch engine {
	case "piper":
		voice, ok, findErr := tts.FindVoice(s.ttsVoicesDir, voiceName)
		if findErr != nil {
			return nil, "", "", findErr
		}
		if !ok {
			return nil, "", "Pick a voice — none selected, or it's no longer available", nil
		}
		if checkOut, checkErr := tts.CheckTool(ctx, s.ttsTool, "--help"); checkErr != nil {
			return nil, "", formatTTSError("Text-to-speech is unavailable because Piper couldn't start", checkOut), nil
		}
		wav, output, err = tts.Synthesize(ctx, s.ttsTool, voice.ModelPath, text)
		return wav, output, "", err

	case "espeak-ng":
		if checkOut, checkErr := tts.CheckTool(ctx, espeakFallbackTool, "--version"); checkErr != nil {
			return nil, "", formatTTSError("Text-to-speech is unavailable because espeak-ng couldn't start", checkOut), nil
		}
		espeakVoices, espeakVoiceOut, listErr := tts.ListESpeakVoices(ctx, espeakFallbackTool)
		if listErr != nil {
			return nil, "", formatTTSError("Text-to-speech is unavailable because espeak-ng voices could not be listed", espeakVoiceOut), nil
		}
		if _, ok := findVoiceByName(espeakVoices, voiceName); !ok {
			return nil, "", "Pick a voice — none selected, or it's no longer available", nil
		}
		wav, output, err = tts.SynthesizeESpeak(ctx, espeakFallbackTool, voiceName, text)
		return wav, output, "", err

	default:
		resolvedEngine, voices, _, msg := s.resolveTTSBackend(ctx)
		if msg != "" {
			return nil, "", msg, nil
		}
		if resolvedEngine == "espeak-ng" && len(voices) > 0 && voiceName == "" {
			voiceName = voices[0].Name
		}
		return s.synthesizeWithEngine(ctx, resolvedEngine, voiceName, text)
	}
}

// populateNodeTelemetry fills data's "Tones & Audio" fields: the node's
// telemetry entries as friendly rows (see buildTelemetryRows), the
// combined custom+stock sound list for the picker, and whichever
// text-to-speech voices are available for "Create from text" (see
// internal/tts's package doc). Best-effort, like the rest of this page's
// supplementary data — a read failure just leaves the section looking
// empty rather than failing the whole page.
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
		data.TelemetryRows = buildTelemetryRows(entries, node)
		data.CTKeys = courtesyToneKeys(entries)
	}
	if files, err := s.sounds.ListAll(); err == nil {
		data.SoundFiles = files
	}
	engine, voices, note, errMsg := s.resolveTTSBackend(context.Background())
	if errMsg != "" {
		data.TTSError = errMsg
		return
	}
	data.TTSEngine = engine
	data.TTSVoices = voices
	data.TTSNotice = note
	if len(data.TTSVoices) == 0 {
		data.TTSError = "Text-to-speech is unavailable: no voices are currently available"
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

// handleNodeCourtesyToneSave saves the "When each courtesy tone plays"
// card's three fields via config.Store.SetCourtesyToneAssignments — a
// narrow update touching only unlinkedct/remotect/linkunkeyct, not the
// rest of the node. This lives on its own route (rather than folding
// into the Setup tab's whole-node save) specifically so this card can be
// shown on the Tones & Audio tab, right next to the ct1-ct8 entries it
// controls, without a blank Setup-tab field ever being able to wipe it or
// vice versa.
func (s *Server) handleNodeCourtesyToneSave(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	unlinkedCT := strings.TrimSpace(r.FormValue("unlinkedct"))
	remoteCT := strings.TrimSpace(r.FormValue("remotect"))
	linkUnkeyCT := strings.TrimSpace(r.FormValue("linkunkeyct"))
	if err := s.store.SetCourtesyToneAssignments(number, unlinkedCT, remoteCT, linkUnkeyCT); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
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

// handleNodeSoundTTS generates a custom sound file from typed text using
// whichever TTS backend is active (Piper first, espeak-ng fallback), then saves
// it through the exact same sounds.Store.Upload path a manual upload
// uses — synthesis just produces a WAV, everything after that (name
// validation, sox transcoding, landing in the custom directory) is
// identical either way. Voice names are always resolved by listing the
// backend's own available voices first; they are never used to build
// file paths directly from form input.
func (s *Server) handleNodeSoundTTS(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("sound_name"))
	if name == "" {
		s.renderNodeEditPage(w, r, number, flash("error", "Enter a name for this sound file"))
		return
	}
	text := strings.TrimSpace(r.FormValue("tts_text"))
	if text == "" {
		s.renderNodeEditPage(w, r, number, flash("error", "Enter the text to speak"))
		return
	}
	voiceName := strings.TrimSpace(r.FormValue("tts_voice"))
	engine := strings.TrimSpace(r.FormValue("tts_engine"))

	wav, output, userErr, err := s.synthesizeWithEngine(r.Context(), engine, voiceName, text)
	if userErr != "" {
		s.renderNodeEditPage(w, r, number, flash("error", userErr))
		return
	}
	if err != nil {
		msg := formatTTSError("Couldn't generate speech: "+err.Error(), output)
		s.renderNodeEditPage(w, r, number, flash("error", msg))
		return
	}
	if _, err := s.sounds.Upload(r.Context(), name, bytes.NewReader(wav)); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Generated the audio, but couldn't convert it: "+err.Error()))
		return
	}
	s.renderNodeEditPage(w, r, number, flash("ok", "Generated \""+name+"\" from text — pick it from any sound field below."))
}

// handleNodeSoundTTSPreview synthesizes speech for the submitted
// voice+text and returns it directly as WAV bytes — no sox transcoding,
// no save. Both backends return WAV output that's browser-playable as-is,
// so unlike the save path (which needs sox's
// 8kHz mono mu-law for app_rpt) this can return exactly what the backend
// produced. Called from the "Preview" button via fetch(), not a normal
// form submission, so errors go back as a plain text body/status code
// for the page's own JS to show, not a flash + full page re-render.
func (s *Server) handleNodeSoundTTSPreview(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.FormValue("tts_text"))
	if text == "" {
		http.Error(w, "Enter the text to speak", http.StatusBadRequest)
		return
	}
	voiceName := strings.TrimSpace(r.FormValue("tts_voice"))
	engine := strings.TrimSpace(r.FormValue("tts_engine"))

	wav, output, userErr, err := s.synthesizeWithEngine(r.Context(), engine, voiceName, text)
	if userErr != "" {
		http.Error(w, userErr, http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		msg := formatTTSError(err.Error(), output)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Write(wav)
}

// handleNodeSoundAudio streams one of the operator's own custom sounds,
// transcoded on the fly to browser-playable WAV (see
// sounds.Store.Preview), for the "Play" button next to each row in the
// Custom sound files table. Never reachable for a stock library file —
// same boundary as handleNodeSoundDelete.
func (s *Server) handleNodeSoundAudio(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	wav, err := s.sounds.Preview(r.Context(), r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Write(wav)
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
