package cloudagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings is the operator's own opt-in configuration for connecting
// this node to the public cloud platform: which server to dial, the
// API key that identifies this device, and whether the connection is
// enabled at all. Unlike internal/wxtone or internal/soundschedule
// (which persist a list of entries), there is exactly one Settings
// value per installation, so SettingsStore holds a single JSON object
// rather than an array.
type Settings struct {
	CloudURL string `json:"cloud_url"`
	APIKey   string `json:"api_key"`
	Enabled  bool   `json:"enabled"`
}

// SettingsStore persists Settings as a single JSON file at path, the
// same shape as internal/wxtone.Store and internal/soundschedule.Store
// (a real mutex, since Run's background goroutine reads this
// concurrently with an HTTP-handler write from the local Cloud Sync
// settings page).
type SettingsStore struct {
	path string
	mu   sync.Mutex
}

// NewSettingsStore builds a SettingsStore backed by path.
func NewSettingsStore(path string) *SettingsStore {
	return &SettingsStore{path: path}
}

// Load reads the current settings. A missing file reads as a zeroed,
// disabled Settings rather than an error — this feature is off until an
// operator explicitly opts in, matching how internal/wxtone.Store.List
// treats a missing file as "nothing configured yet".
func (s *SettingsStore) Load() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var out Settings
	if err := json.Unmarshal(data, &out); err != nil {
		return Settings{}, err
	}
	return out, nil
}

// Save writes settings, creating the parent directory if needed.
func (s *SettingsStore) Save(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
