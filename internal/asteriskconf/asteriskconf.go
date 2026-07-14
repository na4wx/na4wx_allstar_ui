// Package asteriskconf reads and writes Asterisk-style .conf files
// (rpt.conf, iax.conf, usbradio.conf, extensions.conf, ...) while
// preserving comments, blank lines, indentation, and key order. HamVoIP's
// shipped configs are heavily hand-annotated, so a generic INI library
// that discards comments is not acceptable here.
package asteriskconf

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Operator distinguishes Asterisk's two assignment forms. They are
// semantically identical for most files, but which one is used matters
// for round-tripping (extensions.conf conventionally uses "=>" for
// exten/include/ignorepat, while most key/value settings use "=").
type Operator string

const (
	OpEquals Operator = "="
	OpArrow  Operator = "=>"
)

// LineKind identifies what a parsed line represents.
type LineKind int

const (
	KindBlank LineKind = iota
	KindComment
	KindDirective // #include, #exec
	KindSection
	KindKeyValue
)

// Line is one line of a conf file. Unmodified lines round-trip via Raw;
// once a KeyValue line is edited via File.Set, it is re-rendered from its
// structured fields instead.
type Line struct {
	Kind LineKind
	Raw  string

	// KindSection
	Section     string // name inside [ ]
	SectionMeta string // optional (flags/template) suffix, e.g. "(+)" or "(!)"

	// KindKeyValue
	Key      string
	Value    string
	Operator Operator

	// Shared formatting, preserved on edit so re-rendered lines still
	// look like the surrounding file.
	LeadingWS     string
	InlineComment string // includes the leading ";" and any spacing before it

	dirty bool // if true, Raw is stale and must be re-rendered from fields
}

// SetValue updates a KindKeyValue line's value in place, preserving its
// operator, indentation, and inline comment.
func (l *Line) SetValue(value string) {
	l.Value = value
	l.dirty = true
}

func (l *Line) render() string {
	if !l.dirty {
		return l.Raw
	}
	var b strings.Builder
	b.WriteString(l.LeadingWS)
	switch l.Kind {
	case KindSection:
		b.WriteString("[")
		b.WriteString(l.Section)
		b.WriteString("]")
		b.WriteString(l.SectionMeta)
	case KindKeyValue:
		b.WriteString(l.Key)
		b.WriteString(" ")
		b.WriteString(string(l.Operator))
		b.WriteString(" ")
		b.WriteString(l.Value)
	default:
		b.WriteString(l.Raw)
	}
	if l.InlineComment != "" {
		b.WriteString(" ")
		b.WriteString(l.InlineComment)
	}
	return b.String()
}

// File is a parsed Asterisk config file: an ordered list of lines plus
// enough structure to query/edit sections and key/value pairs without
// disturbing anything else.
type File struct {
	Lines []*Line
}

