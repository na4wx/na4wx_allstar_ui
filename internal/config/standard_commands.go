package config

import "fmt"

// standardFunctions is a minimal, site-agnostic DTMF command map — the
// entries from a real HamVoIP-generated [functionsNNNN] stanza that
// take no node number as an argument, so they're safe to reuse
// verbatim on any node. Deliberately excludes entries like
// "cmd,/usr/local/sbin/saytime.pl <node>" from the real stanza this was
// modeled on: those embed the node number in a script argument, and a
// wrong substitution would silently announce incorrect information
// rather than just doing nothing, which is a worse failure mode than
// simply not offering that command yet.
var standardFunctions = []struct{ Digits, Command string }{
	{"1", "ilink,1"},  // disconnect the last-connected node
	{"2", "ilink,2"},  // connect, listen-only
	{"3", "ilink,3"},  // connect, both directions, stays connected
	{"4", "ilink,4"},  // remote command entry
	{"70", "ilink,5"}, // announce connected nodes
	{"71", "ilink,11"},
	{"72", "ilink,12"},
	{"73", "ilink,13"},
	{"75", "ilink,15"},
	{"76", "ilink,6"}, // disconnect everything
	{"77", "ilink,16"},
	{"78", "ilink,18"},
	{"0", "autopatchdn"},
	{"99", "cop,6"}, // PTT on, # to release
}

// standardTelemetry is the courtesy-tone set confirmed present verbatim
// in a real HamVoIP node's [telemetryNNNN] stanza — none of these are
// node-number-specific, they're pure tone-generator/sound-file
// definitions, so unlike standardFunctions this list is complete rather
// than trimmed.
var standardTelemetry = []struct{ Key, Value string }{
	{"ct1", "|t(350,0,100,2048)(500,0,100,2048)(660,0,100,2048)"},
	{"ct2", "|t(660,880,150,2048)"},
	{"ct3", "|t(440,0,150,4096)"},
	{"ct4", "|t(550,0,150,2048)"},
	{"ct5", "|t(660,0,150,2048)"},
	{"ct6", "|t(880,0,150,2048)"},
	{"ct7", "|t(660,440,150,2048)"},
	{"ct8", "|t(700,1100,150,2048)"},
	{"remotetx", "|t(1633,0,50,3000)(0,0,80,0)(1209,0,50,3000)"},
	{"remotemon", "|t(1209,0,50,2048)"},
	{"cmdmode", "|t(900,903,200,2048)"},
	{"functcomplete", "|t(1000,0,100,2048)(0,0,100,0)(1000,0,100,2048)"},
	{"patchup", "rpt/callproceeding"},
	{"patchdown", "rpt/callterminated"},
}

// standardMorse is the CW ID sound-set confirmed present verbatim in a
// real HamVoIP node's [morseNNNN] stanza.
var standardMorse = []struct{ Key, Value string }{
	{"speed", "20"},
	{"frequency", "800"},
	{"amplitude", "4096"},
	{"idfrequency", "750"},
	{"idamplitude", "1024"},
}

// ApplyStandardCommandSet gives dstNumber a working functions/telemetry/
// morse set built from known-good, site-agnostic values rather than
// copied from another node — for the case CloneNodeConfig can't help
// with: the first node on a fresh install, with no other node to copy
// from. Without this (or CloneNodeConfig), a node's functions/telemetry/
// morse fields stay blank, app_rpt falls back to looking for plain
// "functions"/"telemetry"/"morse" sections that don't exist on a real
// HamVoIP install, and the node silently accepts no DTMF commands at
// all — this is what actually happened to the node this was built for.
//
// Mirrors CloneNodeConfig's section-naming convention (functions<N>,
// telemetry<N>, morse<N>) so the two compose with no special-casing
// elsewhere. macro<N> is created empty, since saved macros are
// inherently user-defined — there's no site-agnostic default to offer.
//
// Safe to call more than once: each section is rebuilt from scratch
// from the fixed standard content, so re-running it just re-syncs
// rather than duplicating entries.
func (s *Store) ApplyStandardCommandSet(dstNumber string) error {
	if dstNumber == "" {
		return fmt.Errorf("config: destination node number is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	if !f.HasSection(dstNumber) {
		return fmt.Errorf("config: destination node %s not found in %s", dstNumber, RptConfFile)
	}

	functionsSection := "functions" + dstNumber
	for _, fn := range standardFunctions {
		f.Set(functionsSection, fn.Digits, fn.Command)
	}
	f.Set(dstNumber, "functions", functionsSection)

	telemetrySection := "telemetry" + dstNumber
	for _, t := range standardTelemetry {
		f.Set(telemetrySection, t.Key, t.Value)
	}
	f.Set(dstNumber, "telemetry", telemetrySection)

	morseSection := "morse" + dstNumber
	for _, m := range standardMorse {
		f.Set(morseSection, m.Key, m.Value)
	}
	f.Set(dstNumber, "morse", morseSection)

	macroSection := "macro" + dstNumber
	f.EnsureSection(macroSection)
	f.Set(dstNumber, "macro", macroSection)

	return s.save(RptConfFile, f)
}
