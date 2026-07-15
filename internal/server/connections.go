package server

import (
	"net/http"
	"strings"

	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/system"
)

type connectionsPageData struct {
	pageData
	Nodes         []*config.Node
	Selected      *config.Node
	FunctionsSect string
	Macros        []config.FunctionMacro
	MacroSect     string
	MacroDefs     []config.FunctionMacro
	LinkStatus    string
	LinkStatusErr string
}

// handleConnectionsIndex lists nodes and, once one is selected via
// ?node=, shows its DTMF function macros and live link status.
func (s *Server) handleConnectionsIndex(w http.ResponseWriter, r *http.Request) {
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

	data := connectionsPageData{pageData: pageData{LoggedIn: true}, Nodes: nodes}

	selectedNumber := r.URL.Query().Get("node")
	if selectedNumber == "" && len(nodes) > 0 {
		selectedNumber = nodes[0].Number
	}
	s.populateConnectionsSelection(r, &data, selectedNumber)

	s.render(w, "connections.html", data)
}

func (s *Server) populateConnectionsSelection(r *http.Request, data *connectionsPageData, number string) {
	if number == "" {
		return
	}
	node, err := s.store.LoadNode(number)
	if err != nil {
		return
	}
	data.Selected = node

	section := node.Functions
	if section == "" {
		section = "functions"
	}
	data.FunctionsSect = section

	if macros, err := s.store.ListFunctionMacros(section); err == nil {
		data.Macros = macros
	}

	macroSection := node.Macro
	if macroSection == "" {
		macroSection = "macro"
	}
	data.MacroSect = macroSection

	if defs, err := s.store.ListFunctionMacros(macroSection); err == nil {
		data.MacroDefs = defs
	}

	if out, err := system.AsteriskRX(r.Context(), s.asteriskBin, "rpt nodes "+number); err != nil {
		data.LinkStatusErr = err.Error()
	} else {
		data.LinkStatus = out
	}
}

func (s *Server) handleConnectionsMacroSave(w http.ResponseWriter, r *http.Request) {
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
	section := node.Functions
	if section == "" {
		section = "functions"
	}

	digits := strings.TrimSpace(r.FormValue("digits"))
	command := strings.TrimSpace(r.FormValue("command"))
	if digits == "" || command == "" {
		http.Redirect(w, r, "/connections?node="+number, http.StatusSeeOther)
		return
	}
	if err := s.store.SetFunctionMacro(section, digits, command); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/connections?node="+number, http.StatusSeeOther)
}

func (s *Server) handleConnectionsMacroDelete(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	digits := r.PathValue("digits")
	node, err := s.store.LoadNode(number)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	section := node.Functions
	if section == "" {
		section = "functions"
	}
	if err := s.store.DeleteFunctionMacro(section, digits); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/connections?node="+number, http.StatusSeeOther)
}

// handleConnectionsMacroDefSave and handleConnectionsMacroDefDelete edit
// the node's macro stanza (its saved multi-step DTMF sequences, invoked
// via the "macro,<n>" function) — a different rpt.conf section from the
// function/command map above, but structurally identical (digit key ->
// DTMF string), so they reuse the same Store methods against
// node.Macro instead of node.Functions.

func (s *Server) handleConnectionsMacroDefSave(w http.ResponseWriter, r *http.Request) {
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
	section := node.Macro
	if section == "" {
		section = "macro"
	}

	digits := strings.TrimSpace(r.FormValue("digits"))
	sequence := strings.TrimSpace(r.FormValue("sequence"))
	if digits == "" || sequence == "" {
		http.Redirect(w, r, "/connections?node="+number, http.StatusSeeOther)
		return
	}
	if err := s.store.SetFunctionMacro(section, digits, sequence); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/connections?node="+number, http.StatusSeeOther)
}

func (s *Server) handleConnectionsMacroDefDelete(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	digits := r.PathValue("digits")
	node, err := s.store.LoadNode(number)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	section := node.Macro
	if section == "" {
		section = "macro"
	}
	if err := s.store.DeleteFunctionMacro(section, digits); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/connections?node="+number, http.StatusSeeOther)
}

// handleConnectionsSendDTMF relays a literal DTMF command string to
// `asterisk -rx "rpt fun <node> <digits>"` — i.e. exactly what would
// happen if that sequence were dialed on the radio. The digits are
// supplied by the operator, not inferred from the function map, since
// guessing at command syntax and sending it to a live repeater without
// hardware to verify against is not a risk worth taking; the macro
// table just above this form on the page tells them what to type.
func (s *Server) handleConnectionsSendDTMF(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	digits := strings.TrimSpace(r.FormValue("digits"))

	var data connectionsPageData
	numbers, err := s.store.ListNodes()
	if err == nil {
		for _, n := range numbers {
			if node, err := s.store.LoadNode(n); err == nil {
				data.Nodes = append(data.Nodes, node)
			}
		}
	}
	data.pageData = pageData{LoggedIn: true}

	if digits == "" {
		data.pageData = flash("error", "Enter a DTMF sequence to send")
		s.populateConnectionsSelection(r, &data, number)
		s.render(w, "connections.html", data)
		return
	}

	out, err := system.AsteriskRX(r.Context(), s.asteriskBin, "rpt fun "+number+" "+digits)
	if err != nil {
		data.pageData = flash("error", err.Error())
	} else {
		msg := "Sent " + digits + " to node " + number
		if strings.TrimSpace(out) != "" {
			msg += ": " + strings.TrimSpace(out)
		}
		data.pageData = flash("ok", msg)
	}
	s.populateConnectionsSelection(r, &data, number)
	s.render(w, "connections.html", data)
}
