package server

import (
	"net/http"
	"strconv"
	"strings"

	"hamvoipconfiggui/internal/config"
)

type nodeFormData struct {
	pageData
	Node         *config.Node
	IsNew        bool
	Registration *config.Registration
	Peer         *config.Peer
	RadioDevices []radioChannelOption
}

// radioChannelOption is one entry in the RX/TX channel dropdown: a
// device already set up on the Radio page, shown as "usb (usbradio.conf)"
// but submitted as the literal channel string app_rpt expects.
type radioChannelOption struct {
	Channel string // e.g. "USBRADIO/usb"
	Label   string // e.g. "usb (usbradio.conf)"
}

// loadRegistrationInfo best-effort loads the IAX2 registration and peer
// stanza for a node, for display alongside its rpt.conf identity. Errors
// are swallowed (fields stay nil) since this is supplementary info on a
// page whose primary job is editing rpt.conf.
func (s *Server) loadRegistrationInfo(number string) (*config.Registration, *config.Peer) {
	if number == "" {
		return nil, nil
	}
	reg, _ := s.store.LoadRegistration(number)
	peer, _ := s.store.LoadPeer(number)
	return reg, peer
}

// loadRadioChannelOptions lists every device configured on the Radio
// page, across both driver files, as ready-to-use rpt.conf channel
// strings — so the node form can offer a dropdown instead of asking the
// operator to type "USBRADIO/usb" correctly from memory.
func (s *Server) loadRadioChannelOptions() []radioChannelOption {
	var opts []radioChannelOption
	if devices, err := s.store.ListRadioDevices(config.UsbradioConfFile); err == nil {
		for _, d := range devices {
			opts = append(opts, radioChannelOption{
				Channel: "USBRADIO/" + d,
				Label:   d + " (usbradio.conf)",
			})
		}
	}
	if devices, err := s.store.ListRadioDevices(config.SimpleusbConfFile); err == nil {
		for _, d := range devices {
			opts = append(opts, radioChannelOption{
				Channel: "SimpleUSB/" + d,
				Label:   d + " (simpleusb.conf)",
			})
		}
	}
	return opts
}

type nodesIndexData struct {
	pageData
	Nodes []*config.Node
}

func (s *Server) handleNodesIndex(w http.ResponseWriter, r *http.Request) {
	s.renderNodesIndex(w, r, pageData{LoggedIn: true})
}

func (s *Server) renderNodesIndex(w http.ResponseWriter, r *http.Request, pd pageData) {
	numbers, err := s.store.ListNodes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var nodes []*config.Node
	for _, n := range numbers {
		if node, err := s.store.LoadNode(n); err == nil {
			nodes = append(nodes, node)
		}
	}
	s.render(w, "nodes_index.html", nodesIndexData{
		pageData: pd,
		Nodes:    nodes,
	})
}

// handleNodesSyncExtensions is the bulk counterpart to the per-node
// EnsureNodeExtensions call in handleNodeCreate/handleNodeSave: it
// backfills every configured node's extensions.conf dialplan entries in
// one pass, for nodes that already existed (or lost their entries the
// same way rpt.conf's own did) before this app started managing that
// file, without requiring an individual visit-and-resave per node.
// EnsureNodeExtensions only ever adds missing entries, never touches
// existing ones, so this is safe to run repeatedly.
func (s *Server) handleNodesSyncExtensions(w http.ResponseWriter, r *http.Request) {
	numbers, err := s.store.ListNodes()
	if err != nil {
		s.renderNodesIndex(w, r, flash("error", err.Error()))
		return
	}

	var failed []string
	for _, number := range numbers {
		if err := s.store.EnsureNodeExtensions(number); err != nil {
			failed = append(failed, number+": "+err.Error())
		}
	}

	ok := len(numbers) - len(failed)
	msg := "Synced dialplan entries for " + strconv.Itoa(ok) + " of " + strconv.Itoa(len(numbers)) + " node(s)."
	if len(failed) > 0 {
		s.renderNodesIndex(w, r, flash("error", msg+" Failed: "+strings.Join(failed, "; ")))
		return
	}
	s.renderNodesIndex(w, r, flash("ok", msg))
}

func (s *Server) handleNodeNewForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, "node_form.html", nodeFormData{
		pageData:     pageData{LoggedIn: true},
		Node:         &config.Node{},
		IsNew:        true,
		RadioDevices: s.loadRadioChannelOptions(),
	})
}

func (s *Server) handleNodeEditForm(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	node, err := s.store.LoadNode(number)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	reg, peer := s.loadRegistrationInfo(number)
	s.render(w, "node_form.html", nodeFormData{
		pageData:     pageData{LoggedIn: true},
		Node:         node,
		Registration: reg,
		Peer:         peer,
		RadioDevices: s.loadRadioChannelOptions(),
	})
}

