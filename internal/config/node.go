package config

import (
	"fmt"
	"regexp"
)

// RptConfFile is the standard HamVoIP app_rpt config file name.
const RptConfFile = "rpt.conf"

// nodeSectionRe matches a node's own rpt.conf section name, e.g. "68536"
// — always purely numeric, per the node number field's own validation
// (node_form.html's pattern="[0-9]+"). This is what distinguishes a
// node's section from the many other sections HamVoIP's node-config.sh
// generates in the same file (morse68536, controlstates, schedule68536,
// events68536, functions68536, macro68536, telemetry68536,
// wait-times68536, nodes, ...) — all of which mix in a word, so none of
// them collide with this pattern.
var nodeSectionRe = regexp.MustCompile(`^[0-9]+$`)

// Node is the identity/registration profile for one AllStar node, backed
// by its own numbered rpt.conf stanza (the fields well documented in
// app_rpt's sample rpt.conf). Internet-facing IAX2 registration (iax.conf's
// "register =>" line, i.e. the credentials that connect this node to the
// wider AllStarLink network) is edited separately via the generic config
// editor, since its section layout varies more between HamVoIP releases.
type Node struct {
	Number string // e.g. "2000"; the per-node section name

	// DialString is the raw value of this node's entry in rpt.conf's
	// [nodes] section, conventionally "radio@host:port/node,NONE" —
	// but confirmed, from a real HamVoIP-generated rpt.conf's own
	// comments, that [nodes] is NOT a master registry every node needs
	// an entry in: it's documented there as being for local-LAN-only or
	// private (off of AllStarLink) node aliases specifically, and a
	// normal AllStarLink-registered node has no entry there at all —
	// just its own numbered section. So this is usually empty, and
	// that's normal, not a sign of a broken node.
	DialString string

	RXChannel   string // e.g. "USBRADIO/usb" or "Voter/125"
	TXChannel   string // usually blank (defaults to RXChannel)
	Duplex      string // "0".."4"
	Telemetry   string
	Morse       string
	Functions   string
	Macro       string // which rpt.conf stanza holds this node's saved macros (see the "macro,<n>" function)
	HangTime    string // ms
	AltHangTime string // ms
	TOTime      string // ms, transmit timeout
	IDTime      string // ms between IDs
	IDRecording string // sound file/macro used for station ID

	// These three decide WHICH of the telemetry section's ct1-ct8
	// courtesy tones actually plays in which situation — confirmed
	// against AllStarLink's own rpt.conf documentation and cross-checked
	// against a real node's own inline comments (unlinkedct=ct2,
	// remotect=ct3, linkunkeyct=ct8). Without these, a courtesy tone
	// name like "ct2" means nothing on its own; the "Tones & Audio" UI
	// uses these fields to label each one with what it's actually used
	// for on this specific node, rather than guessing or hardcoding a
	// universal meaning that could be wrong for a node that assigns
	// them differently.
	UnlinkedCT  string // courtesy tone played when not connected to any other node
	RemoteCT    string // courtesy tone played when a remote base is connected locally
	LinkUnkeyCT string // courtesy tone played when a connected/linked node unkeys

	// Scheduler names this node's [scheduleNNNN] section — app_rpt's own
	// native cron-like mechanism (entries there are "<macro number> = MM
	// HH DOM MON DOW"; Asterisk itself dials the referenced macro when the
	// time matches, no separate process needed). Unlike
	// functions/macro/telemetry/morse, a blank Scheduler is never
	// dangerous on its own — app_rpt simply has nothing scheduled, there's
	// no shared bare-name section it silently falls back to relying on —
	// so this is read directly here but, like UnlinkedCT etc., kept out of
	// nodeFields/SaveNode's whole-node resubmit: it's written only through
	// the narrow SetNodeScheduler, by the "Automation" tab's own scoped
	// form, so an ordinary Setup-tab save (which never carries a
	// "scheduler" field at all) can never blank it out.
	Scheduler string
}

