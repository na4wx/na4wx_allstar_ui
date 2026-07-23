package cloudagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"hamvoipconfiggui/internal/config"
)

// startFakeCloud runs handler against every WebSocket connection to a
// local test server and returns its ws:// URL, standing in for the real
// cloud site's /agent endpoint for these tests.
func startFakeCloud(t *testing.T, handler func(ctx context.Context, conn *websocket.Conn)) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		handler(r.Context(), conn)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// TestRunOnceHelloAckAndHeartbeat is the end-to-end happy path over a
// real WebSocket connection: hello is sent with the right API key, and
// once the fake cloud acks it, a status heartbeat event arrives without
// waiting for heartbeatInterval (heartbeatLoop sends one immediately on
// connect).
func TestRunOnceHelloAckAndHeartbeat(t *testing.T) {
	heartbeats := make(chan envelope, 4)
	var helloSeen envelope
	url := startFakeCloud(t, func(ctx context.Context, conn *websocket.Conn) {
		if err := wsjson.Read(ctx, conn, &helloSeen); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeHelloAck, OK: true}); err != nil {
			return
		}
		for {
			var msg envelope
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				return
			}
			if msg.Type == typeEvent && msg.Event == eventStatus {
				heartbeats <- msg
			}
		}
	})

	a := New(t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "does-not-exist-asterisk-binary")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		done <- a.runOnce(ctx, Settings{CloudURL: url, APIKey: "test-key", Enabled: true})
	}()

	select {
	case <-heartbeats:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive a heartbeat event")
	}
	if helloSeen.Type != typeHello || helloSeen.APIKey != "test-key" {
		t.Errorf("hello = %+v, want type=hello apiKey=test-key", helloSeen)
	}

	cancel()
	if ok := <-done; !ok {
		t.Error("runOnce() = false, want true (hello handshake succeeded)")
	}
}

// TestRunOnceRejectedHelloReturnsFalse confirms a rejected API key
// reports helloSucceeded=false, which is what Run uses to keep backing
// off instead of resetting to the initial 1s delay.
func TestRunOnceRejectedHelloReturnsFalse(t *testing.T) {
	url := startFakeCloud(t, func(ctx context.Context, conn *websocket.Conn) {
		var hello envelope
		if err := wsjson.Read(ctx, conn, &hello); err != nil {
			return
		}
		_ = wsjson.Write(ctx, conn, envelope{Type: typeHelloAck, OK: false, Error: "invalid api key"})
	})

	a := New(t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "asterisk")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if ok := a.runOnce(ctx, Settings{CloudURL: url, APIKey: "bad-key", Enabled: true}); ok {
		t.Error("runOnce() = true, want false when the cloud rejects the hello")
	}
}

// TestRunOnceHandlesRelayedCall exercises the actual relay path a real
// cloud backend would use: a "call" message for a registered action
// gets a correlated "result" back with the right id and marshaled data.
func TestRunOnceHandlesRelayedCall(t *testing.T) {
	results := make(chan envelope, 1)
	url := startFakeCloud(t, func(ctx context.Context, conn *websocket.Conn) {
		var hello envelope
		if err := wsjson.Read(ctx, conn, &hello); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeHelloAck, OK: true}); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeCall, ID: "call-1", Action: "system.status"}); err != nil {
			return
		}
		for {
			var msg envelope
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				return
			}
			if msg.Type == typeResult && msg.ID == "call-1" {
				results <- msg
				return
			}
		}
	})

	a := New(t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "does-not-exist-asterisk-binary")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go a.runOnce(ctx, Settings{CloudURL: url, APIKey: "test-key", Enabled: true})

	select {
	case res := <-results:
		if !res.OK || res.Error != "" {
			t.Errorf("result = %+v, want ok=true and no error", res)
		}
		if len(res.Data) == 0 {
			t.Error("result.Data is empty, want the marshaled system.Status")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive a result for the relayed call")
	}
}

// TestReloadClosesActiveConnection is the local kill-switch guarantee
// (see Agent.Reload's doc comment): disabling Cloud Sync, or changing
// its settings, while already connected must cut that connection right
// away, not just stop future reconnect attempts. Without this,
// physical/local access to the node wouldn't reliably override a
// still-open session.
func TestReloadClosesActiveConnection(t *testing.T) {
	heartbeats := make(chan envelope, 1)
	url := startFakeCloud(t, func(ctx context.Context, conn *websocket.Conn) {
		var hello envelope
		if err := wsjson.Read(ctx, conn, &hello); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeHelloAck, OK: true}); err != nil {
			return
		}
		for {
			var msg envelope
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				return
			}
			if msg.Type == typeEvent && msg.Event == eventStatus {
				select {
				case heartbeats <- msg:
				default:
				}
			}
		}
	})

	a := New(t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "does-not-exist-asterisk-binary")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		done <- a.runOnce(ctx, Settings{CloudURL: url, APIKey: "test-key", Enabled: true})
	}()

	select {
	case <-heartbeats:
	case <-time.After(2 * time.Second):
		t.Fatal("connection never became active")
	}

	start := time.Now()
	a.Reload()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("runOnce took %v to return after Reload, want promptly", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runOnce did not return after Reload -- the local kill switch didn't actually close the connection")
	}
}

// TestRunOnceUnknownActionReturnsErrorResult confirms an action name
// the registry doesn't recognize comes back as an error result, not a
// dropped message or a panic.
func TestRunOnceUnknownActionReturnsErrorResult(t *testing.T) {
	results := make(chan envelope, 1)
	url := startFakeCloud(t, func(ctx context.Context, conn *websocket.Conn) {
		var hello envelope
		if err := wsjson.Read(ctx, conn, &hello); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeHelloAck, OK: true}); err != nil {
			return
		}
		if err := wsjson.Write(ctx, conn, envelope{Type: typeCall, ID: "call-2", Action: "system.reboot"}); err != nil {
			return
		}
		for {
			var msg envelope
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				return
			}
			if msg.Type == typeResult && msg.ID == "call-2" {
				results <- msg
				return
			}
		}
	})

	a := New(t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "does-not-exist-asterisk-binary")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go a.runOnce(ctx, Settings{CloudURL: url, APIKey: "test-key", Enabled: true})

	select {
	case res := <-results:
		if res.OK {
			t.Error("result.OK = true, want false for an action not in the registry")
		}
		if res.Error == "" {
			t.Error("result.Error is empty, want an explanation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive a result for the relayed call")
	}
}
