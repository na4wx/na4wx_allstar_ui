package server

import (
	"strings"
	"testing"
)

// realNodesNone / realLstatsEmpty are verbatim output from a real
// HamVoIP node (Asterisk 1.4.23-pre.hamvoip-V1.7.1, app_rpt-0.327) with
// nothing connected — captured rather than invented, since guessing at
// this app's real-world formats has been a recurring source of bugs.
const realNodesNone = `
************************* CONNECTED NODES *************************

<NONE>
`

const realLstatsEmpty = `NODE           PEER                RECONNECTS  DIRECTION  CONNECT TIME        CONNECT STATE
----           ----                ----------  ---------  ------------        -------------
`

// realLstatsConnected mirrors the same header/separator geometry with
// data rows filled in, to exercise the value-extraction path the empty
// capture can't reach.
const realLstatsConnected = `NODE           PEER                RECONNECTS  DIRECTION  CONNECT TIME        CONNECT STATE
----           ----                ----------  ---------  ------------        -------------
49616          NA4WX               0           OUT        00:05:12            ESTABLISHED
2000           W1AW                3           IN         01:20:44            ESTABLISHED
`

func TestParseConnectedNodesNone(t *testing.T) {
	got := parseConnectedNodes(realNodesNone)
	if len(got) != 0 {
		t.Errorf("expected no connected nodes, got %q", got)
	}
}

func TestParseConnectedNodesSome(t *testing.T) {
	out := `
************************* CONNECTED NODES *************************

49616  2000  27225
`
	got := parseConnectedNodes(out)
	want := []string{"49616", "2000", "27225"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("node %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseLstatsEmpty(t *testing.T) {
	headers, rows, ok := parseLstats(realLstatsEmpty)
	if !ok {
		t.Fatal("expected the header/separator pair to be recognized")
	}
	want := []string{"NODE", "PEER", "RECONNECTS", "DIRECTION", "CONNECT TIME", "CONNECT STATE"}
	if len(headers) != len(want) {
		t.Fatalf("got %d headers %q, want %d %q", len(headers), headers, len(want), want)
	}
	for i := range want {
		if headers[i] != want[i] {
			t.Errorf("header %d = %q, want %q", i, headers[i], want[i])
		}
	}
	if len(rows) != 0 {
		t.Errorf("expected no data rows, got %q", rows)
	}
}

// TestParseLstatsMultiWordHeaders guards the specific reason column
// positions are derived from the separator row instead of splitting on
// whitespace: "CONNECT TIME" and "CONNECT STATE" contain spaces, so a
// naive Fields() split yields 8 columns instead of 6 and silently
// misaligns every value under the wrong heading.
func TestParseLstatsMultiWordHeaders(t *testing.T) {
	headers, _, ok := parseLstats(realLstatsEmpty)
	if !ok {
		t.Fatal("parse failed")
	}
	if len(headers) != 6 {
		t.Fatalf("got %d columns %q, want 6 — whitespace splitting would give 8", len(headers), headers)
	}
	if n := len(strings.Fields(realLstatsEmpty[:strings.Index(realLstatsEmpty, "\n")])); n != 8 {
		t.Fatalf("precondition: expected naive Fields() to give 8, got %d", n)
	}
}

func TestParseLstatsRows(t *testing.T) {
	headers, rows, ok := parseLstats(realLstatsConnected)
	if !ok {
		t.Fatal("parse failed")
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %q", len(rows), rows)
	}
	if len(rows[0]) != len(headers) {
		t.Fatalf("row 0 has %d fields, headers have %d", len(rows[0]), len(headers))
	}
	want := []string{"49616", "NA4WX", "0", "OUT", "00:05:12", "ESTABLISHED"}
	for i := range want {
		if rows[0][i] != want[i] {
			t.Errorf("row0[%d] (%s) = %q, want %q", i, headers[i], rows[0][i], want[i])
		}
	}
	if rows[1][0] != "2000" || rows[1][5] != "ESTABLISHED" {
		t.Errorf("row1 = %q", rows[1])
	}
}

// TestParseLstatsUnrecognized asserts the fallback contract: output with
// no separator row must report ok=false so the caller shows raw text
// rather than an empty table that looks like "nothing is connected".
func TestParseLstatsUnrecognized(t *testing.T) {
	if _, _, ok := parseLstats("some other app_rpt version output\nwith no rule row\n"); ok {
		t.Error("expected ok=false for output with no separator row")
	}
}
