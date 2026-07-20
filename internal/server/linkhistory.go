package server

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"hamvoipconfiggui/internal/nodedb"
	"hamvoipconfiggui/internal/system"
)

// linkHistoryLimit is how many past records are kept per node. Small on
// purpose: this is a "what just happened" glance for the home page, not
// a log — Asterisk's own log file (shown on the System page) is where
// real history lives.
const linkHistoryLimit = 10

// linkHistoryInterval is how often each node's link state is sampled.
// Frequent enough to catch a short-lived connection, cheap enough to be
// irrelevant on a dedicated node Pi (one asterisk -rx per node per tick).
const linkHistoryInterval = 30 * time.Second

// linkSnapshot is one recorded moment in a node's connection history,
// holding both commands' output parsed into table form plus the raw
// text as a fallback for output this app doesn't recognize.
type linkSnapshot struct {
	At time.Time

	// Connected nodes, from "rpt nodes".
	Nodes        []string
	ConnectedRaw string

	// Link activity, from "rpt lstats". ActivityOK is false when the
	// output had no recognizable header/separator pair, in which case
	// only ActivityRaw is meaningful.
	Headers     []string
	Rows        [][]string
	ActivityRaw string
	ActivityOK  bool
}

// key is the value used to decide whether this snapshot represents a
// real change from the previous one — the set of connected nodes.
// Deliberately not the whole output: "rpt lstats" includes running
// connection timers, so it differs on literally every poll and would
// otherwise push every real event out of the buffer within minutes.
func (s linkSnapshot) key() string { return strings.Join(s.Nodes, ",") }

// linkHistory keeps a small rolling record, per node, of how its
// connection state has changed over time. Safe for concurrent use: the
// background poller writes while page renders read.
type linkHistory struct {
	mu     sync.Mutex
	byNode map[string][]linkSnapshot
}

func newLinkHistory() *linkHistory {
	return &linkHistory{byNode: make(map[string][]linkSnapshot)}
}

// record parses a round of command output and appends it for node, but
// only when the set of connected nodes differs from the most recent
// record — so the 10 retained records cover 10 real connect/disconnect
// events rather than the last five minutes of polling.
func (h *linkHistory) record(node, connectedOut, activityOut string) {
	snap := linkSnapshot{
		At:           time.Now(),
		Nodes:        parseConnectedNodes(connectedOut),
		ConnectedRaw: connectedOut,
		ActivityRaw:  activityOut,
	}
	snap.Headers, snap.Rows, snap.ActivityOK = parseLstats(activityOut)

	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.byNode[node]
	if n := len(list); n > 0 && list[n-1].key() == snap.key() {
		return
	}
	list = append(list, snap)
	if len(list) > linkHistoryLimit {
		list = list[len(list)-linkHistoryLimit:]
	}
	h.byNode[node] = list
}

// forNode returns node's records newest first, as a copy the caller can
// hold without holding the lock.
func (h *linkHistory) forNode(node string) []linkSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.byNode[node]
	out := make([]linkSnapshot, len(list))
	for i, snap := range list {
		out[len(list)-1-i] = snap
	}
	return out
}

// whenAgo renders a record time as an absolute clock time plus how long
// ago it was, e.g. "15:04:05" / "4m ago".
func whenAgo(at time.Time) (string, string) {
	d := time.Since(at).Round(time.Second)
	var ago string
	switch {
	case d < 5*time.Second:
		ago = "just now"
	case d < time.Minute:
		ago = strconv.Itoa(int(d.Seconds())) + "s ago"
	case d < time.Hour:
		ago = strconv.Itoa(int(d.Minutes())) + "m ago"
	default:
		ago = strconv.Itoa(int(d.Hours())) + "h ago"
	}
	return at.Format("Jan 2, 15:04:05"), ago
}

// connectedNode is one node in a record, paired with whatever the node
// directory knows about it. Callsign is empty for a node that isn't in
// the directory (or when no directory has been downloaded), in which
// case the UI shows the bare number.
type connectedNode struct {
	Number   string `json:"number"`
	Callsign string `json:"callsign"`
	Detail   string `json:"detail"`

	// Keyed is set only on the live "connected right now" list, when
	// app_rpt's RPT_ALINKS reports this adjacent node as transmitting
	// (see keyedNodes). It is never set on historical records, which
	// have no live keyed state to report.
	Keyed bool `json:"keyed"`
}

