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
		settingsPath, cloudURLDefault, store, "asterisk",
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
// install: nothing has been saved yet, so the Cloud Sync card should
// show this server's fixed cloud URL (see -cloud-url in main.go)
// rather than a blank field, while Enabled/APIKey stay their honest
// zero values.
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

// TestPopulateSystemCloudAlwaysUsesFixedURL confirms the Cloud URL
// shown is always this server's fixed cloudURLDefault, never anything
// that might be sitting in the settings file on disk (e.g. from a
// hand-edited file, or a value saved before CloudURL stopped being
// persisted at all) -- the cloud address is baked in at build/deploy
// time, not operator-editable.
func TestPopulateSystemCloudAlwaysUsesFixedURL(t *testing.T) {
	s := newCloudTestServer(t, "wss://fixed.example.com/agent")
	if err := s.cloudAgent.Settings().Save(cloudagent.Settings{
		CloudURL: "wss://should-be-ignored.example.com/agent",
		APIKey:   "hvc_live_abc123",
		Enabled:  true,
	}); err != nil {
		t.Fatal(err)
	}

	var data systemPageData
	s.populateSystemCloud(&data)

	if data.CloudURL != "wss://fixed.example.com/agent" {
		t.Errorf("CloudURL = %q, want the server's fixed default regardless of what was saved", data.CloudURL)
	}
	if !data.CloudEnabled {
		t.Error("CloudEnabled = false, want true")
	}
	if data.CloudAPIKey != "hvc_live_abc123" {
		t.Errorf("CloudAPIKey = %q, want the saved key", data.CloudAPIKey)
	}
}

// TestPopulateSystemCloudLastConnectedNeverConnected covers a fresh
// process that hasn't completed a hello handshake yet -- the Cloud Sync
// card's liveness line should read "never", not a zero-value timestamp
// or an empty string.
func TestPopulateSystemCloudLastConnectedNeverConnected(t *testing.T) {
	s := newCloudTestServer(t, "wss://cloud.example.com/agent")
	var data systemPageData
	s.populateSystemCloud(&data)

	if data.CloudLastConnected != "never" {
		t.Errorf("CloudLastConnected = %q, want \"never\"", data.CloudLastConnected)
	}
}
