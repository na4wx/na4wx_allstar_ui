package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func formRequest(t *testing.T, values url.Values) *http.Request {
	t.Helper()
	r, err := http.NewRequest("POST", "/system/sa818/apply", strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	return r
}

func TestFormatSA818FreqPadsToFourDecimals(t *testing.T) {
	got, err := formatSA818Freq("446.1")
	if err != nil {
		t.Fatalf("formatSA818Freq: %v", err)
	}
	if got != "446.1000" {
		t.Fatalf("formatSA818Freq(446.1) = %q, want 446.1000", got)
	}
}

func TestFormatSA818FreqRejectsGarbage(t *testing.T) {
	if _, err := formatSA818Freq("not-a-number"); err == nil {
		t.Fatalf("formatSA818Freq(garbage) = nil error, want an error")
	}
}

func TestSA818SettingsFromFormBlankCTCSSBecomesZeros(t *testing.T) {
	r := formRequest(t, url.Values{
		"tx_freq": {"446.1"},
		"squelch": {"4"},
		"volume":  {"5"},
		// tx_ctcss/rx_ctcss deliberately omitted, reproducing the real
		// transcript where leaving these blank at the prompts produced
		// an empty field the module rejected.
	})
	s, ferr := sa818SettingsFromForm(r)
	if ferr != "" {
		t.Fatalf("sa818SettingsFromForm error = %q", ferr)
	}
	if s.TxCTCSS != "0000" || s.RxCTCSS != "0000" {
		t.Fatalf("TxCTCSS/RxCTCSS = %q/%q, want 0000/0000", s.TxCTCSS, s.RxCTCSS)
	}
}

func TestSA818SettingsFromFormSameFreqMirrorsTx(t *testing.T) {
	r := formRequest(t, url.Values{
		"tx_freq":   {"446.1"},
		"same_freq": {"1"},
		"squelch":   {"4"},
		"volume":    {"5"},
	})
	s, ferr := sa818SettingsFromForm(r)
	if ferr != "" {
		t.Fatalf("sa818SettingsFromForm error = %q", ferr)
	}
	if s.RxFreqMHz != s.TxFreqMHz {
		t.Fatalf("RxFreqMHz = %q, want it to match TxFreqMHz %q", s.RxFreqMHz, s.TxFreqMHz)
	}
}

func TestSA818SettingsFromFormRejectsOutOfRangeSquelch(t *testing.T) {
	r := formRequest(t, url.Values{
		"tx_freq": {"446.1"},
		"squelch": {"12"},
		"volume":  {"5"},
	})
	_, ferr := sa818SettingsFromForm(r)
	if ferr == "" {
		t.Fatalf("sa818SettingsFromForm() error = \"\", want a validation error for out-of-range squelch")
	}
}
