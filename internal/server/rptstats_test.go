package server

import "testing"

// realRptStats is verbatim output from the real node (Asterisk
// 1.4.23-pre.hamvoip-V1.7.1, app_rpt-0.327), captured rather than
// invented.
const realRptStats = `************************ NODE 52829 STATISTICS *************************

Selected system state............................: 0
Signal on input..................................: NO
System...........................................: ENABLED
Parrot Mode......................................: DISABLED
Scheduler........................................: ENABLED
Tail Time........................................: STANDARD
Time out timer...................................: ENABLED
Incoming connections.............................: ENABLED
Time out timer state.............................: RESET
Time outs since system initialization............: 0
Identifier state.................................: CLEAN
Kerchunks today..................................: 0
Kerchunks since system initialization............: 3
Keyups today.....................................: 0
Keyups since system initialization...............: 16
DTMF commands today..............................: 0
DTMF commands since system initialization........: 5
Last DTMF command executed.......................: 150001
TX time today....................................: 00:00:00.0
TX time since system initialization..............: 00:02:26.702
Uptime...........................................: 03:25:47
Nodes currently connected to us..................: <NONE>
Autopatch........................................: ENABLED
Autopatch state..................................: DOWN
Autopatch called number..........................: N/A
Reverse patch/IAXRPT connected...................: DOWN
User linking commands............................: ENABLED
User functions...................................: ENABLED
`

func TestParseRptStatsRealOutput(t *testing.T) {
	fields, ok := parseRptStats(realRptStats)
	if !ok {
		t.Fatal("expected the stats block to parse")
	}
	if len(fields) != 28 {
		t.Errorf("got %d fields, want 28", len(fields))
	}
	// The banner must not become a field.
	for _, f := range fields {
		if f.Label == "" || f.Label[0] == '*' {
			t.Errorf("banner leaked into fields: %+v", f)
		}
	}
	for _, c := range []struct{ label, want string }{
		{"Signal on input", "NO"},
		{"System", "ENABLED"},
		{"Nodes currently connected to us", "<NONE>"},
		{"Last DTMF command executed", "150001"},
		{"Keyups since system initialization", "16"},
	} {
		if got := fields.Value(c.label); got != c.want {
			t.Errorf("Value(%q) = %q, want %q", c.label, got, c.want)
		}
	}
}

// TestParseRptStatsTimeValues is why the split is at the FIRST colon:
// these values contain colons of their own, and splitting at the last
// one (or on every colon) would truncate them.
func TestParseRptStatsTimeValues(t *testing.T) {
	fields, _ := parseRptStats(realRptStats)
	for _, c := range []struct{ label, want string }{
		{"TX time today", "00:00:00.0"},
		{"TX time since system initialization", "00:02:26.702"},
		{"Uptime", "03:25:47"},
	} {
		if got := fields.Value(c.label); got != c.want {
			t.Errorf("Value(%q) = %q, want %q", c.label, got, c.want)
		}
	}
}

func TestParseRptStatsLabelExactMatch(t *testing.T) {
	fields, _ := parseRptStats(realRptStats)
	if got := fields.Value("Signal"); got != "" {
		t.Errorf("partial label matched: %q", got)
	}
	if got := fields.Value("nope"); got != "" {
		t.Errorf("unknown label returned %q", got)
	}
}

func TestParseRptStatsUnrecognized(t *testing.T) {
	if _, ok := parseRptStats("no colons here\njust text\n"); ok {
		t.Error("expected ok=false when nothing parses")
	}
}

// TestNodeReceiving covers the one field this section exists to
// surface: whether someone is keying the node's receiver right now.
func TestNodeReceiving(t *testing.T) {
	fields, _ := parseRptStats(realRptStats)
	if nodeReceiving(fields) {
		t.Error("Signal on input is NO, so the node should not read as receiving")
	}

	keyed, _ := parseRptStats("Signal on input....: YES\n")
	if !nodeReceiving(keyed) {
		t.Error("Signal on input YES should read as receiving")
	}
}

// TestKeyedNodesDocumentedExample uses the exact RPT_ALINKS example from
// AllStarLink's Event Management documentation: node 2001 is keyed (K),
// node 2000 is not (U).
func TestKeyedNodesDocumentedExample(t *testing.T) {
	out := "Node 52829 variables:\nRPT_ALINKS=2,2000TU,2001RK\nRPT_NUMALINKS=2\n"
	keyed := keyedNodes(out)
	if !keyed["2001"] {
		t.Error("2001 has flag K and should read as keyed")
	}
	if keyed["2000"] {
		t.Error("2000 has flag U and should not read as keyed")
	}
	if len(keyed) != 1 {
		t.Errorf("keyed set = %v, want just {2001}", keyed)
	}
}

// TestKeyedNodesToleratesLayout confirms the value is found regardless of
// whether the command separates the name with =, :, or whitespace, since
// the exact "rpt show variables" layout on this build is unverified.
func TestKeyedNodesToleratesLayout(t *testing.T) {
	for _, out := range []string{
		"RPT_ALINKS=1,3000TK",
		"RPT_ALINKS: 1,3000TK",
		"  RPT_ALINKS   1,3000TK  ",
	} {
		if !keyedNodes(out)["3000"] {
			t.Errorf("did not find keyed node in %q", out)
		}
	}
}

// TestKeyedNodesFailsClosed is the safety contract: no RPT_ALINKS, an
// empty list, or a value that doesn't fit the grammar must yield an empty
// set, never a wrong guess — so a build lacking this variable shows no
// talking markers rather than misleading ones.
func TestKeyedNodesFailsClosed(t *testing.T) {
	for _, out := range []string{
		"",
		"No such command 'rpt show variables'",
		"RPT_ALINKS=0",
		"RPT_ALINKS=garbage-without-grammar",
		"RPT_RXKEYED=1\nRPT_TXKEYED=0",
	} {
		if n := len(keyedNodes(out)); n != 0 {
			t.Errorf("expected empty set for %q, got %d", out, n)
		}
	}
}
