package config

import (
	"strings"

	"hamvoipconfiggui/internal/asteriskconf"
)

// ExtensionsConfFile is the standard HamVoIP dialplan config file name.
const ExtensionsConfFile = "extensions.conf"

// nodeExtenContexts lists, for each dialplan context this app manages,
// the exact "exten" line a node number needs there. Confirmed against a
// real HamVoIP node-config.sh generated extensions.conf: [radio-secure]
// is the context iax.conf's [radio] peer actually routes incoming
// AllStarLink connections through, so a node with no entry there can't
// be connected to at all, regardless of what rpt.conf says. [radio-
// secure-proxy] and [radio-iaxrpt] cover the proxy and Windows IAXRPT
// connection methods the same template sets up for its first node. The
// mixed "=>"/"=" styles and rpt/Rpt casing below match the template
// itself exactly, not a typo.
var nodeExtenContexts = []struct {
	section string
	op      asteriskconf.Operator
	value   func(node string) string
}{
	{"radio-secure", asteriskconf.OpArrow, func(n string) string { return n + ",1,rpt," + n }},
	{"radio-secure-proxy", asteriskconf.OpArrow, func(n string) string { return n + ",1,rpt," + n + "|X" }},
	{"radio-iaxrpt", asteriskconf.OpEquals, func(n string) string { return n + ",1,Rpt," + n + "|X" }},
}

// extenNodeNumber returns the extension name — the part before the
// first comma — of an "exten" line's value, e.g. "68536" from
// "68536,1,rpt,68536".
func extenNodeNumber(value string) string {
	if i := strings.Index(value, ","); i >= 0 {
		return value[:i]
	}
	return value
}

// findExtenValueForNode returns the raw value of the first "exten" line
// in section belonging to node (matched by extension name, not exact
// value, so a hand-customized line for this node is still found).
func findExtenValueForNode(f *asteriskconf.File, section, node string) (string, bool) {
	for _, kv := range f.SectionKeys(section) {
		if kv.Key == "exten" && extenNodeNumber(kv.Value) == node {
			return kv.Value, true
		}
	}
	return "", false
}

// EnsureNodeExtensions adds the "exten" lines node needs across
// extensions.conf's [radio-secure]/[radio-secure-proxy]/[radio-iaxrpt]
// contexts, skipping any context that already has an entry for this
// node (under any value, not just an exact match) so it never
// duplicates or clobbers a hand-customized line. Safe to call
// repeatedly, including on a node that already has some or all of these
// — e.g. to backfill a node that existed before this app managed
// extensions.conf. Missing contexts are created as new sections at the
// end of the file, though on a stock HamVoIP install they're always
// already present.
func (s *Store) EnsureNodeExtensions(node string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(ExtensionsConfFile)
	if err != nil {
		return err
	}
	changed := false
	for _, ctx := range nodeExtenContexts {
		if _, ok := findExtenValueForNode(f, ctx.section, node); ok {
			continue
		}
		f.AppendKeyValue(ctx.section, "exten", ctx.op, ctx.value(node))
		changed = true
	}
	if !changed {
		return nil
	}
	return s.save(ExtensionsConfFile, f)
}

// RemoveNodeExtensions removes any "exten" lines for node from the same
// contexts EnsureNodeExtensions manages, regardless of their exact
// value, so a line that was hand-edited after being created still gets
// cleaned up. Safe to call even if none exist.
func (s *Store) RemoveNodeExtensions(node string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(ExtensionsConfFile)
	if err != nil {
		return err
	}
	changed := false
	for _, ctx := range nodeExtenContexts {
		for {
			value, ok := findExtenValueForNode(f, ctx.section, node)
			if !ok {
				break
			}
			f.DeleteValue(ctx.section, "exten", value)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.save(ExtensionsConfFile, f)
}
