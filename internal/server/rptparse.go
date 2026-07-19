package server

import "strings"

// Parsing for app_rpt's own "rpt nodes" / "rpt lstats" CLI output.
//
// Column positions are derived from the output's own "----" separator
// row rather than hardcoded, and headers are taken verbatim from the
// output's header row rather than being named here. That matters: this
// app has been wrong before by assuming a real-world Asterisk/HamVoIP
// format matched what a sample file suggested. Deriving the shape from
// each response means a different app_rpt version with different or
// extra columns still renders correctly, and anything unparseable falls
// back to showing the raw text rather than silently displaying a
// confidently wrong table.

// isSeparatorLine reports whether a line is a column-rule row, i.e.
// made up only of dashes and spaces, e.g. "----  ----------  ---------".
func isSeparatorLine(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" || !strings.Contains(t, "-") {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] != '-' && t[i] != ' ' {
			return false
		}
	}
	return true
}

// columnStarts returns the index where each run of dashes begins, which
// is where each fixed-width column starts.
func columnStarts(sep string) []int {
	var starts []int
	inRun := false
	for i := 0; i < len(sep); i++ {
		if sep[i] == '-' {
			if !inRun {
				starts = append(starts, i)
				inRun = true
			}
			continue
		}
		inRun = false
	}
	return starts
}

// sliceColumns cuts one output line into fields at the given column
// starts. Each field runs to the start of the next column (so a value
// wider than its own dash run isn't truncated) and the last runs to end
// of line. Lines shorter than a column start yield an empty field
// rather than panicking, since trailing columns are often absent.
func sliceColumns(line string, starts []int) []string {
	out := make([]string, len(starts))
	for i, start := range starts {
		if start >= len(line) {
			continue
		}
		end := len(line)
		if i+1 < len(starts) && starts[i+1] < end {
			end = starts[i+1]
		}
		out[i] = strings.TrimSpace(line[start:end])
	}
	return out
}

// parseLstats turns "rpt lstats" output into headers plus one row per
// connected peer. ok is false when the output has no recognizable
// header/separator pair, in which case the caller should fall back to
// showing the raw text.
func parseLstats(out string) (headers []string, rows [][]string, ok bool) {
	lines := strings.Split(out, "\n")
	sep := -1
	for i, l := range lines {
		if isSeparatorLine(l) {
			sep = i
			break
		}
	}
	// sep must be at least 1: the line above it is the header row.
	if sep < 1 {
		return nil, nil, false
	}
	starts := columnStarts(lines[sep])
	if len(starts) == 0 {
		return nil, nil, false
	}
	headers = sliceColumns(lines[sep-1], starts)
	for _, l := range lines[sep+1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		rows = append(rows, sliceColumns(l, starts))
	}
	return headers, rows, true
}

// statField is one "Label.........: Value" line from "rpt stats".
type statField struct {
	Label string
	Value string
}

// statFields wraps the parsed block so a value can be looked up by label
// without re-scanning at each call site.
type statFields []statField

// Value returns the value for an exact label, or "" if absent. Labels
// are matched exactly rather than by prefix so a future app_rpt adding
// e.g. "Signal on input B" can't be mistaken for "Signal on input".
func (s statFields) Value(label string) string {
	for _, f := range s {
		if f.Label == label {
			return f.Value
		}
	}
	return ""
}

// parseRptStats turns "rpt stats <node>" output into label/value pairs.
// The format is a label padded with dots, a colon, then the value:
//
//	Signal on input..................................: NO
//	TX time today....................................: 00:00:00.0
//
// Splitting at the first colon is what makes the time values work —
// they contain colons of their own, but always after the separator. The
// banner line and blank lines are skipped.
//
// ok is false when nothing parsed, so the caller can fall back to
// showing the raw text rather than an empty panel that would read as
// "this node has no status".
func parseRptStats(out string) (statFields, bool) {
	var fields statFields
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.Contains(t, "***") {
			continue
		}
		i := strings.Index(t, ":")
		if i < 0 {
			continue
		}
		label := strings.TrimRight(strings.TrimSpace(t[:i]), ".")
		label = strings.TrimSpace(label)
		value := strings.TrimSpace(t[i+1:])
		if label == "" {
			continue
		}
		fields = append(fields, statField{Label: label, Value: value})
	}
	return fields, len(fields) > 0
}

// displayHeader softens a column heading for display: app_rpt prints
// its headings in all caps ("CONNECT TIME"), which reads as raw machine
// output on a page meant for people who don't think in CLI. Only
// all-caps headings are touched, so a future app_rpt version already
// using mixed case is left exactly as it wrote it. This is presentation
// only — the heading words themselves are still app_rpt's, not this
// app's invention.
func displayHeader(h string) string {
	if h == "" || h != strings.ToUpper(h) {
		return h
	}
	lower := strings.ToLower(h)
	return strings.ToUpper(lower[:1]) + lower[1:]
}

// parseConnectedNodes pulls the node numbers out of "rpt nodes" output,
// which wraps its list in a "**** CONNECTED NODES ****" banner and uses
// the literal "<NONE>" when nothing is connected. Returns an empty slice
// for the not-connected case, so "no connections" and "couldn't read"
// stay distinguishable to the caller.
func parseConnectedNodes(out string) []string {
	var nodes []string
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.Contains(t, "***") {
			continue
		}
		for _, f := range strings.FieldsFunc(t, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		}) {
			if f == "" || strings.EqualFold(f, "<NONE>") {
				continue
			}
			nodes = append(nodes, f)
		}
	}
	return nodes
}

// nodeReceiving reports whether someone is keying this node's receiver
// right now, from "rpt stats"'s "Signal on input" field.
//
// This is the local RF input specifically. Audio arriving from a linked
// node over the internet does not set it, and this app_rpt build's CLI
// exposes no per-link keyed state at all — "rpt lstats" here reports
// connection state and timers, not who is talking. So this answers "is
// someone transmitting into this node" and deliberately does not claim
// to answer "which connected station is currently talking", which the
// available data cannot support.
func nodeReceiving(fields statFields) bool {
	return strings.EqualFold(fields.Value("Signal on input"), "YES")
}
