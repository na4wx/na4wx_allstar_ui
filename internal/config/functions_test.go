package config

import (
	"os"
	"path/filepath"
	"testing"
)

const testFunctionsConf = `[nodes]
2000 = radio@127.0.0.1:4569/2000,NONE

[functions]
1 = ilink,3
2 = ilink,2
3 = ilink,1
70 = ilink,6
80 = status,1
`

func newFunctionsTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RptConfFile), []byte(testFunctionsConf), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return NewStore(dir)
}

func TestListFunctionMacros(t *testing.T) {
	s := newFunctionsTestStore(t)
	macros, err := s.ListFunctionMacros("functions")
	if err != nil {
		t.Fatalf("ListFunctionMacros: %v", err)
	}
	if len(macros) != 5 {
		t.Fatalf("ListFunctionMacros = %v, want 5 entries", macros)
	}
	if macros[0].Digits != "1" || macros[0].Command != "ilink,3" {
		t.Fatalf("macros[0] = %+v", macros[0])
	}
}

func TestListFunctionMacrosEmptySection(t *testing.T) {
	s := newFunctionsTestStore(t)
	macros, err := s.ListFunctionMacros("nosuchsection")
	if err != nil {
		t.Fatalf("ListFunctionMacros: %v", err)
	}
	if len(macros) != 0 {
		t.Fatalf("expected no macros, got %v", macros)
	}
}

func TestSetFunctionMacroUpdatesExisting(t *testing.T) {
	s := newFunctionsTestStore(t)
	if err := s.SetFunctionMacro("functions", "1", "ilink,3,2000"); err != nil {
		t.Fatalf("SetFunctionMacro: %v", err)
	}
	macros, err := s.ListFunctionMacros("functions")
	if err != nil {
		t.Fatalf("ListFunctionMacros: %v", err)
	}
	if macros[0].Command != "ilink,3,2000" {
		t.Fatalf("macros[0].Command = %q", macros[0].Command)
	}
	if len(macros) != 5 {
		t.Fatalf("update should not change count, got %d", len(macros))
	}
}

func TestSetFunctionMacroAddsNew(t *testing.T) {
	s := newFunctionsTestStore(t)
	if err := s.SetFunctionMacro("functions", "99", "ilink,0"); err != nil {
		t.Fatalf("SetFunctionMacro: %v", err)
	}
	macros, err := s.ListFunctionMacros("functions")
	if err != nil {
		t.Fatalf("ListFunctionMacros: %v", err)
	}
	if len(macros) != 6 {
		t.Fatalf("ListFunctionMacros = %v, want 6 entries", macros)
	}
	if macros[5].Digits != "99" || macros[5].Command != "ilink,0" {
		t.Fatalf("new macro = %+v", macros[5])
	}
}

func TestSetFunctionMacroCreatesSection(t *testing.T) {
	s := newFunctionsTestStore(t)
	if err := s.SetFunctionMacro("myfunctions", "1", "ilink,3"); err != nil {
		t.Fatalf("SetFunctionMacro: %v", err)
	}
	macros, err := s.ListFunctionMacros("myfunctions")
	if err != nil {
		t.Fatalf("ListFunctionMacros: %v", err)
	}
	if len(macros) != 1 {
		t.Fatalf("ListFunctionMacros = %v, want 1 entry", macros)
	}
	// original [functions] section must be untouched
	orig, err := s.ListFunctionMacros("functions")
	if err != nil {
		t.Fatalf("ListFunctionMacros(functions): %v", err)
	}
	if len(orig) != 5 {
		t.Fatalf("original functions section changed: %v", orig)
	}
}

func TestDeleteFunctionMacro(t *testing.T) {
	s := newFunctionsTestStore(t)
	if err := s.DeleteFunctionMacro("functions", "70"); err != nil {
		t.Fatalf("DeleteFunctionMacro: %v", err)
	}
	macros, err := s.ListFunctionMacros("functions")
	if err != nil {
		t.Fatalf("ListFunctionMacros: %v", err)
	}
	if len(macros) != 4 {
		t.Fatalf("ListFunctionMacros = %v, want 4 entries", macros)
	}
	for _, m := range macros {
		if m.Digits == "70" {
			t.Fatalf("macro 70 should have been deleted, still present: %+v", macros)
		}
	}
}
