package server

import (
	"net/http"
	"slices"
	"strings"

	"hamvoipconfiggui/internal/netconfig"
	"hamvoipconfiggui/internal/system"
)

// These are standard paths on a stock Raspberry Pi OS image. They're
// not user-configurable from the UI since getting them wrong has no
// graceful failure mode (a bad dhcpcd.conf path means network edits
// silently go nowhere). The Asterisk log path is NOT included here —
// unlike these, it varies with where Asterisk itself is installed
// (e.g. HamVoIP's non-standard /usr/local/hamvoip-asterisk prefix), so
// it's a Server field set from -asterisk-log instead; see main.go.
const (
	dhcpcdConfPath      = "/etc/dhcpcd.conf"
	defaultNetInterface = "eth0"
)

type systemPageData struct {
	pageData
	Hostname        string
	Interfaces      []netconfig.Interface
	Static          *netconfig.StaticConfig
	DefaultIface    string
	AvailableIfaces []string
	LogLines        []string
	LogError        string
	NetError        string
}

func (s *Server) handleSystemPage(w http.ResponseWriter, r *http.Request) {
	s.renderSystemPage(w, r, pageData{LoggedIn: true})
}

func (s *Server) renderSystemPage(w http.ResponseWriter, r *http.Request, pd pageData) {
	ctx := r.Context()

	hostname, _ := system.Hostname(ctx)

	data := systemPageData{
		pageData:     pd,
		Hostname:     hostname,
		DefaultIface: defaultNetInterface,
	}

	if ifaces, err := netconfig.ListInterfaces(ctx); err != nil {
		data.NetError = err.Error()
	} else {
		data.Interfaces = ifaces
	}

	if static, err := netconfig.ReadManagedBlock(dhcpcdConfPath); err == nil {
		data.Static = static
	}

	if names, err := system.ListNetworkInterfaces(); err == nil {
		data.AvailableIfaces = names
	}
	// Keep the currently configured interface selectable even if it
	// isn't detected right now (e.g. this session is off-Pi, or the
	// interface is temporarily down) — otherwise saving again without
	// changing anything would silently drop it from the dropdown.
	if data.Static != nil && data.Static.Interface != "" && !slices.Contains(data.AvailableIfaces, data.Static.Interface) {
		data.AvailableIfaces = append(data.AvailableIfaces, data.Static.Interface)
	}

	if lines, err := system.LogTail(s.asteriskLog, 100); err != nil {
		data.LogError = err.Error()
	} else {
		data.LogLines = lines
	}

	s.render(w, "system.html", data)
}

func (s *Server) handleSystemHostname(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("hostname"))
	pd := pageData{LoggedIn: true}
	if err := system.SetHostname(r.Context(), name); err != nil {
		pd = flash("error", err.Error())
	} else {
		pd = flash("ok", "Hostname updated. Reboot for the change to fully take effect.")
	}
	s.renderSystemPage(w, r, pd)
}

func (s *Server) handleSystemPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	username := s.currentUsername(r)
	current := r.FormValue("current_password")
	next := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if !s.auth.Verify(username, current) {
		s.renderSystemPage(w, r, flash("error", "Current password is incorrect"))
		return
	}
	if next != confirm {
		s.renderSystemPage(w, r, flash("error", "New passwords do not match"))
		return
	}
	if err := s.auth.SetCredentials(username, next); err != nil {
		s.renderSystemPage(w, r, flash("error", err.Error()))
		return
	}
	// SetCredentials invalidates every session, including this request's;
	// send the user back through login rather than rendering a page that
	// requireAuth would immediately bounce anyway.
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleSystemNetwork(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	mode := r.FormValue("mode") // "dhcp" or "static"
	var cfg *netconfig.StaticConfig
	if mode == "static" {
		cfg = &netconfig.StaticConfig{
			Interface: strings.TrimSpace(r.FormValue("interface")),
			Address:   strings.TrimSpace(r.FormValue("address")),
			Router:    strings.TrimSpace(r.FormValue("router")),
			DNS:       strings.TrimSpace(r.FormValue("dns")),
		}
		if cfg.Interface == "" || cfg.Address == "" {
			s.renderSystemPage(w, r, flash("error", "Interface and address are required for a static configuration"))
			return
		}
	}

	if err := netconfig.WriteManagedBlock(dhcpcdConfPath, cfg); err != nil {
		s.renderSystemPage(w, r, flash("error", err.Error()))
		return
	}
	s.renderSystemPage(w, r, flash("ok", "Network configuration saved. Reboot to apply it — this does not take effect until then."))
}

func (s *Server) handleSystemRestartAsterisk(w http.ResponseWriter, r *http.Request) {
	if err := system.AsteriskRestart(r.Context(), s.asteriskBin); err != nil {
		s.renderSystemPage(w, r, flash("error", err.Error()))
		return
	}
	s.renderSystemPage(w, r, flash("ok", "Asterisk restarted"))
}

func (s *Server) handleSystemReboot(w http.ResponseWriter, r *http.Request) {
	if err := system.Reboot(r.Context()); err != nil {
		s.renderSystemPage(w, r, flash("error", err.Error()))
		return
	}
	// Best-effort: the process is very likely to be killed by the
	// shutdown before this ever reaches the client.
	s.renderSystemPage(w, r, flash("ok", "Rebooting now — this page will stop responding shortly."))
}