// nodeFields lists the per-node-section keys mapped onto Node, in the
// order they should appear when a brand new section is created.
var nodeFields = []struct {
	key string
	get func(*Node) *string
}{
	{"rxchannel", func(n *Node) *string { return &n.RXChannel }},
	{"txchannel", func(n *Node) *string { return &n.TXChannel }},
	{"duplex", func(n *Node) *string { return &n.Duplex }},
	{"telemetry", func(n *Node) *string { return &n.Telemetry }},
	{"morse", func(n *Node) *string { return &n.Morse }},
	{"functions", func(n *Node) *string { return &n.Functions }},
	{"macro", func(n *Node) *string { return &n.Macro }},
	{"hangtime", func(n *Node) *string { return &n.HangTime }},
	{"althangtime", func(n *Node) *string { return &n.AltHangTime }},
	{"totime", func(n *Node) *string { return &n.TOTime }},
	{"idtime", func(n *Node) *string { return &n.IDTime }},
	{"idrecording", func(n *Node) *string { return &n.IDRecording }},
}

// courtesyToneFields lists the unlinkedct/remotect/linkunkeyct keys.
// Deliberately kept out of nodeFields: those three are edited via their
// own "Tones & Audio" tab form, which posts only these three keys, so
// SaveNode's whole-node resubmit must never touch them — otherwise every
// ordinary Setup-tab save (which no longer carries these fields at all)
// would blank them out. LoadNode still reads them directly, below. See
// SetCourtesyToneAssignments for the narrow write path.
var courtesyToneFields = []struct {
	key string
	get func(*Node) *string
}{
	{"unlinkedct", func(n *Node) *string { return &n.UnlinkedCT }},
	{"remotect", func(n *Node) *string { return &n.RemoteCT }},
	{"linkunkeyct", func(n *Node) *string { return &n.LinkUnkeyCT }},
}

// ListNodes returns node numbers found in rpt.conf: every section whose
// name is purely numeric (see nodeSectionRe), in file order. This
// deliberately does not depend on rpt.conf's [nodes] section having an
// entry for each node — see the Node.DialString doc comment for why.
func (s *Store) ListNodes() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, sec := range f.Sections() {
		if nodeSectionRe.MatchString(sec) {
			out = append(out, sec)
		}
	}
	return out, nil
}

// LoadNode reads a single node's identity from rpt.conf.
func (s *Store) LoadNode(number string) (*Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return nil, err
	}
	if !f.HasSection(number) {
		return nil, fmt.Errorf("config: node %s not found in %s", number, RptConfFile)
	}
	n := &Node{Number: number}
	if dial, ok := f.Get("nodes", number); ok {
		n.DialString = dial
	}
	for _, fld := range nodeFields {
		if v, ok := f.Get(number, fld.key); ok {
			*fld.get(n) = v
		}
	}
	for _, fld := range courtesyToneFields {
		if v, ok := f.Get(number, fld.key); ok {
			*fld.get(n) = v
		}
	}
	if v, ok := f.Get(number, "scheduler"); ok {
		n.Scheduler = v
	}
	return n, nil
}

// SaveNode writes n back to rpt.conf. The [nodes] entry is only written
// when DialString is explicitly set — see the Node.DialString doc
// comment for why an empty one is normal and should stay absent rather
// than being defaulted to something. Renaming a node (changing Number
// on an existing node) is not supported here — delete and recreate
// instead, since it would also require updating every other place the
// old number is referenced (dialplan, other nodes' link lists, etc.).
func (s *Store) SaveNode(n *Node) error {
	if n.Number == "" {
		return fmt.Errorf("config: node number is required")
	}
	if !nodeSectionRe.MatchString(n.Number) {
		return fmt.Errorf("config: node number must be numeric")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}

	if n.DialString != "" {
		f.EnsureSection("nodes")
		f.Set("nodes", n.Number, n.DialString)
	}

	f.EnsureSection(n.Number)
	for _, fld := range nodeFields {
		v := *fld.get(n)
		if v == "" {
			f.Delete(n.Number, fld.key)
			continue
		}
		f.Set(n.Number, fld.key, v)
	}

	return s.save(RptConfFile, f)
}