func (s *Server) handleNodeRegistrationSave(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	node, err := s.store.LoadNode(number)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	reg := config.Registration{
		Node:     number,
		Password: r.FormValue("reg_password"),
		Host:     r.FormValue("reg_host"),
		Port:     r.FormValue("reg_port"),
	}
	peer := &config.Peer{
		Node:    number,
		Type:    r.FormValue("peer_type"),
		Context: r.FormValue("peer_context"),
		Host:    r.FormValue("peer_host"),
		Secret:  r.FormValue("peer_secret"),
		Auth:    r.FormValue("peer_auth"),
	}

	var saveErr error
	if reg.Password != "" || reg.Host != "" {
		saveErr = s.store.SaveRegistration(reg)
	}
	if saveErr == nil && (peer.Type != "" || peer.Context != "" || peer.Secret != "") {
		saveErr = s.store.SavePeer(peer)
	}

	if saveErr != nil {
		regPtr, peerPtr := s.loadRegistrationInfo(number)
		s.render(w, "node_form.html", nodeFormData{
			pageData:     flash("error", saveErr.Error()),
			Node:         node,
			Registration: regPtr,
			Peer:         peerPtr,
			RadioDevices: s.loadRadioChannelOptions(),
		})
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// nodeFromForm builds a config.Node from posted form values. number, if
// non-empty, overrides the form's "number" field (used when editing an
// existing node, whose number field is read-only in the UI).
func nodeFromForm(r *http.Request, number string) *config.Node {
	n := &config.Node{
		Number:      number,
		DialString:  r.FormValue("dial_string"),
		RXChannel:   r.FormValue("rxchannel"),
		TXChannel:   r.FormValue("txchannel"),
		Duplex:      r.FormValue("duplex"),
		Telemetry:   r.FormValue("telemetry"),
		Morse:       r.FormValue("morse"),
		Functions:   r.FormValue("functions"),
		Macro:       r.FormValue("macro"),
		HangTime:    r.FormValue("hangtime"),
		AltHangTime: r.FormValue("althangtime"),
		TOTime:      r.FormValue("totime"),
		IDTime:      r.FormValue("idtime"),
		IDRecording: r.FormValue("idrecording"),
	}
	if number == "" {
		n.Number = r.FormValue("number")
	}
	return n
}

// extensionsSyncFailedMsg formats the flash shown when a node's rpt.conf
// save succeeded but syncing its extensions.conf dialplan entries
// afterward failed — the node itself is fine, this just means the
// [radio-secure]/[radio-secure-proxy]/[radio-iaxrpt] entries it needs to
// actually be reachable weren't added and need manual attention.
func extensionsSyncFailedMsg(err error) string {
	return "Node saved, but updating extensions.conf's dialplan entries failed: " + err.Error() + " — add them manually via Raw Config (see EnsureNodeExtensions's contexts: radio-secure, radio-secure-proxy, radio-iaxrpt)."
}

func (s *Server) handleNodeCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	n := nodeFromForm(r, "")
	if err := s.store.SaveNode(n); err != nil {
		s.render(w, "node_form.html", nodeFormData{
			pageData:     flash("error", err.Error()),
			Node:         n,
			IsNew:        true,
			RadioDevices: s.loadRadioChannelOptions(),
		})
		return
	}
	if err := s.store.EnsureNodeExtensions(n.Number); err != nil {
		s.render(w, "node_form.html", nodeFormData{
			pageData:     flash("error", extensionsSyncFailedMsg(err)),
			Node:         n,
			RadioDevices: s.loadRadioChannelOptions(),
		})
		return
	}
	http.Redirect(w, r, "/nodes/"+n.Number, http.StatusSeeOther)
}

func (s *Server) handleNodeSave(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	n := nodeFromForm(r, number)
	if err := s.store.SaveNode(n); err != nil {
		reg, peer := s.loadRegistrationInfo(number)
		s.render(w, "node_form.html", nodeFormData{
			pageData:     flash("error", err.Error()),
			Node:         n,
			Registration: reg,
			Peer:         peer,
			RadioDevices: s.loadRadioChannelOptions(),
		})
		return
	}
	// Idempotent, so this also backfills entries for a node that existed
	// before this app managed extensions.conf (or that lost them the
	// same way rpt.conf's own entries were found to disappear) — simply
	// re-saving an existing node now self-heals this.
	if err := s.store.EnsureNodeExtensions(number); err != nil {
		reg, peer := s.loadRegistrationInfo(number)
		s.render(w, "node_form.html", nodeFormData{
			pageData:     flash("error", extensionsSyncFailedMsg(err)),
			Node:         n,
			Registration: reg,
			Peer:         peer,
			RadioDevices: s.loadRadioChannelOptions(),
		})
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

func (s *Server) handleNodeDelete(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if err := s.store.DeleteNode(number); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.store.RemoveNodeExtensions(number); err != nil {
		s.renderNodesIndex(w, r, flash("error", "Node "+number+" deleted, but removing its extensions.conf dialplan entries failed: "+err.Error()+" — remove them manually via Raw Config."))
		return
	}
	http.Redirect(w, r, "/nodes", http.StatusSeeOther)
}
