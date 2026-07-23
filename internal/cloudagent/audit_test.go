package cloudagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hamvoipconfiggui/internal/config"
)

// TestAuditWriterEmptyPathIsNoOp covers the documented behavior in
// audit.go's doc comment: an empty auditLogPath (the default until an
// operator/deployment opts in) must never create a file or panic.
func TestAuditWriterEmptyPathIsNoOp(t *testing.T) {
	w := newAuditWriter("")
	w.log(auditEntry{Action: "system.status", OK: true})
	// No path was given, so there's nothing to check beyond "didn't panic".
}

// TestAuditWriterAppendsJSONLines confirms log() appends one JSON
// object per call, creating the parent directory on first write, and
// that multiple entries land as separate lines (the JSON Lines format
// the doc comment promises — easy to tail/grep on the device).
func TestAuditWriterAppendsJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cloud-actions.log")
	w := newAuditWriter(path)

	w.log(auditEntry{Action: "system.status", OK: true})
	w.log(auditEntry{Action: "system.reboot", OK: false, Error: "capability disabled"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), string(data))
	}
	if !strings.Contains(lines[0], `"action":"system.status"`) || !strings.Contains(lines[0], `"ok":true`) {
		t.Errorf("line 1 = %q, missing expected fields", lines[0])
	}
	if !strings.Contains(lines[1], `"action":"system.reboot"`) || !strings.Contains(lines[1], `"ok":false`) || !strings.Contains(lines[1], `"error":"capability disabled"`) {
		t.Errorf("line 2 = %q, missing expected fields", lines[1])
	}
}

// TestDispatchWritesAuditEntryPerCall confirms dispatch() (not just
// log() in isolation) records one entry for every attempt it makes,
// both for a recognized action and for an unknown one.
func TestDispatchWritesAuditEntryPerCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cloud-actions.log")
	a := New(path, config.NewStore(t.TempDir()), "does-not-exist-asterisk-binary", nil, nil, nil, "", "818-prog", "", path)

	if _, err := a.dispatch(context.Background(), "system.status", nil); err != nil {
		t.Fatalf("dispatch(system.status) error = %v", err)
	}
	if _, err := a.dispatch(context.Background(), "no.such.action", nil); err == nil {
		t.Fatal("dispatch(no.such.action) error = nil, want an error")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d audit lines, want 2: %q", len(lines), string(data))
	}
	if !strings.Contains(lines[0], `"action":"system.status"`) || !strings.Contains(lines[0], `"ok":true`) {
		t.Errorf("line 1 = %q, want system.status ok=true", lines[0])
	}
	if !strings.Contains(lines[1], `"action":"no.such.action"`) || !strings.Contains(lines[1], `"ok":false`) {
		t.Errorf("line 2 = %q, want no.such.action ok=false", lines[1])
	}
}

// TestDispatchAuditEntryNeverIncludesParams is the security property
// dispatch.go's doc comment calls out by name: several actions carry
// secrets (an SkywarnPlus Pushover API token, here) that must never
// reach the plaintext audit log, even though the action itself fails
// (SkywarnPlus isn't installed in this test) and even though the value
// was present in the params this call actually sent.
func TestDispatchAuditEntryNeverIncludesParams(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cloud-actions.log")
	a := New(path, config.NewStore(t.TempDir()), "asterisk", nil, nil, nil, "", "818-prog", "", path)

	const secret = "supersecretpushovertoken123"
	params := []byte(`{"enable":true,"userKey":"u","apiToken":"` + secret + `","debug":false}`)
	if _, err := a.dispatch(context.Background(), "skywarn.setPushover", params); err == nil {
		t.Fatal("dispatch(skywarn.setPushover) error = nil, want an error (SkywarnPlus not installed)")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("audit log contains the secret param value: %q", string(data))
	}
	if !strings.Contains(string(data), `"action":"skywarn.setPushover"`) {
		t.Errorf("audit log missing the action name: %q", string(data))
	}
}
