package server

import (
	"net/http"
	"strings"

	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/system"
)

// standardCommandSetSentinel is the "copy_from"/"from" value meaning
// "bootstrap a working command/tone set from known-good defaults"
// rather than cloning another node — see config.ApplyStandardCommandSet.
// Offered alongside real node numbers in the same picker.
const standardCommandSetSentinel = "__standard__"

// nodeFormData backs the consolidated node edit page: identity, radio
// hardware (pick an existing device or create one inline), timing,
// command/tone set, AllStarLink registration, and live connection
// status and DTMF/macro management — everything that used to be split
// across the Nodes, Radio, and Connections pages. This is edit-only;
// a brand new node is created via the separate minimal setup wizard
// (node_new.go / node_new.html), which redirects here once done.
type nodeFormData struct {
	pageData
	Node *config.Node

	Registration *config.Registration
	Peer         *config.Peer

	// Radio hardware. RadioMode picks which sub-section the template
	// shows expanded ("existing" picks from RadioDevices by name, "new"
	// shows the full inline device form). Device holds in-progress
	// "new device" field values — blank normally, non-blank only when
	// re-rendering after a failed radio_mode=new submission, so nothing
	// the operator typed is lost.
	RadioMode     string
	RadioDevices  []radioChannelOption
	Device        *config.RadioDevice
	DetectedCards []system.SoundCard
	CTCSSTones    []string

	// OtherNodes lists every other configured node number, offered
	// alongside standardCommandSetSentinel as sources for a working
	// command/tone set — see config.CloneNodeConfig / ApplyStandardCommandSet.
	OtherNodes []string

	// Live status, populated only for an existing node (see
	// populateNodeLiveStatus in node_live.go) — absorbed from what used
	// to be the standalone Connections page.
	FunctionsSect string
	Macros        []config.FunctionMacro
	MacroSect     string
	MacroDefs     []config.FunctionMacro
	LinkStatus    string
	LinkStatusErr string
}

// radioChannelOption is one entry in the RX/TX channel dropdown: a
// device already configured, shown as "usb (usbradio.conf)" but
// submitted as the literal channel string app_rpt expects.
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

// loadRadioChannelOptions lists every configured radio device, across
// both driver files, as ready-to-use rpt.conf channel strings — so the
// "use an existing device" picker offers a dropdown instead of asking
// the operator to type "USBRADIO/usb" correctly from memory.
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

// newNodeFormData assembles the parts of nodeFormData that don't depend
// on whether this is a fresh render or an error re-render: the existing-
// device picker options, a blank in-progress device (for the "create
// new" sub-form's starting state), detected sound cards, CTCSS tone
// suggestions, and the list of other nodes to copy a command set from.
func (s *Server) newNodeFormData(n *config.Node) nodeFormData {
	cards, _ := system.ListSoundCards()
	return nodeFormData{
		Node:          n,
		RadioMode:     "existing",
		RadioDevices:  s.loadRadioChannelOptions(),
		Device:        &config.RadioDevice{},
		DetectedCards: cards,
		CTCSSTones:    standardCTCSSTones,
		OtherNodes:    s.loadOtherNodeNumbers(n.Number),
	}
}

// renderNodeEditPage loads number fresh from disk and renders its full
// consolidated page. Use this when the node itself wasn't just being
// edited (macro/DTMF actions, clone-config, registration) — for a
// SaveNode failure, use renderNodeEditPageWithNode instead so the
// operator's just-typed values aren't lost.
func (s *Server) renderNodeEditPage(w http.ResponseWriter, r *http.Request, number string, pd pageData) {
	node, err := s.store.LoadNode(number)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderNodeEditPageWithNode(w, r, node, pd)
}

// renderNodeEditPageWithNode renders the consolidated node page from an
// in-memory node (e.g. one that just failed to save) rather than
// reloading from disk, so a validation error doesn't discard everything
// the operator typed.
func (s *Server) renderNodeEditPageWithNode(w http.ResponseWriter, r *http.Request, n *config.Node, pd pageData) {
	data := s.newNodeFormData(n)
	data.Registration, data.Peer = s.loadRegistrationInfo(n.Number)
	data.pageData = pd
	s.populateNodeLiveStatus(r, &data)
	s.render(w, "node_form.html", data)
}

