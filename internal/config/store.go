// Package config maps HamVoIP's Asterisk config files onto small
// domain structs the web layer can render as forms, using
// internal/asteriskconf to read and write without disturbing comments.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"hamvoipconfiggui/internal/asteriskconf"
)

// Store centralizes access to the Asterisk config directory
// (/etc/asterisk on a real HamVoIP node). A single mutex serializes all
// reads/writes across files: config edits are infrequent and small, so
// simplicity wins over per-file locking.
type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name)
}

// load parses a config file relative to the store's directory. Caller
// must hold s.mu.
func (s *Store) load(name string) (*asteriskconf.File, error) {
	f, err := asteriskconf.ParseFile(s.path(name))
	if err != nil {
		return nil, fmt.Errorf("config: load %s: %w", name, err)
	}
	return f, nil
}

// save writes a config file back, preserving its original permissions if
// it already exists (defaulting to 0644 for a brand new file). Caller
// must hold s.mu.
func (s *Store) save(name string, f *asteriskconf.File) error {
	perm := os.FileMode(0644)
	if fi, err := os.Stat(s.path(name)); err == nil {
		perm = fi.Mode().Perm()
	}
	if err := f.WriteFile(s.path(name), perm); err != nil {
		return fmt.Errorf("config: save %s: %w", name, err)
	}
	return nil
}

// RawFile exposes a config file's parsed form for the generic
// section/key editor. Changes made via the returned *asteriskconf.File
// are not persisted until SaveRaw is called with the same name.
func (s *Store) RawFile(name string) (*asteriskconf.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(name)
}

func (s *Store) SaveRaw(name string, f *asteriskconf.File) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(name, f)
}
