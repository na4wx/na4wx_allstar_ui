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

	// onChange, if set, is called after every successful write to a
	// config file — every one of which is an Asterisk/app_rpt file
	// (rpt.conf, iax.conf, extensions.conf, usbradio.conf/simpleusb.conf)
	// that Asterisk only picks up on its next restart. The server package
	// uses this single hook to flag "Asterisk must be restarted" across
	// every page, rather than every save handler having to remember to
	// say so itself.
	onChange func(file string)
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// SetChangeHook installs fn to be called with the file name after every
// successful save (see the onChange field doc). Not safe to call
// concurrently with saves; intended to be set once at startup.
func (s *Store) SetChangeHook(fn func(file string)) {
	s.onChange = fn
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name)
}

// load parses a config file relative to the store's directory. A file
// that doesn't exist yet is treated as a valid, empty config rather
// than an error — HamVoIP nodes commonly have only one of
// usbradio.conf/simpleusb.conf present (never both), and in general
// nothing here should hard-fail a page just because a file it hasn't
// needed yet was never created. The first Save* call against it will
// create the file. Caller must hold s.mu.
func (s *Store) load(name string) (*asteriskconf.File, error) {
	f, err := asteriskconf.ParseFile(s.path(name))
	if os.IsNotExist(err) {
		return &asteriskconf.File{}, nil
	}
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
	if s.onChange != nil {
		s.onChange(name)
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
