package server

import (
	"os"
	"path/filepath"
	"testing"

	"hamvoipconfiggui/internal/config"
)

func TestAutomationActionByKey(t *testing.T) {
	a, ok := automationActionByKey("connect_stay")
	if !ok || a.Command != "ilink,3" || !a.NeedsTarget {
		t.Fatalf("connect_stay = %+v, %v", a, ok)
	}
	if _, ok := automationActionByKey("nope"); ok {
		t.Fatal("expected no match for unknown key")
	}
}

func TestAutomationActionByCommand(t *testing.T) {
	a, ok := automationActionByCommand("ilink,6")
	if !ok || a.Key != "disconnect_all" || a.NeedsTarget {
		t.Fatalf("ilink,6 = %+v, %v", a, ok)
	}
	if _, ok := automationActionByCommand("cop,6"); ok {
		t.Fatal("expected no match for an unscoped command")
	}
}

func TestAllocateDigitSkipsUsedAndStartsAt900(t *testing.T) {
	entries := []config.FunctionMacro{{Digits: "1", Command: "ilink,1"}, {Digits: "900", Command: "localplay,x"}}
	got := allocateDigit(entries)
	if got != "901" {
		t.Fatalf("allocateDigit = %q, want 901", got)
	}
}

func TestAllocateDigitEmpty(t *testing.T) {
	if got := allocateDigit(nil); got != "900" {
		t.Fatalf("allocateDigit(nil) = %q, want 900", got)
	}
}

func TestAllocateMacroNumberFillsGapAndSkipsZero(t *testing.T) {
	entries := []config.FunctionMacro{{Digits: "0", Command: "startup stuff"}, {Digits: "1", Command: "*81"}, {Digits: "3", Command: "*32000"}}
	got := allocateMacroNumber(entries)
	if got != "2" {
		t.Fatalf("allocateMacroNumber = %q, want 2 (0 reserved, 1 and 3 used)", got)
	}
}

func TestAllocateMacroNumberEmptyNeverZero(t *testing.T) {
	if got := allocateMacroNumber(nil); got != "1" {
		t.Fatalf("allocateMacroNumber(nil) = %q, want 1", got)
	}
}

func TestBuildAutomationDTMF(t *testing.T) {
	if got := buildAutomationDTMF("3", "2000", true); got != "*32000" {
		t.Fatalf("buildAutomationDTMF with target = %q, want *32000", got)
	}
	if got := buildAutomationDTMF("76", "", false); got != "*76" {
		t.Fatalf("buildAutomationDTMF without target = %q, want *76", got)
	}
}

// TestParseAutomationMacroRealDigitMapping uses the exact digit->command
// mapping confirmed present in config.standard_commands.go — the crux of
// this reverse-parse: it must resolve through the node's real functions
// table, not assume digit-equals-ilink-number (digit "76" -> ilink,6 is
// the case that assumption gets wrong).
func TestParseAutomationMacroRealDigitMapping(t *testing.T) {
	functionsEntries := []config.FunctionMacro{
		{Digits: "1", Command: "ilink,1"},
		{Digits: "2", Command: "ilink,2"},
		{Digits: "3", Command: "ilink,3"},
		{Digits: "76", Command: "ilink,6"},
	}

	label, ok := parseAutomationMacro("*32000", functionsEntries)
	if !ok || label != "Connect (stay connected) 2000" {
		t.Fatalf("*32000 -> %q, %v", label, ok)
	}

	label, ok = parseAutomationMacro("*76", functionsEntries)
	if !ok || label != "Disconnect all" {
		t.Fatalf("*76 -> %q, %v", label, ok)
	}

	label, ok = parseAutomationMacro("*12000", functionsEntries)
	if !ok || label != "Disconnect a specific node 2000" {
		t.Fatalf("*12000 -> %q, %v", label, ok)
	}
}

// TestParseAutomationMacroLongestPrefixWins constructs the genuine
// ambiguity the longest-first sort exists for: digit "7" (a scoped
// action, disconnect-one) is itself a prefix of digit "76" (also
// scoped, disconnect-all). "*76" must resolve as disconnect-all (digit
// "76" consuming the whole string), not as digit "7" plus a leftover
// "6" target for disconnect-one.
func TestParseAutomationMacroLongestPrefixWins(t *testing.T) {
	functionsEntries := []config.FunctionMacro{
		{Digits: "7", Command: "ilink,1"},
		{Digits: "76", Command: "ilink,6"},
	}
	label, ok := parseAutomationMacro("*76", functionsEntries)
	if !ok || label != "Disconnect all" {
		t.Fatalf("*76 -> %q, %v, want \"Disconnect all\" via the longer digit 76", label, ok)
	}
}

func TestParseAutomationMacroUnrecognizedFallsBack(t *testing.T) {
	functionsEntries := []config.FunctionMacro{{Digits: "99", Command: "cop,6"}}
	if _, ok := parseAutomationMacro("*81 *80#", functionsEntries); ok {
		t.Fatal("a hand-authored macro unrelated to automation must not be recognized")
	}
	if _, ok := parseAutomationMacro("not dtmf at all", functionsEntries); ok {
		t.Fatal("a non-DTMF value must not be recognized")
	}
}

func newAutomationTestStore(t *testing.T) *config.Store {
	t.Helper()
	dir := t.TempDir()
	fixture := `[functions2000]
1=ilink,1

[2000]
rxchannel = SimpleUSB/usb
duplex = 4
functions = functions2000
`
	if err := os.WriteFile(filepath.Join(dir, config.RptConfFile), []byte(fixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return config.NewStore(dir)
}

func TestEnsureFunctionDigitReusesExisting(t *testing.T) {
	store := newAutomationTestStore(t)
	digit, err := ensureFunctionDigit(store, "functions2000", "ilink,1")
	if err != nil {
		t.Fatal(err)
	}
	if digit != "1" {
		t.Fatalf("digit = %q, want 1 (reuse the existing mapping)", digit)
	}
	entries, err := store.ListFunctionMacros("functions2000")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("ensureFunctionDigit should not have created a new entry, got %v", entries)
	}
}

func TestEnsureFunctionDigitAllocatesWhenMissing(t *testing.T) {
	store := newAutomationTestStore(t)
	digit, err := ensureFunctionDigit(store, "functions2000", "ilink,3")
	if err != nil {
		t.Fatal(err)
	}
	if digit != "900" {
		t.Fatalf("digit = %q, want 900 (allocated)", digit)
	}
	entries, err := store.ListFunctionMacros("functions2000")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Digits == "900" && e.Command == "ilink,3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a new 900=ilink,3 entry, got %v", entries)
	}
}
