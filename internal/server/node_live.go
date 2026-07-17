package server

import (
	"net/http"
	"strings"

	"hamvoipconfiggui/internal/system"
)

// populateNodeLiveStatus fills data's live-operations fields for node:
// its DTMF function map, saved macros, and a live snapshot of who's
// currently connected. This is what used to be the standalone
// Connections page — now part of the same page as the rest of a node's
// configuration, since it's scoped to exactly one node anyway and
// there's no reason to make the operator navigate elsewhere and
// re-select the node to see or act on it.
func (s *Server) populateNodeLiveStatus(r *http.Request, data *nodeFormData) {
	node := data.Node
	if node == nil || node.Number == "" {
		return
	}

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

	if out, err := system.AsteriskRX(r.Context(), s.asteriskBin, "rpt nodes "+node.Number); err != nil {
		data.LinkStatusErr = err.Error()
	} else {
		data.LinkStatus = out
	}
}

func (s *Server) handleNodeMacroSave(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
		return
	}
	if err := s.store.SetFunctionMacro(section, digits, command); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

func (s *Server) handleNodeMacroDelete(w http.ResponseWriter, r *http.Request) {
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
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeMacroDefSave and handleNodeMacroDefDelete edit the node's
// macro stanza (its saved multi-step DTMF sequences, invoked via the
// "macro,<n>" function) — a different rpt.conf section from the
// function/command map above, but structurally identical (digit key ->
// DTMF string), so they reuse the same Store methods against
// node.Macro instead of node.Functions.

func (s *Server) handleNodeMacroDefSave(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
		return
	}
	if err := s.store.SetFunctionMacro(section, digits, sequence); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

func (s *Server) handleNodeMacroDefDelete(w http.ResponseWriter, r *http.Request) {
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
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeSendDTMF relays a literal DTMF command string to
// `asterisk -rx "rpt fun <node> <digits>"` — i.e. exactly what would
// happen if that sequence were dialed on the radio. The digits are
// supplied by the operator, not inferred from the function map, since
// guessing at command syntax and sending it to a live repeater without
// hardware to verify against is not a risk worth taking; the command
// list on this same page tells them what to type.
func (s *Server) handleNodeSendDTMF(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	digits := strings.TrimSpace(r.FormValue("digits"))

	if digits == "" {
		s.renderNodeEditPage(w, r, number, flash("error", "Enter a DTMF sequence to send"))
		return
	}

	out, err := system.AsteriskRX(r.Context(), s.asteriskBin, "rpt fun "+number+" "+digits)
	if err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	msg := "Sent " + digits + " to node " + number
	if strings.TrimSpace(out) != "" {
		msg += ": " + strings.TrimSpace(out)
	}
	s.renderNodeEditPage(w, r, number, flash("ok", msg))
}
