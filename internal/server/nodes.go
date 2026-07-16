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

	// OtherNodes lists every other configured node number, offered as
	// sources to copy a working functions/macro/telemetry/morse set
	// from — see CloneNodeConfig. Empty if this is the only node (or
	// the first one being created), since there'd be nothing to copy.
	OtherNodes []string
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

// loadOtherNodeNumbers lists every configured node except exclude, for
// the "copy command/tone set from" picker. Best-effort: an error just
// means an empty list (the picker simply won't offer anything), not a
// page failure.
func (s *Server) loadOtherNodeNumbers(exclude string) []string {
	numbers, err := s.store.ListNodes()
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range numbers {
		if n != exclude {
			out = append(out, n)
		}
	}
	return out
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
		OtherNodes:   s.loadOtherNodeNumbers(""),
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
		OtherNodes:   s.loadOtherNodeNumbers(number),
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
			OtherNodes:   s.loadOtherNodeNumbers(number),
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
	copyFrom := strings.TrimSpace(r.FormValue("copy_from"))
	if err := s.store.SaveNode(n); err != nil {
		s.render(w, "node_form.html", nodeFormData{
			pageData:     flash("error", err.Error()),
			Node:         n,
			IsNew:        true,
			RadioDevices: s.loadRadioChannelOptions(),
			OtherNodes:   s.loadOtherNodeNumbers(""),
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
	// Best-effort: the node itself is already fully saved at this point,
	// so a clone failure shouldn't block getting to the (now real) node
	// — just surface it clearly rather than silently leaving the new
	// node with no working command set.
	if copyFrom != "" {
		if err := s.store.CloneNodeConfig(copyFrom, n.Number); err != nil {
			reg, peer := s.loadRegistrationInfo(n.Number)
			s.render(w, "node_form.html", nodeFormData{
				pageData:     flash("error", "Node created, but copying the command/tone set from "+copyFrom+" failed: "+err.Error()+" — use the same option on this node's edit page to retry."),
				Node:         n,
				Registration: reg,
				Peer:         peer,
				RadioDevices: s.loadRadioChannelOptions(),
				OtherNodes:   s.loadOtherNodeNumbers(n.Number),
			})
			return
		}
	}
	http.Redirect(w, r, "/nodes/"+n.Number, http.StatusSeeOther)
}

// handleNodeCloneConfig is the "repair" counterpart to handleNodeCreate's
// copy_from option: lets an existing node — including one created
// before this app knew how to give a new node a working command set —
// get one in a single click, by copying another node's functions/macro/
// telemetry/morse sections. See config.Store.CloneNodeConfig.
func (s *Server) handleNodeCloneConfig(w http.ResponseWriter, r *http.Request) {
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
	from := strings.TrimSpace(r.FormValue("from"))
	if from == "" {
		s.render(w, "node_form.html", nodeFormData{
			pageData:     flash("error", "Pick a node to copy from"),
			Node:         node,
			RadioDevices: s.loadRadioChannelOptions(),
			OtherNodes:   s.loadOtherNodeNumbers(number),
		})
		return
	}

	if err := s.store.CloneNodeConfig(from, number); err != nil {
		reg, peer := s.loadRegistrationInfo(number)
		s.render(w, "node_form.html", nodeFormData{
			pageData:     flash("error", err.Error()),
			Node:         node,
			Registration: reg,
			Peer:         peer,
			RadioDevices: s.loadRadioChannelOptions(),
			OtherNodes:   s.loadOtherNodeNumbers(number),
		})
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
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
			OtherNodes:   s.loadOtherNodeNumbers(number),
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
			OtherNodes:   s.loadOtherNodeNumbers(number),
		})
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeDelete removes number's rpt.conf entry plus everything else
// this app knows how to attach to a node — its extensions.conf dialplan
// entries and its iax.conf registration/peer stanza — so deleting a
// node doesn't leave it still trying to register with AllStarLink
// (found the hard way: a node deleted before this cleanup existed kept
// showing up "Rejected" in iax2 show registry indefinitely, since
// nothing had ever removed its orphaned register => line). Each cleanup
// step is attempted even if an earlier one fails, and any failures are
// reported together rather than stopping partway through.
func (s *Server) handleNodeDelete(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if err := s.store.DeleteNode(number); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var failed []string
	if err := s.store.RemoveNodeExtensions(number); err != nil {
		failed = append(failed, "extensions.conf dialplan entries: "+err.Error())
	}
	if err := s.store.DeleteRegistration(number); err != nil {
		failed = append(failed, "iax.conf registration: "+err.Error())
	}
	if err := s.store.DeletePeer(number); err != nil {
		failed = append(failed, "iax.conf peer stanza: "+err.Error())
	}
	if len(failed) > 0 {
		s.renderNodesIndex(w, r, flash("error", "Node "+number+" deleted, but some related config wasn't fully cleaned up: "+strings.Join(failed, "; ")+" — check manually via Raw Config."))
		return
	}
	http.Redirect(w, r, "/nodes", http.StatusSeeOther)
}