// Parse reads an Asterisk-style config file from r.
func Parse(r io.Reader) (*File, error) {
	f := &File{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	currentSection := ""
	for scanner.Scan() {
		line := scanner.Text()
		f.Lines = append(f.Lines, parseLine(line, &currentSection))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("asteriskconf: scan: %w", err)
	}
	return f, nil
}

// ParseFile opens and parses path.
func ParseFile(path string) (*File, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return Parse(fh)
}

func parseLine(raw string, currentSection *string) *Line {
	trimmed := strings.TrimLeft(raw, " \t")
	leadingWS := raw[:len(raw)-len(trimmed)]

	if trimmed == "" {
		return &Line{Kind: KindBlank, Raw: raw}
	}
	if strings.HasPrefix(trimmed, ";") {
		return &Line{Kind: KindComment, Raw: raw}
	}
	if strings.HasPrefix(trimmed, "#") {
		return &Line{Kind: KindDirective, Raw: raw}
	}
	if strings.HasPrefix(trimmed, "[") {
		if end := strings.IndexByte(trimmed, ']'); end >= 0 {
			name := trimmed[1:end]
			meta := ""
			rest := trimmed[end+1:]
			// meta is any "(...)" immediately following, e.g. (+) or (!)
			if strings.HasPrefix(rest, "(") {
				if close := strings.IndexByte(rest, ')'); close >= 0 {
					meta = rest[:close+1]
				}
			}
			*currentSection = name
			return &Line{
				Kind:        KindSection,
				Raw:         raw,
				Section:     name,
				SectionMeta: meta,
				LeadingWS:   leadingWS,
			}
		}
	}

	// key/value: split on the first "=>" or "=", whichever comes first.
	opIdx, op := -1, OpEquals
	if i := strings.Index(trimmed, "=>"); i >= 0 {
		opIdx, op = i, OpArrow
	}
	if i := strings.Index(trimmed, "="); i >= 0 && (opIdx == -1 || i < opIdx) {
		opIdx, op = i, OpEquals
	}
	if opIdx == -1 {
		// Not a recognizable key/value line; preserve verbatim.
		return &Line{Kind: KindComment, Raw: raw}
	}

	key := strings.TrimSpace(trimmed[:opIdx])
	rest := trimmed[opIdx+len(op):]

	value, inlineComment := splitInlineComment(rest)

	return &Line{
		Kind:          KindKeyValue,
		Raw:           raw,
		Key:           key,
		Value:         strings.TrimSpace(value),
		Operator:      op,
		LeadingWS:     leadingWS,
		InlineComment: inlineComment,
	}
}

// splitInlineComment separates a value from a trailing "; comment",
// respecting that ';' inside quotes is not a comment delimiter.
func splitInlineComment(s string) (value, comment string) {
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case ';':
			if !inQuote {
				return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i:])
			}
		}
	}
	return strings.TrimSpace(s), ""
}

// String reconstructs the file's text.
func (f *File) String() string {
	var b strings.Builder
	for _, l := range f.Lines {
		b.WriteString(l.render())
		b.WriteString("\n")
	}
	return b.String()
}

// WriteFile writes the reconstructed file to path with the given
// permissions, replacing its contents.
func (f *File) WriteFile(path string, perm os.FileMode) error {
	return os.WriteFile(path, []byte(f.String()), perm)
}

// Sections returns section names in file order (first occurrence).
func (f *File) Sections() []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range f.Lines {
		if l.Kind == KindSection && !seen[l.Section] {
			seen[l.Section] = true
			out = append(out, l.Section)
		}
	}
	return out
}

// HasSection reports whether name exists.
func (f *File) HasSection(name string) bool {
	for _, l := range f.Lines {
		if l.Kind == KindSection && l.Section == name {
			return true
		}
	}
	return false
}

// sectionBounds returns the line index range [start, end) covering
// section's body (i.e. everything up to but not including the next
// section header or EOF). It does not include the header line itself.
// Returns ok=false if the section does not exist.
func (f *File) sectionBounds(name string) (start, end int, ok bool) {
	for i, l := range f.Lines {
		if l.Kind == KindSection && l.Section == name {
			start = i + 1
			ok = true
			for end = start; end < len(f.Lines); end++ {
				if f.Lines[end].Kind == KindSection {
					break
				}
			}
			return
		}
	}
	return 0, 0, false
}

// KV is a single key/value pair with its source position.
type KV struct {
	Key   string
	Value string
}

// SectionKeys returns the key/value pairs within section, in file order,
// including duplicates (e.g. extensions.conf "exten =>" lines).
func (f *File) SectionKeys(section string) []KV {
	start, end, ok := f.sectionBounds(section)
	if !ok {
		return nil
	}
	var out []KV
	for i := start; i < end; i++ {
		l := f.Lines[i]
		if l.Kind == KindKeyValue {
			out = append(out, KV{Key: l.Key, Value: l.Value})
		}
	}
	return out
}

