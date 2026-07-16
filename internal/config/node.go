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

// DeleteNode removes a node's [nodes] entry and its per-node section
// entirely.
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
