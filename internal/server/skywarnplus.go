package server

import (
	"context"
	"net/http"
	"strings"

	"hamvoipconfiggui/internal/skywarnplus"
)

// skywarnToggleKeys lists the boolean features exposed on the Weather
// Alerts card, saved together from one form/button — matching this
// app's own "select over checkbox for an explicit on/off setting"
// convention (an unchecked checkbox submits nothing at all, which is
// exactly the kind of ambiguity this app avoids elsewhere) rather than
// SkyControl.py's own one-key-per-invocation shape.
var skywarnToggleKeys = []string{"enable", "sayalert", "sayallclear", "tailmessage"}

// populateNodeSkywarn fills data's "Weather Alerts" fields from an
// operator-installed copy of SkywarnPlus, if there is one — see
// internal/skywarnplus's package doc for why installation itself is
// install.sh's job, not this app's. Best-effort like the rest of this
// page's supplementary data: any read failure just leaves the section
// looking not-installed rather than failing the whole page. The county
// picker's option list is populated regardless of whether SkywarnPlus is
// installed, since it's just this app's own bundled reference data.
func (s *Server) populateNodeSkywarn(ctx context.Context, data *nodeFormData) {
	data.CountyCodeOptions = skywarnplus.ListCounties()
	if !skywarnplus.IsInstalled(s.skywarnDir) {
		return
	}
	data.SkywarnInstalled = true
	status, err := skywarnplus.GetStatus(ctx, s.skywarnDir)
	if err != nil {
		return
	}
	data.SkywarnStatus = status
	if data.Node == nil {
		return
	}
	for _, n := range status.Nodes {
		if n == data.Node.Number {
			data.SkywarnNodeRegistered = true
			break
		}
	}
}

// handleNodeSkywarnToggle saves every boolean feature in one submission
// (see skywarnToggleKeys), each via SkywarnPlus's own SkyControl.py — a
// failure on one key doesn't stop the others from being attempted, and
// every failure is reported together, matching handleNodeDelete's own
// "attempt everything, report all failures together" pattern.
func (s *Server) handleNodeSkywarnToggle(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	var failed []string
	for _, key := range skywarnToggleKeys {
		value := r.FormValue(key) == "true"
		if _, err := skywarnplus.SetToggle(r.Context(), s.skywarnDir, key, value); err != nil {
			failed = append(failed, key+": "+err.Error())
		}
	}
	if len(failed) > 0 {
		s.renderNodeEditPage(w, r, number, flash("error", "Some settings couldn't be saved: "+strings.Join(failed, "; ")))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeSkywarnAddCounty adds one county code to SkywarnPlus's list.
// SetCounties always replaces the whole list (sky_configure.py has no
// single-item-append for counties, unlike AddNode for the node list), so
// this reads the current list first and appends to it — a no-op if the
// code's already present, rather than a duplicate entry.
func (s *Server) handleNodeSkywarnAddCounty(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	if code == "" {
		s.renderNodeEditPage(w, r, number, flash("error", "Pick a county to add"))
		return
	}
	status, err := skywarnplus.GetStatus(r.Context(), s.skywarnDir)
	if err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Couldn't read SkywarnPlus's current settings: "+err.Error()))
		return
	}
	codes := status.CountyCodes
	already := false
	for _, c := range codes {
		if c == code {
			already = true
			break
		}
	}
	if !already {
		codes = append(codes, code)
	}
	if _, err := skywarnplus.SetCounties(r.Context(), s.skywarnDir, codes); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Couldn't add county: "+err.Error()))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeSkywarnDeleteCounty removes one county code, the same
// read-modify-replace-whole-list way handleNodeSkywarnAddCounty adds one.
func (s *Server) handleNodeSkywarnDeleteCounty(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	code := r.PathValue("code")
	status, err := skywarnplus.GetStatus(r.Context(), s.skywarnDir)
	if err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Couldn't read SkywarnPlus's current settings: "+err.Error()))
		return
	}
	remaining := make([]string, 0, len(status.CountyCodes))
	for _, c := range status.CountyCodes {
		if c != code {
			remaining = append(remaining, c)
		}
	}
	if _, err := skywarnplus.SetCounties(r.Context(), s.skywarnDir, remaining); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Couldn't remove county: "+err.Error()))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeSkywarnRegister adds this node's own number to SkywarnPlus's
// broadcast list — idempotent (sky_configure.py's add-node is a no-op if
// already present).
func (s *Server) handleNodeSkywarnRegister(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := skywarnplus.AddNode(r.Context(), s.skywarnDir, number); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", "Couldn't register this node with SkywarnPlus: "+err.Error()))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}
