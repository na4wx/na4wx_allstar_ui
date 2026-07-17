package server

import "testing"

func TestDefaultNodeIDRecording(t *testing.T) {
	cases := []struct{ in, want string }{
		{"n0call", "|iN0CALL"},
		{"NA4WX", "|iNA4WX"},
		{"  na4wx  ", "|iNA4WX"},
	}
	for _, c := range cases {
		if got := defaultNodeIDRecording(c.in); got != c.want {
			t.Errorf("defaultNodeIDRecording(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