// connectedRecord is one row of the home page's "Connected nodes"
// history table.
type connectedRecord struct {
	When  string
	Ago   string
	Nodes []connectedNode
}

// activityRecord is one row of the "Link activity" history table. Each
// sampled moment contributes one row per connected peer, so the table
// reads chronologically; Empty marks a moment when nothing was linked,
// which is itself the meaningful event when a link drops.
type activityRecord struct {
	When   string
	Ago    string
	Fields []string
	Empty  bool
}

// directory is the lookup buildLinkTables uses to turn node numbers into
// callsigns. Satisfied by *nodedb.Store; an interface so the table
// building can be tested without one.
type directory interface {
	Lookup(number string) (nodedb.Entry, bool)
}

// describeNode pairs a node number with its directory entry. Detail is
// the description and location joined for a tooltip, skipping either if
// blank — plenty of real entries have an empty description (node 52829's
// own is empty in the live database).
func describeNode(dir directory, number string) connectedNode {
	n := connectedNode{Number: number}
	if dir == nil {
		return n
	}
	e, ok := dir.Lookup(number)
	if !ok {
		return n
	}
	n.Callsign = e.Label()
	var parts []string
	if e.Description != "" {
		parts = append(parts, e.Description)
	}
	if e.Location != "" {
		parts = append(parts, e.Location)
	}
	n.Detail = strings.Join(parts, " · ")
	return n
}

// buildLinkTables flattens a node's records into the two tables the home
// page renders. Headers come from the most recent record that parsed
// cleanly, so the table keeps working even if a later sample was taken
// while Asterisk was mid-restart and returned something unrecognizable.
//
// The activity table gains a "Callsign" column immediately after
// app_rpt's own first column, which its header row identifies as the
// node number. If that lookup misses, the cell is simply blank — no
// attempt is made to guess a callsign from any other column, since what
// the remaining columns contain varies by app_rpt version.
func buildLinkTables(dir directory, snaps []linkSnapshot) (connected []connectedRecord, headers []string, activity []activityRecord) {
	for _, snap := range snaps {
		when, ago := whenAgo(snap.At)

		rec := connectedRecord{When: when, Ago: ago}
		for _, number := range snap.Nodes {
			rec.Nodes = append(rec.Nodes, describeNode(dir, number))
		}
		connected = append(connected, rec)

		if headers == nil && snap.ActivityOK {
			for i, h := range snap.Headers {
				headers = append(headers, displayHeader(h))
				if i == 0 {
					headers = append(headers, "Callsign")
				}
			}
		}
		if !snap.ActivityOK || len(snap.Rows) == 0 {
			activity = append(activity, activityRecord{When: when, Ago: ago, Empty: true})
			continue
		}
		for _, row := range snap.Rows {
			fields := make([]string, 0, len(row)+1)
			for i, v := range row {
				fields = append(fields, v)
				if i == 0 {
					fields = append(fields, describeNode(dir, v).Callsign)
				}
			}
			activity = append(activity, activityRecord{When: when, Ago: ago, Fields: fields})
		}
	}
	return connected, headers, activity
}

// StartLinkHistoryPoller samples every configured node's link state on
// an interval, so the history reflects what actually happened rather
// than only what someone happened to have the page open for. Runs until
// ctx is cancelled. Call once, from main.
func (s *Server) StartLinkHistoryPoller(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(linkHistoryInterval)
		defer ticker.Stop()
		s.sampleLinkHistory(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sampleLinkHistory(ctx)
			}
		}
	}()
}

// sampleLinkHistory records one round of snapshots. Errors are ignored
// rather than logged per-tick: Asterisk being down is an expected,
// already-visible state (the home page's status pill says so), and
// logging it every 30 seconds would bury the log in noise.
func (s *Server) sampleLinkHistory(ctx context.Context) {
	numbers, err := s.store.ListNodes()
	if err != nil {
		return
	}
	for _, number := range numbers {
		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		connected, err := system.AsteriskRX(callCtx, s.asteriskBin, "rpt nodes "+number)
		if err != nil {
			cancel()
			continue
		}
		activity, _ := system.AsteriskRX(callCtx, s.asteriskBin, "rpt lstats "+number)
		cancel()
		s.history.record(number, connected, activity)
	}
}
