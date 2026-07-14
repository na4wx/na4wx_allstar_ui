package config

import "fmt"

// RptConfFile is the standard HamVoIP app_rpt config file name.
const RptConfFile = "rpt.conf"

// Node is the identity/registration profile for one AllStar node, backed
// by rpt.conf's [nodes] entry (the local IAX dial string used by app_rpt
// to reach the node) and its per-node stanza (the fields well documented
// in app_rpt's sample rpt.conf). Internet-facing IAX2 registration
// (iax.conf's "register =>" line, i.e. the credentials that connect this
// node to the wider AllStarLink network) is edited separately via the
// generic config editor, since its section layout varies more between
// HamVoIP releases.
type Node struct {
	Number string // e.g. "2000"; the [nodes] key and per-node section name

	// DialString is the raw value of the [nodes] entry, conventionally
	// "radio@host:port/node,NONE". Kept as opaque text so unusual
	// formats round-trip untouched; ParsedHost/ParsedPort are a
	// best-effort convenience for the UI only.
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

// ListNodes returns node numbers from rpt.conf's [nodes] section, in
// file order.
func (s *Store) ListNodes() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return nil, err
	}
	kv := f.SectionKeys("nodes")
	out := make([]string, 0, len(kv))
	for _, p := range kv {
		out = append(out, p.Key)
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

// SaveNode writes n back to rpt.conf. If n.Number is not already present
// in [nodes], both the section and the [nodes] entry are created.
// Renaming a node (changing Number on an existing node) is not
// supported here — delete and recreate instead, since it would also
// require updating every other place the old number is referenced
// (dialplan, other nodes' link lists, etc.).
func (s *Store) SaveNode(n *Node) error {
	if n.Number == "" {
		return fmt.Errorf("config: node number is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(RptConfFile)
	if err != nil {
		return err
	}

	f.EnsureSection("nodes")
	if n.DialString != "" {
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
