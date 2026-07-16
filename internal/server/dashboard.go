package server

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/system"
)

// radioChannelRef is a parsed rpt.conf rxchannel/txchannel value like
// "SimpleUSB/usb" — the driver prefix maps directly to which config
// file the device is expected to live in.
type radioChannelRef struct {
	File string
	Name string
}

// parseRadioChannel splits a channel string into the file/name a Radio
// page device is stored under, or ok=false if it's not a driver this
// app manages (e.g. a Voter or other non-USB channel type, which this
// health check has no basis to say anything about).
func parseRadioChannel(channel string) (ref radioChannelRef, ok bool) {
	driver, name, found := strings.Cut(channel, "/")
	if !found || name == "" {
		return radioChannelRef{}, false
	}
	switch driver {
	case "USBRADIO":
		return radioChannelRef{File: config.UsbradioConfFile, Name: name}, true
	case "SimpleUSB":
		return radioChannelRef{File: config.SimpleusbConfFile, Name: name}, true
	default:
		return radioChannelRef{}, false
	}
}

// nodeQuickStatus is one node's at-a-glance info on the Dashboard: who
// else is connected, app_rpt's own link-activity output, and whether
// its configured radio device actually exists. The activity output is
// shown unparsed rather than reformatted into a "currently
// transmitting: yes/no" claim — rpt lstats's exact columns vary by
// app_rpt version, and this app has no hardware to verify a parsed
// interpretation against.
type nodeQuickStatus struct {
	Node         *config.Node
	Connected    string
	ConnectedErr string
	Activity     string
	ActivityErr  string

	// MissingDevice is set when this node's rxchannel points at a
	// device that doesn't exist in usbradio.conf/simpleusb.conf — the
	// exact condition that makes chan_simpleusb/chan_usbradio refuse to
	// load and Asterisk fail to start (found the hard way: a node's
	// device stanza had vanished, apparently from something outside
	// this app regenerating the file, e.g. HamVoIP's node-config.sh).
	MissingDevice *radioChannelRef
}

type dashboardPageData struct {
	pageData
	Nodes  []*config.Node
	Status system.Status
	Quick  []nodeQuickStatus
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.renderDashboard(w, r, pageData{LoggedIn: true})
}

func (s *Server) renderDashboard(w http.ResponseWriter, r *http.Request, pd pageData) {
	numbers, err := s.store.ListNodes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var nodes []*config.Node
	for _, n := range numbers {
		node, err := s.store.LoadNode(n)
		if err != nil {
			continue // skip malformed entries rather than failing the whole page
		}
		nodes = append(nodes, node)
	}

	status := system.Snapshot(r.Context(), s.asteriskBin)

	var quick []nodeQuickStatus
	for _, node := range nodes {
		q := nodeQuickStatus{Node: node}
		if out, err := system.AsteriskRX(r.Context(), s.asteriskBin, "rpt nodes "+node.Number); err != nil {
			q.ConnectedErr = err.Error()
		} else {
			q.Connected = out
		}
		if out, err := system.AsteriskRX(r.Context(), s.asteriskBin, "rpt lstats "+node.Number); err != nil {
			q.ActivityErr = err.Error()
		} else {
			q.Activity = out
		}
		if ref, ok := parseRadioChannel(node.RXChannel); ok {
			if devices, err := s.store.ListRadioDevices(ref.File); err == nil && !slices.Contains(devices, ref.Name) {
				q.MissingDevice = &ref
			}
		}
		quick = append(quick, q)
	}

	s.render(w, "dashboard.html", dashboardPageData{
		pageData: pd,
		Nodes:    nodes,
		Status:   status,
		Quick:    quick,
	})
}

// handleDashboardLink sends a quick link ("*3<target>") or unlink
// ("*1<target>") touch-tone command from the Dashboard's quick actions —
// the same underlying mechanism as the Connections page's touch-tone
// sender (asterisk -rx "rpt fun <node> <digits>"), scoped to just these
// two standard AllStarLink codes rather than an arbitrary typed
// sequence, since this is meant to be a one-click shortcut.
func (s *Server) handleDashboardLink(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	target := strings.TrimSpace(r.FormValue("target"))
	if target == "" {
		s.renderDashboard(w, r, flash("error", "Enter a node number to link or unlink"))
		return
	}

	var digits string
	switch r.FormValue("action") {
	case "link":
		digits = "*3" + target
	case "unlink":
		digits = "*1" + target
	default:
		s.renderDashboard(w, r, flash("error", "Unknown action"))
		return
	}

	out, err := system.AsteriskRX(r.Context(), s.asteriskBin, "rpt fun "+number+" "+digits)
	if err != nil {
		s.renderDashboard(w, r, flash("error", err.Error()))
		return
	}
	msg := "Sent " + digits + " to node " + number
	if strings.TrimSpace(out) != "" {
		msg += ": " + strings.TrimSpace(out)
	}
	s.renderDashboard(w, r, flash("ok", msg))
}

// handleDashboardRecreateDevice is the Dashboard's one-click failsafe
// for the exact failure this project hit on real hardware: a node's
// configured radio device stanza went missing — apparently from
// something outside this app regenerating usbradio.conf/simpleusb.conf,
// e.g. HamVoIP's node-config.sh, which documents itself as overwriting
// rpt.conf on every run — which stops chan_simpleusb/chan_usbradio from
// loading and Asterisk from starting at all.
//
// This recreates the missing stanza with generic, safe starting-point
// values, not the operator's actual tuned audio levels — those can't be
// recovered once the section is gone, so re-tuning afterward (e.g. via
// HamVoIP's own simpleusb-tune-menu) is expected, not optional. This
// only runs when clicked; nothing here happens automatically.
func (s *Server) handleDashboardRecreateDevice(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	node, err := s.store.LoadNode(number)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ref, ok := parseRadioChannel(node.RXChannel)
	if !ok {
		s.renderDashboard(w, r, flash("error", "Node "+number+"'s radio device isn't a recognized USB driver type — nothing to recreate"))
		return
	}
	if devices, err := s.store.ListRadioDevices(ref.File); err == nil && slices.Contains(devices, ref.Name) {
		s.renderDashboard(w, r, flash("ok", "Device "+ref.Name+" already exists in "+ref.File+" — nothing to do"))
		return
	}

	d := &config.RadioDevice{
		Name:        ref.Name,
		CarrierFrom: "usb",
		TXPrelim:    "yes",
		RXMixerSet:  "500",
		TXMixerSet:  "500",
	}
	if err := s.store.SaveRadioDevice(ref.File, d); err != nil {
		s.renderDashboard(w, r, flash("error", err.Error()))
		return
	}

	// Give safe_asterisk's own retry loop a moment to notice the fix and
	// bring Asterisk up before reporting back, so the message reflects
	// reality rather than the stale pre-fix state.
	time.Sleep(4 * time.Second)
	running := system.AsteriskRunning(r.Context(), s.asteriskBin)

	msg := fmt.Sprintf("Recreated device %q in %s with generic starting-point levels (500/500) — the original tuned audio levels couldn't be recovered. Visit the Radio page to fine-tune, or re-run simpleusb-tune-menu.", ref.Name, ref.File)
	if running {
		msg += " Asterisk is now running."
	} else {
		msg += " Asterisk is still not running — check asterisk -cvvvvv for what's blocking it now."
	}
	s.renderDashboard(w, r, flash("ok", msg))
}
