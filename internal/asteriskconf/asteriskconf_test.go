package asteriskconf

import (
	"strings"
	"testing"
)

// rptFixture is representative of a real HamVoIP rpt.conf: heavily
// commented, mixes "=" and "=>", has a [nodes] stanza and a per-node
// stanza.
const rptFixture = `;
; rpt.conf - app_rpt configuration
;
[nodes]
2000 = radio@127.0.0.1:4569/2000,NONE

[2000]
; Radio Interface Type
rxchannel = USBRADIO/usb          ; USB radio interface
duplex = 1                        ; 1=repeater, 0=simplex
hangtime = 5000
totime = 200000                   ; TX timeout in ms
telemetry = telemetry
; DTMF function map
functions = functions
`

func TestParseRoundTrip(t *testing.T) {
	f, err := Parse(strings.NewReader(rptFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := f.String()
	if got != rptFixture {
		t.Fatalf("round trip mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, rptFixture)
	}
}

func TestSections(t *testing.T) {
	f, err := Parse(strings.NewReader(rptFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"nodes", "2000"}
	got := f.Sections()
	if len(got) != len(want) {
		t.Fatalf("Sections() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Sections()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGet(t *testing.T) {
	f, err := Parse(strings.NewReader(rptFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	v, ok := f.Get("2000", "rxchannel")
	if !ok || v != "USBRADIO/usb" {
		t.Fatalf("Get(2000, rxchannel) = %q, %v", v, ok)
	}
	v, ok = f.Get("2000", "hangtime")
	if !ok || v != "5000" {
		t.Fatalf("Get(2000, hangtime) = %q, %v", v, ok)
	}
	if _, ok := f.Get("2000", "nonexistent"); ok {
		t.Fatalf("Get(2000, nonexistent) should not be found")
	}
	if _, ok := f.Get("nosuchsection", "rxchannel"); ok {
		t.Fatalf("Get(nosuchsection, ...) should not be found")
	}
}

func TestSetPreservesCommentAndFormatting(t *testing.T) {
	f, err := Parse(strings.NewReader(rptFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	f.Set("2000", "hangtime", "3000")
	out := f.String()
	if !strings.Contains(out, "hangtime = 3000") {
		t.Fatalf("expected updated hangtime, got:\n%s", out)
	}

	f.Set("2000", "totime", "150000")
	out = f.String()
	if !strings.Contains(out, "totime = 150000") || !strings.Contains(out, "; TX timeout in ms") {
		t.Fatalf("expected inline comment preserved after edit, got:\n%s", out)
	}

	// Unrelated lines/comments must be untouched.
	if !strings.Contains(out, "; DTMF function map") {
		t.Fatalf("unrelated comment lost, got:\n%s", out)
	}
}

func TestSetNewKeyAppendsToSection(t *testing.T) {
	f, err := Parse(strings.NewReader(rptFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	f.Set("2000", "rpt_stats", "3")
	v, ok := f.Get("2000", "rpt_stats")
	if !ok || v != "3" {
		t.Fatalf("new key not set: %q, %v", v, ok)
	}
	// Must be appended within the [2000] section, not e.g. before [nodes]
	// content or after EOF outside any section.
	lines := f.SectionKeys("2000")
	if lines[len(lines)-1].Key != "rpt_stats" {
		t.Fatalf("new key not appended at end of section, got %+v", lines)
	}
}

func TestSetNewKeyPreservesTrailingBlankLine(t *testing.T) {
	// [nodes]'s body includes the blank line separating it from [2000];
	// a newly appended "3000 = ..." entry must land before that blank
	// line, not after it, so the blank line keeps separating sections.
	f, err := Parse(strings.NewReader(rptFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	f.Set("nodes", "3000", "radio@127.0.0.1:4569/3000,NONE")
	out := f.String()
	want := "2000 = radio@127.0.0.1:4569/2000,NONE\n3000 = radio@127.0.0.1:4569/3000,NONE\n\n[2000]"
	if !strings.Contains(out, want) {
		t.Fatalf("expected new entry before trailing blank line, got:\n%s", out)
	}
}

func TestSetNewSection(t *testing.T) {
	f, err := Parse(strings.NewReader(rptFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	f.Set("3000", "rxchannel", "USBRADIO/usb1")
	if !f.HasSection("3000") {
		t.Fatalf("section 3000 not created")
	}
	v, ok := f.Get("3000", "rxchannel")
	if !ok || v != "USBRADIO/usb1" {
		t.Fatalf("Get(3000, rxchannel) = %q, %v", v, ok)
	}
}

func TestDelete(t *testing.T) {
	f, err := Parse(strings.NewReader(rptFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !f.Delete("2000", "telemetry") {
		t.Fatalf("Delete(2000, telemetry) should report true")
	}
	if _, ok := f.Get("2000", "telemetry"); ok {
		t.Fatalf("telemetry should be gone")
	}
	if f.Delete("2000", "telemetry") {
		t.Fatalf("second Delete should report false")
	}
}

func TestAppendKeyValueAddsDuplicateKey(t *testing.T) {
	f, err := Parse(strings.NewReader(`[general]
register => 2000:pass1@example.com
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	f.AppendKeyValue("general", "register", OpArrow, "3000:pass2@example.com")

	all := f.GetAll("general", "register")
	if len(all) != 2 {
		t.Fatalf("GetAll(register) = %v, want 2 entries", all)
	}
	if all[0] != "2000:pass1@example.com" || all[1] != "3000:pass2@example.com" {
		t.Fatalf("GetAll(register) = %v", all)
	}
}

func TestSetDoesNotDuplicateOnRepeatedCalls(t *testing.T) {
	// Guards against Set's fallback-to-append path accidentally adding a
	// second line for a key that already exists (the bug AppendKeyValue
	// was split out to avoid conflating with Set's "unique key" contract).
	f, err := Parse(strings.NewReader(`[section]
key = 1
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	f.Set("section", "key", "2")
	f.Set("section", "key", "3")
	all := f.GetAll("section", "key")
	if len(all) != 1 || all[0] != "3" {
		t.Fatalf("GetAll(key) = %v, want [3]", all)
	}
}

func TestDeleteValueRemovesOnlyMatchingLine(t *testing.T) {
	f, err := Parse(strings.NewReader(`[radio-secure]
exten => 68536,1,rpt,68536
exten => 52829,1,rpt,52829
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !f.DeleteValue("radio-secure", "exten", "52829,1,rpt,52829") {
		t.Fatalf("DeleteValue should report true")
	}
	all := f.GetAll("radio-secure", "exten")
	if len(all) != 1 || all[0] != "68536,1,rpt,68536" {
		t.Fatalf("GetAll(exten) after DeleteValue = %v, want [68536,1,rpt,68536]", all)
	}
	if f.DeleteValue("radio-secure", "exten", "52829,1,rpt,52829") {
		t.Fatalf("second DeleteValue for the same value should report false")
	}
}

func TestDeleteSection(t *testing.T) {
	f, err := Parse(strings.NewReader(rptFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !f.DeleteSection("2000") {
		t.Fatalf("DeleteSection(2000) should report true")
	}
	if f.HasSection("2000") {
		t.Fatalf("section 2000 should be gone")
	}
	// [nodes] and its content must be untouched.
	if !f.HasSection("nodes") {
		t.Fatalf("unrelated section [nodes] should survive")
	}
	if v, ok := f.Get("nodes", "2000"); !ok || v != "radio@127.0.0.1:4569/2000,NONE" {
		t.Fatalf("nodes entry corrupted: %q, %v", v, ok)
	}
	if f.DeleteSection("2000") {
		t.Fatalf("second DeleteSection should report false")
	}
}

// extensionsFixture exercises duplicate keys and "=>" operator, as seen
// in extensions.conf ("exten =>" lines repeat the same key many times).
const extensionsFixture = `[macro-autopatchup]
exten => s,1,Set(TIMEOUT(digit)=5)
exten => s,n,Set(TIMEOUT(response)=10)
exten => s,n,Authenticate(1234)
exten => s,n,Wait(1)
`

func TestGetAllDuplicateKeys(t *testing.T) {
	f, err := Parse(strings.NewReader(extensionsFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	all := f.GetAll("macro-autopatchup", "exten")
	if len(all) != 4 {
		t.Fatalf("GetAll(exten) len = %d, want 4: %v", len(all), all)
	}
	if all[0] != "s,1,Set(TIMEOUT(digit)=5)" {
		t.Fatalf("unexpected first exten value: %q", all[0])
	}
}

func TestExtensionsRoundTrip(t *testing.T) {
	f, err := Parse(strings.NewReader(extensionsFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := f.String(); got != extensionsFixture {
		t.Fatalf("round trip mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, extensionsFixture)
	}
}

func TestSectionKeysOrderPreserved(t *testing.T) {
	f, err := Parse(strings.NewReader(extensionsFixture))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	kv := f.SectionKeys("macro-autopatchup")
	want := []string{
		"s,1,Set(TIMEOUT(digit)=5)",
		"s,n,Set(TIMEOUT(response)=10)",
		"s,n,Authenticate(1234)",
		"s,n,Wait(1)",
	}
	if len(kv) != len(want) {
		t.Fatalf("got %d kv pairs, want %d", len(kv), len(want))
	}
	for i, w := range want {
		if kv[i].Value != w {
			t.Fatalf("kv[%d].Value = %q, want %q", i, kv[i].Value, w)
		}
	}
}
