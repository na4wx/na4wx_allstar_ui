// Package wxtone holds the operator's own alert-driven courtesy-tone
// mappings — a safer, fully-visible alternative to SkywarnPlus's own
// courtesy-tone/ID swap (see internal/skywarnplus's package doc for why
// that one stays out of scope): rather than a second, uncoordinated
// process silently overwriting a sound file, this app's own ticker (see
// internal/server's StartWXTonePoller) reads SkywarnPlus's own
// already-fetched active-alert count and performs the same kind of file
// swap itself, fully tracked here.
//
// Deliberately never touches rpt.conf's own ctX= value — every entry's
// CTKey must already point at a custom sound file (validated at save
// time), and only that file's on-disk bytes are ever swapped, matching
// SkyControl.py's own changeCT technique. Changing rpt.conf itself would
// require an Asterisk restart to take effect in this app (see
// internal/server's restartNeeded flag), which is far too disruptive to
// trigger automatically every time a weather alert starts or clears.
package wxtone

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Mode values track which sound is currently applied to an entry's
// CTKey destination file, so the poller only copies a file when the
// desired mode actually differs from what's already there.
const (
	ModeNormal = "normal"
	ModeWX     = "wx"
)

// Entry is one alert-driven courtesy-tone mapping.
type Entry struct {
	ID    string `json:"id"`
	Node  string `json:"node"`
	CTKey string `json:"ct_key"` // e.g. "ct1" -- one of this node's existing courtesy-tone keys

	// NormalSound/WXSound are internal/sounds File.Name values (custom or
	// stock), the sources copied onto CTKey's own destination file
	// depending on whether SkywarnPlus currently has an active alert.
	NormalSound string `json:"normal_sound"`
	WXSound     string `json:"wx_sound"`

	// Mode is which sound is currently applied -- ModeNormal or ModeWX,
	// defaulting to ModeNormal for a freshly-created entry (matching
	// SkywarnPlus's own "initialize to normal" first-run behavior).
	Mode string `json:"mode"`
}

// Store persists Entry records as a single JSON file at path, the same
// shape as internal/soundschedule.Store (a real mutex, since a
// background ticker reads this concurrently with HTTP-handler writes).
type Store struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) load() ([]Entry, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) save(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

// List returns every configured mapping, across all nodes.
func (s *Store) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// ListForNode returns the configured mappings for one node.
func (s *Store) ListForNode(node string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range entries {
		if e.Node == node {
			out = append(out, e)
		}
	}
	return out, nil
}

// Save adds e as a new entry (blank ID -- a fresh one is generated, Mode
// defaulting to ModeNormal) or updates an existing entry with a matching
// ID.
func (s *Store) Save(e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	if e.ID == "" {
		id, err := newID()
		if err != nil {
			return err
		}
		e.ID = id
		if e.Mode == "" {
			e.Mode = ModeNormal
		}
		entries = append(entries, e)
		return s.save(entries)
	}
	for i := range entries {
		if entries[i].ID == e.ID {
			entries[i] = e
			return s.save(entries)
		}
	}
	entries = append(entries, e)
	return s.save(entries)
}

// SetMode updates just the last-applied mode for one entry, used by the
// poller after it successfully swaps a file -- avoids a full
// read-modify-write of every field for what's otherwise a pure read-only
// mapping from the operator's point of view.
func (s *Store) SetMode(id, mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].ID == id {
			entries[i].Mode = mode
			return s.save(entries)
		}
	}
	return nil
}

// Delete removes one entry by ID. Deleting an ID that doesn't exist is
// not an error.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	out := entries[:0]
	for _, e := range entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	return s.save(out)
}

// DeleteByNode removes every entry for node -- used to clean up when a
// node itself is deleted, since these entries live outside rpt.conf and
// nothing else knows to remove them.
func (s *Store) DeleteByNode(node string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	out := entries[:0]
	for _, e := range entries {
		if e.Node != node {
			out = append(out, e)
		}
	}
	return s.save(out)
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
