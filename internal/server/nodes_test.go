package server

import "testing"

func TestIDTimeToMinutes(t *testing.T) {
	cases := []struct {
		ms   string
		want string
	}{
		{"", ""},
		{"300000", "5"},
		{"90000", "1.5"},
		{"600000", "10"},
		{"0", "0"},
		{"not-a-number", ""},
	}
	for _, c := range cases {
		if got := idTimeToMinutes(c.ms); got != c.want {
			t.Errorf("idTimeToMinutes(%q) = %q, want %q", c.ms, got, c.want)
		}
	}
}

func TestMinutesToIDTime(t *testing.T) {
	cases := []struct {
		minutes string
		want    string
	}{
		{"", ""},
		{"5", "300000"},
		{"1.5", "90000"},
		{"10", "600000"},
		{"0", "0"},
		{"not-a-number", "not-a-number"},
	}
	for _, c := range cases {
		if got := minutesToIDTime(c.minutes); got != c.want {
			t.Errorf("minutesToIDTime(%q) = %q, want %q", c.minutes, got, c.want)
		}
	}
}

func TestIDTimeMinutesRoundTrip(t *testing.T) {
	for _, ms := range []string{"300000", "90000", "600000", "180000"} {
		minutes := idTimeToMinutes(ms)
		if got := minutesToIDTime(minutes); got != ms {
			t.Errorf("round trip %q -> %q -> %q, want back to %q", ms, minutes, got, ms)
		}
	}
}
