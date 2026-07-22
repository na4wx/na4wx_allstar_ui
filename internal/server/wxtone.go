package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/skywarnplus"
	"hamvoipconfiggui/internal/wxtone"
)

// wxTonePollInterval checks roughly twice per SkywarnPlus cron cycle
// (that cron runs every 60s — see install.sh's SkywarnPlus section), so
// a courtesy tone reacts to an alert starting or clearing within about a
// minute, without polling SkywarnPlus's own state file needlessly often.
const wxTonePollInterval = 30 * time.Second

// StartWXTonePoller checks on wxTonePollInterval whether SkywarnPlus
// currently has any active alert and swaps each configured wxtone.Entry's
// courtesy tone accordingly — the safer, fully-visible alternative to
// SkywarnPlus's own courtesy-tone swap (see internal/wxtone's package
// doc for why this never touches rpt.conf itself, only the underlying
// sound file's bytes). Runs until ctx is cancelled. Call once, from main.
func (s *Server) StartWXTonePoller(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(wxTonePollInterval)
		defer ticker.Stop()
		s.checkWXTones(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkWXTones(ctx)
			}
		}
	}()
}

// checkWXTones reads SkywarnPlus's current active-alert count once (not
// per entry — it's one global count, not per-node, since SkywarnPlus
// itself tracks one set of county codes regardless of which nodes are
// registered to announce), then brings every configured entry's
// courtesy tone in line with the desired mode. Every step is
// best-effort: a problem with one entry (a missing node, a ctX key that
// no longer resolves to a custom sound file, a copy failure) is logged
// and skipped, never fatal to the ticker or to any other entry.
func (s *Server) checkWXTones(ctx context.Context) {
	if !skywarnplus.IsInstalled(s.skywarnDir) {
		return
	}
	entries, err := s.wxTones.List()
	if err != nil || len(entries) == 0 {
		return
	}
	status, err := skywarnplus.GetStatus(ctx, s.skywarnDir)
	if err != nil {
		log.Printf("wxtone: couldn't read SkywarnPlus status: %v", err)
		return
	}
	desired := wxtone.ModeNormal
	if status.ActiveAlertCount > 0 {
		desired = wxtone.ModeWX
	}
	for _, e := range entries {
		if e.Mode == desired {
			continue
		}
		if err := s.applyWXTone(e, desired); err != nil {
			log.Printf("wxtone: node %s %s: %v", e.Node, e.CTKey, err)
			continue
		}
		if err := s.wxTones.SetMode(e.ID, desired); err != nil {
			log.Printf("wxtone: node %s %s: recording new mode: %v", e.Node, e.CTKey, err)
		}
	}
}

