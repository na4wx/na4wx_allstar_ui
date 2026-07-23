package cloudagent

import (
	"context"
	"testing"
	"time"

	"hamvoipconfiggui/internal/config"
)

func TestNextBackoffDoublesAndCaps(t *testing.T) {
	cases := []struct {
		cur  time.Duration
		want time.Duration
	}{
		{1 * time.Second, 2 * time.Second},
		{2 * time.Second, 4 * time.Second},
		{32 * time.Second, 60 * time.Second}, // 64s would exceed the cap
		{60 * time.Second, 60 * time.Second}, // already at the cap
	}
	for _, c := range cases {
		if got := nextBackoff(c.cur); got != c.want {
			t.Errorf("nextBackoff(%v) = %v, want %v", c.cur, got, c.want)
		}
	}
}

func TestJitterStaysInRange(t *testing.T) {
	d := 10 * time.Second
	for i := 0; i < 100; i++ {
		got := jitter(d)
		if got < 0 || got >= d {
			t.Fatalf("jitter(%v) = %v, want in [0, %v)", d, got, d)
		}
	}
}

func TestJitterZeroIsZero(t *testing.T) {
	if got := jitter(0); got != 0 {
		t.Errorf("jitter(0) = %v, want 0", got)
	}
}

// TestRunSkipsConnectingWhenDisabled confirms Run never dials out at
// all while the feature is off/unconfigured -- the "no inbound ports,
// ever" invariant's outbound-side counterpart: nothing should even try
// to reach a cloud URL until the operator has actually opted in with a
// URL and key.
func TestRunSkipsConnectingWhenDisabled(t *testing.T) {
	a := newTestAgent(t, t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "asterisk")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	a.Run(ctx) // Settings default to zero value (Enabled: false) -- must return via ctx timeout, not hang or panic, without ever attempting a dial.
}

// TestReloadWakesWaitEarly confirms Reload actually shortens the wait
// rather than being a no-op -- the settings-save handler depends on
// this to make turning the feature on take effect promptly.
func TestReloadWakesWaitEarly(t *testing.T) {
	a := newTestAgent(t, t.TempDir()+"/settings.json", config.NewStore(t.TempDir()), "asterisk")
	done := make(chan bool, 1)
	go func() {
		done <- a.wait(context.Background(), 10*time.Second)
	}()

	time.Sleep(20 * time.Millisecond) // let the goroutine reach the select
	a.Reload()

	select {
	case ok := <-done:
		if !ok {
			t.Error("wait() returned false, want true (woken by Reload, not ctx cancellation)")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait() did not return promptly after Reload")
	}
}
