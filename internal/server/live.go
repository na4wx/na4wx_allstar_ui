package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"hamvoipconfiggui/internal/system"
)

// livePollInterval is how often a node's live state is re-read while at
// least one browser is watching it over SSE. Short, because the point of
// this stream is to catch someone keying up — a PTT can last only a
// second or two. It costs a few `asterisk -rx` calls per tick, but only
// while a browser actually has the page open (the poller starts on the
// first subscriber and stops when the last one leaves).
const livePollInterval = 2 * time.Second

// liveFetchTimeout bounds one round of CLI reads.
const liveFetchTimeout = 8 * time.Second

// liveKeepalive is how often an SSE comment is sent on an otherwise idle
// connection, so proxies and load balancers don't treat it as dead.
const liveKeepalive = 25 * time.Second

// liveNodeState is the moment-to-moment state pushed to the "Right now"
// card: whether the local receiver is keyed, the raw "Signal on input"
// value (shown in the stats table, kept in sync with the pill), and the
// currently connected nodes with their keyed flags.
type liveNodeState struct {
	Receiving     bool            `json:"receiving"`
	SignalOnInput string          `json:"signalOnInput"`
	Connected     []connectedNode `json:"connected"`
}

// readLiveNodeState runs the CLI reads behind the "Right now" card. It's
// the single source of that state, shared by the initial page render and
// the SSE poller so the two can never drift.
func (s *Server) readLiveNodeState(ctx context.Context, number string) liveNodeState {
	var st liveNodeState
	if out, err := system.AsteriskRX(ctx, s.asteriskBin, "rpt stats "+number); err == nil {
		fields, _ := parseRptStats(out)
		st.Receiving = nodeReceiving(fields)
		st.SignalOnInput = fields.Value("Signal on input")
	}
	if out, err := system.AsteriskRX(ctx, s.asteriskBin, "rpt nodes "+number); err == nil {
		for _, num := range parseConnectedNodes(out) {
			st.Connected = append(st.Connected, describeNode(s.nodes, num))
		}
	}
	s.markKeyed(ctx, number, st.Connected)
	return st
}

// markKeyed flags which of connected are transmitting right now, from
// app_rpt's RPT_ALINKS (see keyedNodes). Best-effort and additive: on a
// build without that variable, nothing is marked. Skipped entirely when
// nothing is connected, so an idle node makes no extra CLI call.
func (s *Server) markKeyed(ctx context.Context, number string, connected []connectedNode) {
	if len(connected) == 0 {
		return
	}
	out, err := system.AsteriskRX(ctx, s.asteriskBin, "rpt show variables "+number)
	if err != nil {
		return
	}
	keyed := keyedNodes(out)
	for i := range connected {
		if keyed[connected[i].Number] {
			connected[i].Keyed = true
		}
	}
}

// liveHub fans out per-node live state to any number of connected
// browsers over SSE. One background poller runs per node that has at
// least one subscriber; it broadcasts only when the state actually
// changes, so an idle node produces no traffic beyond keepalives.
type liveHub struct {
	server *Server

	mu       sync.Mutex
	channels map[string]map[chan []byte]struct{}
	stops    map[string]chan struct{}
}

func newLiveHub(s *Server) *liveHub {
	return &liveHub{
		server:   s,
		channels: make(map[string]map[chan []byte]struct{}),
		stops:    make(map[string]chan struct{}),
	}
}

// subscribe registers a listener for node and returns its channel plus
// an unsubscribe func. The first subscriber for a node starts its
// poller. The channel is buffered and lossy on the sending side (see
// broadcast), so a slow reader can't stall the poller or other clients.
func (h *liveHub) subscribe(node string) (<-chan []byte, func()) {
	ch := make(chan []byte, 4)
	h.mu.Lock()
	subs := h.channels[node]
	if subs == nil {
		subs = make(map[chan []byte]struct{})
		h.channels[node] = subs
		stop := make(chan struct{})
		h.stops[node] = stop
		go h.poll(node, stop)
	}
	subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() { h.unsubscribe(node, ch) }
}

func (h *liveHub) unsubscribe(node string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.channels[node]
	if subs == nil {
		return
	}
	if _, ok := subs[ch]; ok {
		delete(subs, ch)
		close(ch)
	}
	if len(subs) == 0 {
		delete(h.channels, node)
		if stop := h.stops[node]; stop != nil {
			close(stop)
			delete(h.stops, node)
		}
	}
}

// broadcast delivers payload to every subscriber of node, dropping it for
// any whose buffer is full rather than blocking — a stalled client falls
// behind and catches up on the next change, never holding up the rest.
func (h *liveHub) broadcast(node string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.channels[node] {
		select {
		case ch <- payload:
		default:
		}
	}
}

// poll re-reads node's live state on an interval and broadcasts it when
// it differs from the last, until stop is closed (the last subscriber
// left). Deduping on the serialized state is what keeps an idle node
// silent: connection timers aren't in this payload, so it only changes
// on a real event (keyup, connect, disconnect).
func (h *liveHub) poll(node string, stop chan struct{}) {
	ticker := time.NewTicker(livePollInterval)
	defer ticker.Stop()

	var last string
	tick := func() {
		ctx, cancel := context.WithTimeout(context.Background(), liveFetchTimeout)
		st := h.server.readLiveNodeState(ctx, node)
		cancel()
		b, err := json.Marshal(st)
		if err != nil {
			return
		}
		if string(b) == last {
			return
		}
		last = string(b)
		h.broadcast(node, b)
	}

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			tick()
		}
	}
}

// handleNodeLive streams a node's live state to the browser as
// Server-Sent Events. It sends the current state immediately on connect
// (so the card doesn't wait for the first change), then one event per
// change, plus periodic keepalive comments. The stream ends when the
// browser disconnects (request context cancelled).
//
// This is a progressive enhancement: the "Right now" card is already
// rendered server-side on page load, so if EventSource is unavailable or
// blocked by a proxy, the card still shows a correct snapshot — it just
// won't update without a reload.
func (s *Server) handleNodeLive(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // tell nginx not to buffer the stream

	ctx := r.Context()

	writeState := func(b []byte) bool {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// Immediate snapshot, so a freshly-connected client isn't blank until
	// the poller happens to see a change.
	initCtx, cancel := context.WithTimeout(ctx, liveFetchTimeout)
	initial := s.readLiveNodeState(initCtx, number)
	cancel()
	if b, err := json.Marshal(initial); err == nil {
		if !writeState(b) {
			return
		}
	}

	ch, unsubscribe := s.live.subscribe(number)
	defer unsubscribe()

	keepalive := time.NewTicker(liveKeepalive)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-ch:
			if !ok {
				return
			}
			if !writeState(b) {
				return
			}
		case <-keepalive.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