// resolveCTDestPath finds the real on-disk file e.CTKey's current
// rpt.conf value points at, confirming it's actually one of this app's
// own custom sound files (via an exact Ref match against
// sounds.ListCustom(), not path-string inspection) — never the stock
// library (read-only) and never an arbitrary path some other config
// tool might have put there. This is the same check at both save time
// (the handler below) and apply time (here), since the operator could
// have changed ctX's value on the Tones & Audio tab after configuring a
// mapping.
func (s *Server) resolveCTDestPath(node *config.Node, ctKey string) (string, error) {
	section := node.Telemetry
	if section == "" {
		section = "telemetry"
	}
	telemetryEntries, err := s.store.ListTelemetryEntries(section)
	if err != nil {
		return "", fmt.Errorf("read telemetry section: %w", err)
	}
	var value string
	found := false
	for _, te := range telemetryEntries {
		if te.Key == ctKey {
			value, found = te.Value, true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("%s is no longer present in this node's telemetry section", ctKey)
	}
	if _, ok := config.ParseSingleTone(value); ok {
		return "", fmt.Errorf("%s is a tone-generator value, not a sound file — pick a custom sound file for it on the Tones & Audio tab first", ctKey)
	}
	if config.IsToneValue(value) {
		return "", fmt.Errorf("%s is a tone-generator value, not a sound file — pick a custom sound file for it on the Tones & Audio tab first", ctKey)
	}
	customFiles, err := s.sounds.ListCustom()
	if err != nil {
		return "", fmt.Errorf("list custom sounds: %w", err)
	}
	for _, f := range customFiles {
		if f.Ref == value {
			return s.sounds.ResolveCustomPath(f.Name)
		}
	}
	return "", fmt.Errorf("%s isn't set to one of this app's own custom sound files (it may point at the read-only stock library, or a file managed by Raw Config)", ctKey)
}

// resolveSoundSourcePath finds name's real on-disk file, trying the
// custom directory first (the common case — most operators will pick
// their own recordings) and falling back to the stock library, matching
// how every other sound picker in this app offers custom+stock together.
func (s *Server) resolveSoundSourcePath(name string) (string, error) {
	if path, err := s.sounds.ResolveCustomPath(name); err == nil {
		return path, nil
	}
	return s.sounds.ResolveStockPath(name)
}

// applyWXTone copies whichever sound (e.NormalSound or e.WXSound,
// depending on desired) onto e.CTKey's existing destination file —
// never touching rpt.conf itself, so this takes effect immediately with
// no Asterisk restart, the same technique SkywarnPlus's own SkyControl.py
// uses for its courtesy-tone swap.
func (s *Server) applyWXTone(e wxtone.Entry, desired string) error {
	node, err := s.store.LoadNode(e.Node)
	if err != nil {
		return fmt.Errorf("load node: %w", err)
	}
	destPath, err := s.resolveCTDestPath(node, e.CTKey)
	if err != nil {
		return err
	}
	sourceName := e.NormalSound
	if desired == wxtone.ModeWX {
		sourceName = e.WXSound
	}
	srcPath, err := s.resolveSoundSourcePath(sourceName)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", sourceName, err)
	}
	if srcPath == destPath {
		return nil // already identical on disk, nothing to copy
	}
	return copyFileContents(srcPath, destPath)
}

// copyFileContents overwrites dest's content with src's, matching
// SkyControl.py's own shutil.copyfile semantics (dest keeps its own
// filename/extension; only its bytes change) — a temp-file-plus-rename
// would be safer against a mid-copy crash, but app_rpt could be reading
// this exact file at the moment of a swap, so replacing it in place
// (same inode, same open-file-handle semantics Asterisk already expects
// for a file it might have open) is closer to what this app's other
// direct-write paths already assume.
func copyFileContents(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// populateNodeWXTones fills data's "WX courtesy tone" rows — best-effort
// like the rest of this page's supplementary data, and only meaningful
// once SkywarnPlus is installed (see populateNodeSkywarn).
func (s *Server) populateNodeWXTones(data *nodeFormData) {
	if data.Node == nil || data.Node.Number == "" || !data.SkywarnInstalled {
		return
	}
	entries, err := s.wxTones.ListForNode(data.Node.Number)
	if err != nil {
		return
	}
	data.WXTones = entries
}

// handleNodeWXToneSave adds or updates one alert-driven courtesy-tone
// mapping, rejecting a CTKey that doesn't currently resolve to one of
// this app's own custom sound files (see resolveCTDestPath) rather than
// silently accepting a mapping that could never actually apply.
func (s *Server) handleNodeWXToneSave(w http.ResponseWriter, r *http.Request) {
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
	ctKey := r.FormValue("wxtone_ctkey")
	normalSound := r.FormValue("wxtone_normal")
	wxSound := r.FormValue("wxtone_wx")
	if ctKey == "" || normalSound == "" || wxSound == "" {
		s.renderNodeEditPage(w, r, number, flash("error", "Pick a courtesy tone key and both a normal and WX sound"))
		return
	}
	if _, err := s.resolveCTDestPath(node, ctKey); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	if _, err := s.resolveSoundSourcePath(normalSound); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Normal sound: "+err.Error()))
		return
	}
	if _, err := s.resolveSoundSourcePath(wxSound); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "WX sound: "+err.Error()))
		return
	}
	entry := wxtone.Entry{Node: number, CTKey: ctKey, NormalSound: normalSound, WXSound: wxSound}
	if err := s.wxTones.Save(entry); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeWXToneDelete removes one alert-driven courtesy-tone mapping.
// Deliberately does not restore the courtesy tone's file to any
// particular state on removal — whatever is currently applied (normal or
// wx) simply stops being managed, same as this app leaves any other
// manually-set value alone once a feature stops touching it.
func (s *Server) handleNodeWXToneDelete(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	id := r.PathValue("id")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.wxTones.Delete(id); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}
