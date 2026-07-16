package config

import (
	"fmt"
	"strings"

	"hamvoipconfiggui/internal/asteriskconf"
)

// IaxConfFile is the standard HamVoIP IAX2 config file name.
const IaxConfFile = "iax.conf"

// Registration is one IAX2 "register =>" line — the credentials that
// connect this node's app_rpt instance to the wider AllStarLink network
// (as opposed to rpt.conf's [nodes] entry, which is only the local IAX
// dial string app_rpt uses to reach its own node). Asterisk allows
// "register =>" in any section (conventionally [general]), so these are
// found and edited by scanning the whole file rather than one section.
type Registration struct {
	Node     string
	Password string
	Host     string
	Port     string // may be empty; Asterisk defaults to 4569
}

func (r Registration) value() string {
	host := r.Host
	if r.Port != "" {
		host += ":" + r.Port
	}
	return fmt.Sprintf("%s:%s@%s", r.Node, r.Password, host)
}

// parseRegistration parses a "register =>" value of the form
// "node:password@host[:port]". Returns ok=false if it doesn't match
// that shape (e.g. a hand-written line this tool shouldn't try to
// round-trip as structured data).
func parseRegistration(value string) (Registration, bool) {
	at := strings.LastIndex(value, "@")
	if at < 0 {
		return Registration{}, false
	}
	cred, hostport := value[:at], value[at+1:]

	colon := strings.Index(cred, ":")
	if colon < 0 {
		return Registration{}, false
	}
	node, password := cred[:colon], cred[colon+1:]

	host, port := hostport, ""
	if i := strings.LastIndex(hostport, ":"); i >= 0 {
		host, port = hostport[:i], hostport[i+1:]
	}

	return Registration{Node: node, Password: password, Host: host, Port: port}, true
}

// ListRegistrations scans iax.conf for every "register =>" line.
func (s *Store) ListRegistrations() ([]Registration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(IaxConfFile)
	if err != nil {
		return nil, err
	}

	var out []Registration
	for _, l := range f.Lines {
		if l.Kind != asteriskconf.KindKeyValue || l.Key != "register" {
			continue
		}
		reg, ok := parseRegistration(l.Value)
		if !ok {
			continue
		}
		out = append(out, reg)
	}
	return out, nil
}

// LoadRegistration returns the registration for a specific node number,
// if one exists.
func (s *Store) LoadRegistration(node string) (*Registration, error) {
	regs, err := s.ListRegistrations()
	if err != nil {
		return nil, err
	}
	for _, r := range regs {
		if r.Node == node {
			return &r, nil
		}
	}
	return nil, nil
}

// SaveRegistration adds or updates the "register =>" line for
// reg.Node. New registrations are added to [general] (the conventional
// location) unless section is already known from a prior load.
func (s *Store) SaveRegistration(reg Registration) error {
	if reg.Node == "" || reg.Password == "" || reg.Host == "" {
		return fmt.Errorf("config: node, password, and host are required for a registration")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(IaxConfFile)
	if err != nil {
		return err
	}

	found := false
	for _, l := range f.Lines {
		if l.Kind != asteriskconf.KindKeyValue || l.Key != "register" {
			continue
		}
		if parsed, ok := parseRegistration(l.Value); ok && parsed.Node == reg.Node {
			l.SetValue(reg.value())
			found = true
			break
		}
	}

	if !found {
		f.EnsureSection("general")
		f.AppendKeyValue("general", "register", asteriskconf.OpArrow, reg.value())
	}

	return s.save(IaxConfFile, f)
}

// DeleteRegistration removes the "register =>" line for node, if any.
func (s *Store) DeleteRegistration(node string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(IaxConfFile)
	if err != nil {
		return err
	}
	kept := make([]*asteriskconf.Line, 0, len(f.Lines))
	for _, l := range f.Lines {
		if l.Kind == asteriskconf.KindKeyValue && l.Key == "register" {
			if parsed, ok := parseRegistration(l.Value); ok && parsed.Node == node {
				continue
			}
		}
		kept = append(kept, l)
	}
	f.Lines = kept
	return s.save(IaxConfFile, f)
}

// Peer is the IAX2 friend/peer stanza matching a node's registration —
// what tells Asterisk how to authenticate and route calls from the
// AllStarLink network for this node.
type Peer struct {
	Node    string
	Type    string // friend | peer | user
	Context string
	Host    string // typically "dynamic"
	Secret  string
	Auth    string // rsa | md5 | plaintext
}

var peerFields = []struct {
	key string
	get func(*Peer) *string
}{
	{"type", func(p *Peer) *string { return &p.Type }},
	{"context", func(p *Peer) *string { return &p.Context }},
	{"host", func(p *Peer) *string { return &p.Host }},
	{"secret", func(p *Peer) *string { return &p.Secret }},
	{"auth", func(p *Peer) *string { return &p.Auth }},
}

// LoadPeer reads the [<node>] friend/peer stanza from iax.conf.
func (s *Store) LoadPeer(node string) (*Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(IaxConfFile)
	if err != nil {
		return nil, err
	}
	if !f.HasSection(node) {
		return nil, nil
	}
	p := &Peer{Node: node}
	for _, fld := range peerFields {
		if v, ok := f.Get(node, fld.key); ok {
			*fld.get(p) = v
		}
	}
	return p, nil
}

// SavePeer writes p's stanza back to iax.conf, creating it if new.
func (s *Store) SavePeer(p *Peer) error {
	if p.Node == "" {
		return fmt.Errorf("config: node is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(IaxConfFile)
	if err != nil {
		return err
	}
	f.EnsureSection(p.Node)
	for _, fld := range peerFields {
		v := *fld.get(p)
		if v == "" {
			f.Delete(p.Node, fld.key)
			continue
		}
		f.Set(p.Node, fld.key, v)
	}
	return s.save(IaxConfFile, f)
}

// DeletePeer removes the [<node>] friend/peer stanza from iax.conf
// entirely, if present. A no-op if the node has no peer stanza.
func (s *Store) DeletePeer(node string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(IaxConfFile)
	if err != nil {
		return err
	}
	f.DeleteSection(node)
	return s.save(IaxConfFile, f)
}
