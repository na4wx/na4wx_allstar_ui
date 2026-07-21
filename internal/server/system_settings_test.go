package server

import (
	"net/http/httptest"
	"testing"
)

// TestRefererPath covers handleApplyRestart's redirect target: it must
// only ever send the operator back to a page on this same server, never
// wherever an arbitrary (and possibly spoofed) Referer header points.
func TestRefererPath(t *testing.T) {
	cases := []struct {
		name     string
		referer  string
		wantPath string
	}{
		{"no referer", "", "/"},
		{"same-origin path", "http://example.com/nodes/2000", "/nodes/2000"},
		{"same-origin with query", "http://example.com/system?x=1", "/system?x=1"},
		{"different host", "http://evil.example/nodes/2000", "/"},
		{"malformed", "://not a url", "/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://example.com/system/apply-restart", nil)
			if c.referer != "" {
				r.Header.Set("Referer", c.referer)
			}
			if got := refererPath(r); got != c.wantPath {
				t.Errorf("refererPath = %q, want %q", got, c.wantPath)
			}
		})
	}
}
