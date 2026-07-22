package config

import (
	"os"
	"path/filepath"
	"testing"
)

const testScheduleConf = `[nodes]
2000 = radio@127.0.0.1:4569/2000,NONE

[schedule]
1 = 00 20 * * 2
2 = 00 06 * * *
`

func newScheduleTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RptConfFile), []byte(testScheduleConf), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return NewStore(dir)
}

func TestListScheduleEntries(t *testing.T) {
	s := newScheduleTestStore(t)
	entries, err := s.ListScheduleEntries("schedule")
	if err != nil {
		t.Fatalf("ListScheduleEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ListScheduleEntries = %v, want 2 entries", entries)
	}
	if entries[0].MacroNum != "1" || entries[0].TimeSpec != "00 20 * * 2" {
		t.Fatalf("entries[0] = %+v", entries[0])
	}
}

func TestListScheduleEntriesEmptySection(t *testing.T) {
	s := newScheduleTestStore(t)
	entries, err := s.ListScheduleEntries("nosuchsection")
	if err != nil {
		t.Fatalf("ListScheduleEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %v", entries)
	}
}

func TestSetScheduleEntryUpdatesExisting(t *testing.T) {
	s := newScheduleTestStore(t)
	if err := s.SetScheduleEntry("schedule", "1", "00 21 * * 2"); err != nil {
		t.Fatalf("SetScheduleEntry: %v", err)
	}
	entries, err := s.ListScheduleEntries("schedule")
	if err != nil {
		t.Fatalf("ListScheduleEntries: %v", err)
	}
	if entries[0].TimeSpec != "00 21 * * 2" {
		t.Fatalf("entries[0].TimeSpec = %q", entries[0].TimeSpec)
	}
	if len(entries) != 2 {
		t.Fatalf("update should not change count, got %d", len(entries))
	}
}

func TestSetScheduleEntryAddsNew(t *testing.T) {
	s := newScheduleTestStore(t)
	if err := s.SetScheduleEntry("schedule", "3", "* * * * *"); err != nil {
		t.Fatalf("SetScheduleEntry: %v", err)
	}
	entries, err := s.ListScheduleEntries("schedule")
	if err != nil {
		t.Fatalf("ListScheduleEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("ListScheduleEntries = %v, want 3 entries", entries)
	}
	if entries[2].MacroNum != "3" || entries[2].TimeSpec != "* * * * *" {
		t.Fatalf("new entry = %+v", entries[2])
	}
}

func TestSetScheduleEntryCreatesSection(t *testing.T) {
	s := newScheduleTestStore(t)
	if err := s.SetScheduleEntry("schedule2000", "1", "00 20 * * 2"); err != nil {
		t.Fatalf("SetScheduleEntry: %v", err)
	}
	entries, err := s.ListScheduleEntries("schedule2000")
	if err != nil {
		t.Fatalf("ListScheduleEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListScheduleEntries = %v, want 1 entry", entries)
	}
	// original [schedule] section must be untouched
	orig, err := s.ListScheduleEntries("schedule")
	if err != nil {
		t.Fatalf("ListScheduleEntries(schedule): %v", err)
	}
	if len(orig) != 2 {
		t.Fatalf("original schedule section changed: %v", orig)
	}
}

func TestDeleteScheduleEntry(t *testing.T) {
	s := newScheduleTestStore(t)
	if err := s.DeleteScheduleEntry("schedule", "2"); err != nil {
		t.Fatalf("DeleteScheduleEntry: %v", err)
	}
	entries, err := s.ListScheduleEntries("schedule")
	if err != nil {
		t.Fatalf("ListScheduleEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListScheduleEntries = %v, want 1 entry", entries)
	}
	for _, e := range entries {
		if e.MacroNum == "2" {
			t.Fatalf("entry 2 should have been deleted, still present: %+v", entries)
		}
	}
}
