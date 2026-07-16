package server

import (
	"net/http"
	"strings"

	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/system"
)

// nodeQuickStatus is one node's at-a-glance info on the Dashboard: who
// else is connected, and app_rpt's own link-activity output. The
// activity output is shown unparsed rather than reformatted into a
// "currently transmitting: yes/no" claim — rpt lstats's exact columns
// vary by app_rpt version, and this app has no hardware to verify a
// parsed interpretation against.
type nodeQuickStatus struct {
	Node         *config.Node
	Connected    string
	ConnectedErr string
	Activity     string
	ActivityErr  string
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