func (s *Server) handleNodeEditForm(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderNodeEditPage(w, r, number, pageData{LoggedIn: true})
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

// applyInlineRadioDevice handles the node form's "radio_mode" field: if
// the operator chose to create a new device rather than pick an
// existing one, save it and point n.RXChannel at it. The device is
// saved BEFORE n itself is saved anywhere the caller uses this, so a
// device-save failure can never leave a node referencing something that
// doesn't exist — the exact failure mode that took a real node offline
// during this app's development. On failure, returns the in-progress
// device (so the caller can show it back to the operator) and an error
// message; ok is false if there was nothing to do (radio_mode != "new").
func (s *Server) applyInlineRadioDevice(r *http.Request, n *config.Node) (device *config.RadioDevice, errMsg string, attempted bool) {
	if r.FormValue("radio_mode") != "new" {
		return nil, "", false
	}
	file := r.FormValue("device_file")
	if !isRadioFileParam(file) {
		return &config.RadioDevice{}, "Pick usbradio.conf or simpleusb.conf for the new radio device", true
	}
	d := radioDeviceFromForm(r, "")
	if d.Name == "" {
		return d, "The new radio device needs a name", true
	}
	if err := s.store.SaveRadioDevice(file, d); err != nil {
		return d, "Couldn't save the new radio device: " + err.Error(), true
	}
	n.RXChannel = radioChannelString(file, d.Name)
	return d, "", true
}

func (s *Server) handleNodeSave(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	n := nodeFromForm(r, number)

	device, errMsg, attempted := s.applyInlineRadioDevice(r, n)
	if attempted && errMsg != "" {
		// Build the re-render by hand rather than via
		// renderNodeEditPageWithNode, so the in-progress "new device"
		// fields the operator actually typed (device) are shown back
		// instead of a blank sub-form.
		data := s.newNodeFormData(n)
		data.Registration, data.Peer = s.loadRegistrationInfo(number)
		data.pageData = flash("error", errMsg)
		data.RadioMode = "new"
		data.Device = device
		s.populateNodeLiveStatus(r, &data)
		s.render(w, "node_form.html", data)
		return
	}

	if err := s.store.SaveNode(n); err != nil {
		s.renderNodeEditPageWithNode(w, r, n, flash("error", err.Error()))
		return
	}
	// Idempotent, so this also backfills entries for a node that existed
	// before this app managed extensions.conf (or that lost them the
	// same way rpt.conf's own entries were found to disappear) — simply
	// re-saving an existing node now self-heals this.
	if err := s.store.EnsureNodeExtensions(number); err != nil {
		s.renderNodeEditPageWithNode(w, r, n, flash("error", extensionsSyncFailedMsg(err)))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeCloneConfig gives an existing node — including one created
// before this app knew how to give a new node a working command set —
// a complete functions/macro/telemetry/morse set in one click, either
// copied from another node (config.Store.CloneNodeConfig) or bootstrapped
// from known-good defaults (config.Store.ApplyStandardCommandSet) when
// standardCommandSetSentinel is chosen — the same two options offered
// at node-creation time.
func (s *Server) handleNodeCloneConfig(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	from := strings.TrimSpace(r.FormValue("from"))
	if from == "" {
		s.renderNodeEditPage(w, r, number, flash("error", "Pick a command/tone set source"))
		return
	}

	if from == standardCommandSetSentinel {
		if err := s.store.ApplyStandardCommandSet(number); err != nil {
			s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
			return
		}
	} else {
		if err := s.store.CloneNodeConfig(from, number); err != nil {
			s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
			return
		}
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

func (s *Server) handleNodeRegistrationSave(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if _, err := s.store.LoadNode(number); err != nil {
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
		s.renderNodeEditPage(w, r, number, flash("error", saveErr.Error()))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// deviceStillReferenced reports whether any node other than exclude
// still points its rxchannel/txchannel at file/name — used to guard
// handleNodeDelete's optional "also delete its radio device" so it can
// never remove a device another node needs.
func (s *Server) deviceStillReferenced(file, name, exclude string) bool {
	numbers, err := s.store.ListNodes()
	if err != nil {
		return true // fail safe: assume referenced, don't delete
	}
	for _, num := range numbers {
		if num == exclude {
			continue
		}
		node, err := s.store.LoadNode(num)
		if err != nil {
			continue
		}
		for _, ch := range []string{node.RXChannel, node.TXChannel} {
			if ref, ok := parseRadioChannel(ch); ok && ref.File == file && ref.Name == name {
				return true
			}
		}
	}
	return false
}

// companionSectionStillReferenced reports whether any node other than
// exclude has the same field (functions/macro/telemetry/morse) pointed
// at section — used to guard cleanup of a deleted node's companion
// sections, in the unusual case one was deliberately shared rather than
// being this app's own auto-generated one.
func (s *Server) companionSectionStillReferenced(get func(*config.Node) string, section, exclude string) bool {
	numbers, err := s.store.ListNodes()
	if err != nil {
		return true // fail safe: assume referenced, don't delete
	}
	for _, num := range numbers {
		if num == exclude {
			continue
		}
		node, err := s.store.LoadNode(num)
		if err != nil {
			continue
		}
		if get(node) == section {
			return true
		}
	}
	return false
}

// companionSectionSpecs lists a deleted node's functions/macro/
// telemetry/morse fields alongside the section-name prefix
// CloneNodeConfig/ApplyStandardCommandSet use when generating one for a
// node — e.g. a node numbered 52829 with Functions == "functions52829"
// matches prefix "functions". Matching the full <prefix><number>
// pattern (not just "is this field non-empty") is what makes cleanup
// safe: it only ever targets a section this app generated specifically
// for the node being deleted, never a bare "functions" default or a
// custom name someone typed in by hand.
var companionSectionSpecs = []struct {
	get    func(*config.Node) string
	prefix string
}{
	{func(n *config.Node) string { return n.Functions }, "functions"},
	{func(n *config.Node) string { return n.Macro }, "macro"},
	{func(n *config.Node) string { return n.Telemetry }, "telemetry"},
	{func(n *config.Node) string { return n.Morse }, "morse"},
}

// handleNodeDelete removes number's rpt.conf entry plus everything else
// this app knows how to attach to a node — its functions/macro/
// telemetry/morse companion sections, its extensions.conf dialplan
// entries, and its iax.conf registration/peer stanza — so deleting a
// node doesn't leave it still trying to register with AllStarLink
// (found the hard way: a node deleted before that particular cleanup
// existed kept showing up "Rejected" in iax2 show registry
// indefinitely, since nothing had ever removed its orphaned register
// => line) or leave orphaned command/tone sections behind (found via
// this feature's own end-to-end test). Each cleanup step is attempted
// even if an earlier one fails, and any failures are reported together
// rather than stopping partway through.
//
// Optionally also deletes the node's radio device, if delete_device=1
// was submitted — off by default, and skipped (with a clear reason)
// if another node still references that device.
func (s *Server) handleNodeDelete(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	node, err := s.store.LoadNode(number)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	deleteDevice := r.FormValue("delete_device") == "1"
	deviceRef, hasDevice := parseRadioChannel(node.RXChannel)

	if err := s.store.DeleteNode(number); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var failed []string
	for _, spec := range companionSectionSpecs {
		section := spec.get(node)
		if section == "" || section != spec.prefix+number {
			continue
		}
		if s.companionSectionStillReferenced(spec.get, section, number) {
			continue
		}
		if err := s.store.DeleteRptSection(section); err != nil {
			failed = append(failed, "companion section "+section+": "+err.Error())
		}
	}
	if err := s.store.RemoveNodeExtensions(number); err != nil {
		failed = append(failed, "extensions.conf dialplan entries: "+err.Error())
	}
	if err := s.store.DeleteRegistration(number); err != nil {
		failed = append(failed, "iax.conf registration: "+err.Error())
	}
	if err := s.store.DeletePeer(number); err != nil {
		failed = append(failed, "iax.conf peer stanza: "+err.Error())
	}
	if deleteDevice && hasDevice {
		if s.deviceStillReferenced(deviceRef.File, deviceRef.Name, number) {
			failed = append(failed, "radio device "+deviceRef.Name+" was NOT deleted — another node still references it")
		} else if err := s.store.DeleteRadioDevice(deviceRef.File, deviceRef.Name); err != nil {
			failed = append(failed, "radio device: "+err.Error())
		}
	}
	if len(failed) > 0 {
		s.renderHome(w, r, flash("error", "Node "+number+" deleted, but some related config wasn't fully cleaned up: "+strings.Join(failed, "; ")+" — check manually via Raw Config."))
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
