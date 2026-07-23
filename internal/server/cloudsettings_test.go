package server

import (
	"path/filepath"
	"testing"

	"hamvoipconfiggui/internal/cloudagent"
	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/sounds"
	"hamvoipconfiggui/internal/soundschedule"
	"hamvoipconfiggui/internal/wxtone"
)

func newCloudTestServer(t *testing.T, cloudURLDefault string) *Server {
	t.Helper()
	settingsPath := filepath.Join(t.TempDir(), "cloud-agent.json")
	store := config.NewStore(t.TempDir())
	agent := cloudagent.New(
		settingsPath, store, "asterisk",
		sounds.New(t.TempDir(), t.TempDir(), "sox"),
		soundschedule.New(filepath.Join(t.TempDir(), "sound-schedule.json")),
		wxtone.New(filepath.Join(t.TempDir(), "wx-tones.json")),
		"", "818-prog", filepath.Join(t.TempDir(), "sa818-last.json"),
		"",
	)
	return &Server{
		store:           store,
		cloudAgent:      agent,
		cloudURLDefault: cloudURLDefault,
	}
}

// TestPopulateSystemCloudUsesDefaultWhenUnconfigured covers a fresh
// install: nothing has been saved yet, so the form should pre-fill with
// this server's configured default cloud URL (see -cloud-url in
// main.go) rather than showing a blank field, while Enabled/APIKey stay
// their honest zero values.
func TestPopulateSystemCloudUsesDefaultWhenUnconfigured(t *testing.T) {
	s := newCloudTestServer(t, "wss://cloud.example.com/agent")
	var data systemPageData
	s.populateSystemCloud(&data)

	if data.CloudURL != "wss://cloud.example.com/agent" {
		t.Errorf("CloudURL = %q, want the configured default", data.CloudURL)
	}
	if data.CloudEnabled {
		t.Error("CloudEnabled = true, want false for an unconfigured install")
	}
	if data.CloudAPIKey != "" {
		t.Errorf("CloudAPIKey = %q, want empty", data.CloudAPIKey)
	}
}

// TestPopulateSystemCloudPrefersSavedURLOverDefault confirms an
// operator's own saved URL is never silently overwritten by this
// server's default once something real has been saved.
func TestPopulateSystemCloudPrefersSavedURLOverDefault(t *testing.T) {
	s := newCloudTestServer(t, "wss://default.example.com/agent")
	if err := s.cloudAgent.Settings().Save(cloudagent.Settings{
		CloudURL: "wss://operators-own-cloud.example.com/agent",
		APIKey:   "hvc_live_abc123",
		Enabled:  true,
	}); err != nil {
		t.Fatal(err)
	}

	var data systemPageData
	s.populateSystemCloud(&data)

	if data.CloudURL != "wss://operators-own-cloud.example.com/agent" {
		t.Errorf("CloudURL = %q, want the operator's saved URL, not the default", data.CloudURL)
	}
	if !data.CloudEnabled {
		t.Error("CloudEnabled = false, want true")
	}
	if data.CloudAPIKey != "hvc_live_abc123" {
		t.Errorf("CloudAPIKey = %q, want the saved key", data.CloudAPIKey)
	}
}
