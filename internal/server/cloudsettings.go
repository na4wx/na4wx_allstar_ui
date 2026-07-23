package server

import (
	"net/http"
	"strings"

	"hamvoipconfiggui/internal/cloudagent"
)

// populateSystemCloud fills systemPageData's Cloud Sync fields from the
// operator's own saved settings (see internal/cloudagent.Settings),
// falling back to this server's configured default cloud URL only when
// nothing has been saved yet — so the form isn't blank on first visit,
// but never overwrites what the operator actually chose.
func (s *Server) populateSystemCloud(data *systemPageData) {
	settings, err := s.cloudAgent.Settings().Load()
	if err != nil {
		return
	}
	if settings.CloudURL == "" {
		settings.CloudURL = s.cloudURLDefault
	}
	data.CloudURL = settings.CloudURL
	data.CloudAPIKey = settings.APIKey
	data.CloudEnabled = settings.Enabled
	data.CloudAllowRemoteReboot = settings.AllowRemoteReboot
	data.CloudAllowRawConfigEdit = settings.AllowRawConfigEdit
}

// handleSystemCloudSave saves the Cloud Sync card in one submission —
// the URL, API key, and enabled flag together, matching this app's own
// "select over checkbox for an explicit on/off setting" convention (see
// skywarnToggleKeys's doc comment): an unchecked checkbox submits
// nothing at all, so Enabled is read the same explicit way. Saving
// wakes a currently-waiting Agent.Run loop immediately (see
// cloudagent.Agent.Reload) so turning the feature on, or fixing a bad
// API key, takes effect right away instead of waiting for the next
// backoff/poll tick.
func (s *Server) handleSystemCloudSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	settings := cloudagent.Settings{
		CloudURL:           strings.TrimSpace(r.FormValue("cloud_url")),
		APIKey:             strings.TrimSpace(r.FormValue("cloud_api_key")),
		Enabled:            r.FormValue("cloud_enabled") == "true",
		AllowRemoteReboot:  r.FormValue("cloud_allow_remote_reboot") == "true",
		AllowRawConfigEdit: r.FormValue("cloud_allow_raw_config_edit") == "true",
	}
	if settings.Enabled && (settings.CloudURL == "" || settings.APIKey == "") {
		s.renderSystemPage(w, r, flash("error", "Enter both a cloud URL and an API key before enabling Cloud Sync"))
		return
	}
	if err := s.cloudAgent.Settings().Save(settings); err != nil {
		s.renderSystemPage(w, r, flash("error", err.Error()))
		return
	}
	s.cloudAgent.Reload()
	s.renderSystemPage(w, r, flash("ok", "Cloud Sync settings saved."))
}