// Get returns the first value for key within section.
func (f *File) Get(section, key string) (string, bool) {
	start, end, ok := f.sectionBounds(section)
	if !ok {
		return "", false
	}
	for i := start; i < end; i++ {
		l := f.Lines[i]
		if l.Kind == KindKeyValue && l.Key == key {
			return l.Value, true
		}
	}
	return "", false
}

// GetAll returns every value for key within section, in file order.
func (f *File) GetAll(section, key string) []string {
	start, end, ok := f.sectionBounds(section)
	if !ok {
		return nil
	}
	var out []string
	for i := start; i < end; i++ {
		l := f.Lines[i]
		if l.Kind == KindKeyValue && l.Key == key {
			out = append(out, l.Value)
		}
	}
	return out
}

// Set updates the first occurrence of key within section, preserving its
// operator, indentation, and inline comment. If the key does not exist,
// a new "key = value" line is appended to the end of the section. If the
// section does not exist, it is created at the end of the file.
//
// Set assumes key is unique within section. For keys that legitimately
// repeat (iax.conf's "register =>", extensions.conf's "exten =>"), use
// AppendKeyValue instead — Set would otherwise overwrite the first
// existing occurrence rather than adding a new one.
func (f *File) Set(section, key, value string) {
	start, end, ok := f.sectionBounds(section)
	if !ok {
		f.AppendKeyValue(section, key, OpEquals, value)
		return
	}
	for i := start; i < end; i++ {
		l := f.Lines[i]
		if l.Kind == KindKeyValue && l.Key == key {
			l.Value = value
			l.dirty = true
			return
		}
	}
	f.AppendKeyValue(section, key, OpEquals, value)
}

// AppendKeyValue always adds a new "key <op> value" line at the end of
// section's key/value lines, regardless of whether key already appears
// there. Creates section (at the end of the file) if it doesn't exist.
func (f *File) AppendKeyValue(section, key string, op Operator, value string) {
	start, end, ok := f.sectionBounds(section)
	if !ok {
		f.Lines = append(f.Lines, &Line{Kind: KindSection, Section: section, dirty: true})
		f.Lines = append(f.Lines, &Line{Kind: KindKeyValue, Key: key, Value: value, Operator: op, dirty: true})
		return
	}
	// New lines go after the last existing key/value line, so trailing
	// blank lines/comments that separate this section from the next
	// stay trailing rather than getting split.
	insertAt := start
	for i := start; i < end; i++ {
		if f.Lines[i].Kind == KindKeyValue {
			insertAt = i + 1
		}
	}
	newLine := &Line{Kind: KindKeyValue, Key: key, Value: value, Operator: op, dirty: true}
	f.Lines = append(f.Lines[:insertAt], append([]*Line{newLine}, f.Lines[insertAt:]...)...)
}

// Delete removes the first occurrence of key within section. Reports
// whether a line was removed.
func (f *File) Delete(section, key string) bool {
	start, end, ok := f.sectionBounds(section)
	if !ok {
		return false
	}
	for i := start; i < end; i++ {
		l := f.Lines[i]
		if l.Kind == KindKeyValue && l.Key == key {
			f.Lines = append(f.Lines[:i], f.Lines[i+1:]...)
			return true
		}
	}
	return false
}

// EnsureSection creates section (with no body) if it does not already
// exist, appending it to the end of the file.
func (f *File) EnsureSection(name string) {
	if f.HasSection(name) {
		return
	}
	f.Lines = append(f.Lines, &Line{Kind: KindSection, Section: name, dirty: true})
}

// DeleteSection removes a section's header and its entire body. Reports
// whether the section was found.
func (f *File) DeleteSection(name string) bool {
	if !f.HasSection(name) {
		return false
	}
	kept := make([]*Line, 0, len(f.Lines))
	skipping := false
	for _, l := range f.Lines {
		if l.Kind == KindSection {
			skipping = l.Section == name
			if skipping {
				continue
			}
		}
		if skipping {
			continue
		}
		kept = append(kept, l)
	}
	f.Lines = kept
	return true
}