// SetCourtesyToneAssignments updates just unlinkedct/remotect/linkunkeyct
// on number's own rpt.conf section, leaving every other field on that
// section untouched — the narrow-update counterpart to SaveNode's
// whole-node rewrite, used by the "Tones & Audio" tab's own scoped form
// (see SaveNode's courtesyToneFields comment for why these three can't
// go through SaveNode). Blank clears a key, falling back to app_rpt's own
// default for that situation, matching SaveNode's blank-means-delete
// convention for every other field.
func (s *Store) SetCourtesyToneAssignments(number, unlinkedCT, remoteCT, linkUnkeyCT string) error {
	if !nodeSectionRe.MatchString(number) {
		return fmt.Errorf("config: node number must be numeric")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	if !f.HasSection(number) {
		return fmt.Errorf("config: node %s not found in %s", number, RptConfFile)
	}
	set := func(key, value string) {
		if value == "" {
			f.Delete(number, key)
			return
		}
		f.Set(number, key, value)
	}
	set("unlinkedct", unlinkedCT)
	set("remotect", remoteCT)
	set("linkunkeyct", linkUnkeyCT)
	return s.save(RptConfFile, f)
}

// SetNodeScheduler updates just the scheduler key on number's own rpt.conf
// section — the narrow-update counterpart to SaveNode's whole-node
// rewrite, used to self-heal a blank Node.Scheduler (a node created before
// the "Automation" tab existed) to a correctly-named section the first
// time an automation entry is saved for it, without risking any other
// field (see the Scheduler field's own doc comment for why it can't go
// through SaveNode). Blank clears the key, leaving the node with no
// scheduled events.
func (s *Store) SetNodeScheduler(number, section string) error {
	if !nodeSectionRe.MatchString(number) {
		return fmt.Errorf("config: node number must be numeric")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	if !f.HasSection(number) {
		return fmt.Errorf("config: node %s not found in %s", number, RptConfFile)
	}
	if section == "" {
		f.Delete(number, "scheduler")
	} else {
		f.Set(number, "scheduler", section)
	}
	return s.save(RptConfFile, f)
}

// companionSections lists the five per-node auxiliary section types a
// real HamVoIP node uses: functions is its DTMF command map (what makes
// *3<node> etc. work at all), macro its saved multi-step sequences,
// telemetry its courtesy tones, morse its CW ID sound set, scheduler its
// app_rpt-native day/time schedule (see the Scheduler field's doc
// comment). defaultName is the bare section name app_rpt itself falls
// back to using when a node's own field is left blank — matching that
// exactly matters for the first four, since a node whose field is blank
// is silently relying on that section existing, not on having no command
// set; scheduler has no such danger (a blank one is simply "nothing
// scheduled"), but Clone/Normalize still give a cloned/repaired node its
// own correctly-named section for consistency with the other four.
var companionSections = []struct {
	nodeKey     string
	defaultName string
}{
	{"functions", "functions"},
	{"macro", "macro"},
	{"telemetry", "telemetry"},
	{"morse", "morse"},
	{"scheduler", "schedule"},
}

// CloneNodeConfig gives dstNumber its own working copy of srcNumber's
// functions/macro/telemetry/morse sections — new sections named e.g.
// "functions"+dstNumber, containing the same entries as whatever
// srcNumber currently uses — and points dstNumber's own fields at them.
//
// This exists because this app's node creation only ever wrote a node's
// identity stanza (rxchannel, duplex, timing), never a working command
// set: a newly created node's functions/macro/telemetry/morse fields
// are blank, which makes app_rpt fall back to looking for plain
// "functions"/"macro"/"telemetry"/"morse" sections that don't exist on
// a real HamVoIP install (only the number-suffixed ones from whichever
// node node-config.sh originally set up do) — so the new node silently
// accepts no DTMF commands at all. Cloning a working node's sections is
// the fix, both for giving a brand new node a complete set at creation
// time and for repairing an existing node that was created before this
// existed.
//
// Safe to call more than once: each destination section is rebuilt from
// scratch from the source's current content, so re-running it after
// editing the source re-syncs the clone rather than duplicating entries.
func (s *Store) CloneNodeConfig(srcNumber, dstNumber string) error {
	if srcNumber == "" || dstNumber == "" {
		return fmt.Errorf("config: source and destination node numbers are required")
	}
	if srcNumber == dstNumber {
		return fmt.Errorf("config: source and destination nodes must be different")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	if !f.HasSection(srcNumber) {
		return fmt.Errorf("config: source node %s not found in %s", srcNumber, RptConfFile)
	}
	if !f.HasSection(dstNumber) {
		return fmt.Errorf("config: destination node %s not found in %s", dstNumber, RptConfFile)
	}

	for _, cs := range companionSections {
		srcSection, _ := f.Get(srcNumber, cs.nodeKey)
		if srcSection == "" {
			srcSection = cs.defaultName
		}
		dstSection := cs.defaultName + dstNumber

		if f.HasSection(srcSection) {
			for _, kv := range f.SectionKeys(srcSection) {
				f.Set(dstSection, kv.Key, kv.Value)
			}
		} else {
			f.EnsureSection(dstSection)
		}
		f.Set(dstNumber, cs.nodeKey, dstSection)
	}

	return s.save(RptConfFile, f)
}

// NormalizeNodeConfig makes number self-contained: for each of its
// functions/macro/telemetry/morse fields, if the section it points at
// isn't this app's own <prefix><number> name, that section's contents
// are copied into a correctly-named one and the field is repointed
// there.
//
// This repairs a node set up by hand or by HamVoIP's node-config.sh —
// the classic case being one whose [52829] section still points at
// functions1998 because only the section header was renamed from the
// shipped template, leaving the node borrowing another (possibly
// nonexistent) node's command set. After this runs, the node owns
// sections named for itself, so editing or deleting it through this app
// affects only its own config.
//
// A blank field is treated as pointing at app_rpt's bare fallback name
// (e.g. "functions"), which is normalized the same way — that bare
// section usually doesn't exist on a stock install, so the result is an
// empty but correctly-named section the operator can then fill via the
// command list.
//
// Section contents are copied verbatim. Command or telemetry values that
// embed a node number as a literal argument (e.g. a saytime script call)
// are deliberately NOT rewritten: blindly substituting a number inside
// an arbitrary command could corrupt a value that legitimately refers to
// a different node. The old source sections are left in place, since
// they may be shared with another node and removing them safely needs a
// cross-node check this method can't make on its own.
//
// Returns the fields that were changed (e.g. "functions", "morse"), so
// the caller can tell the operator whether anything actually needed
// repair. A node already using correctly-named sections yields an empty
// list and no write.
func (s *Store) NormalizeNodeConfig(number string) ([]string, error) {
	if number == "" {
		return nil, fmt.Errorf("config: node number is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return nil, err
	}
	if !f.HasSection(number) {
		return nil, fmt.Errorf("config: node %s not found in %s", number, RptConfFile)
	}

	var changed []string
	for _, cs := range companionSections {
		current, _ := f.Get(number, cs.nodeKey)
		if current == "" {
			current = cs.defaultName
		}
		desired := cs.defaultName + number
		if current == desired {
			continue
		}
		// Rebuild the correctly-named section from scratch out of the
		// current one's content (or empty if the source doesn't exist),
		// then repoint the node at it. current != desired is guaranteed
		// here, so clearing desired never touches the source.
		f.DeleteSection(desired)
		if f.HasSection(current) {
			for _, kv := range f.SectionKeys(current) {
				f.Set(desired, kv.Key, kv.Value)
			}
		} else {
			f.EnsureSection(desired)
		}
		f.Set(number, cs.nodeKey, desired)
		changed = append(changed, cs.nodeKey)
	}

	if len(changed) == 0 {
		return nil, nil
	}
	if err := s.save(RptConfFile, f); err != nil {
		return nil, err
	}
	return changed, nil
}

// DeleteNode removes a node's [nodes] entry and its per-node section
// entirely. This does not touch its functions/macro/telemetry/morse
// companion sections (see CloneNodeConfig) — those might still be
// referenced by another node, so removing them safely needs a
// cross-node check only the caller (which can see every node) can make;
// see the server package's node-delete handler.
func (s *Store) DeleteNode(number string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	f.Delete("nodes", number)
	f.DeleteSection(number)
	return s.save(RptConfFile, f)
}

// DeleteRptSection removes an arbitrary rpt.conf section by name. Used
// to clean up a deleted node's functions/macro/telemetry/morse
// companion sections once the caller has confirmed no other node still
// references them — see CloneNodeConfig/ApplyStandardCommandSet, which
// create these sections.
func (s *Store) DeleteRptSection(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}
	f.DeleteSection(name)
	return s.save(RptConfFile, f)
}
