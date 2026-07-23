package cloudagent

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"hamvoipconfiggui/internal/config"
)

// TestWatchStartsPushingLiveEvents confirms a "watch" envelope actually
// starts a poller that pushes "event"/"nodeLive" data for that node,
// without waiting a full liveWatchPollInterval (poll sends immediately
// on start, matching heartbeatLoop's own "send once, then tick").
func TestWatchStartsPushingLiveEvents(t *testing.T) {
	liveEvents := make(chan envelope, 4)
	url := startFakeCloud(t, func(ctx context.Context, conn *websocket.Conn) {
		var hello envelope
		if err := wsjson.Read(ctx, conn, &hello); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeHelloAck, OK: true}); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeWatch, Node: "2000"}); err != nil {
			return
		}
		for {
			var msg envelope
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				return
			}
			if msg.Type == typeEvent && msg.Event == nodeLiveEvent {
				select {
				case liveEvents <- msg:
				default:
				}
			}
		}
	})

	a := New(t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "does-not-exist-asterisk-binary")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go a.runOnce(ctx, Settings{CloudURL: url, APIKey: "test-key", Enabled: true})

	select {
	case msg := <-liveEvents:
		if msg.Node != "2000" {
			t.Errorf("event.Node = %q, want 2000", msg.Node)
		}
		if len(msg.Data) == 0 {
			t.Error("event.Data is empty, want the marshaled liveNodeState")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive a nodeLive event after watching")
	}
}

// TestUnwatchStopsPushingLiveEvents confirms an "unwatch" actually stops
// the poller rather than leaving it running forever.
func TestUnwatchStopsPushingLiveEvents(t *testing.T) {
	var eventCount int
	liveEvents := make(chan envelope, 16)
	stopWatching := make(chan struct{})
	url := startFakeCloud(t, func(ctx context.Context, conn *websocket.Conn) {
		var hello envelope
		if err := wsjson.Read(ctx, conn, &hello); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeHelloAck, OK: true}); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeWatch, Node: "2000"}); err != nil {
			return
		}
		go func() {
			<-stopWatching
			_ = wsjson.Write(ctx, conn, envelope{Type: typeUnwatch, Node: "2000"})
		}()
		for {
			var msg envelope
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				return
			}
			if msg.Type == typeEvent && msg.Event == nodeLiveEvent {
				select {
				case liveEvents <- msg:
				default:
				}
			}
		}
	})

	a := New(t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "does-not-exist-asterisk-binary")
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	go a.runOnce(ctx, Settings{CloudURL: url, APIKey: "test-key", Enabled: true})

	// Let a couple of events through, then unwatch.
	<-liveEvents
	close(stopWatching)

	// Drain whatever was already in flight, then confirm nothing new
	// arrives for a couple of poll intervals.
	drainDeadline := time.After(300 * time.Millisecond)
drain:
	for {
		select {
		case <-liveEvents:
			eventCount++
		case <-drainDeadline:
			break drain
		}
	}

	select {
	case <-liveEvents:
		t.Fatal("received a nodeLive event after unwatch -- the poller wasn't actually stopped")
	case <-time.After(2*liveWatchPollInterval + 500*time.Millisecond):
		// Good: no more events after waiting past two poll intervals.
	}
}

// TestWatchIsIdempotent confirms watching an already-watched node
// doesn't start a second, duplicate poller (which would double the
// event rate).
func TestWatchIsIdempotent(t *testing.T) {
	lw := newLiveWatches()
	a := New(t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "asterisk")

	url := startFakeCloud(t, func(ctx context.Context, conn *websocket.Conn) {
		<-ctx.Done()
	})
	conn, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lw.watch(ctx, a, conn, "2000")
	lw.mu.Lock()
	firstStop := lw.stops["2000"]
	lw.mu.Unlock()

	lw.watch(ctx, a, conn, "2000") // should be a no-op
	lw.mu.Lock()
	secondStop := lw.stops["2000"]
	count := len(lw.stops)
	lw.mu.Unlock()

	if count != 1 {
		t.Fatalf("stops has %d entries, want 1", count)
	}
	if firstStop != secondStop {
		t.Error("watch() replaced the existing poller instead of leaving it alone")
	}
}
