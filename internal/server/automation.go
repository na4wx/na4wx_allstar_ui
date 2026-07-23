package server

import (
	"net/http"
	"strings"

	"hamvoipconfiggui/internal/automation"
)

// populateNodeAutomation fills data's "Scheduled connections" rows.
// Best-effort, like the rest of this page's supplementary data — a read
// failure just leaves the section looking empty rather than failing the
// whole page.
func (s *Server) populateNodeAutomation(data *nodeFormData) {
	node := data.Node
	if node == nil || node.Number == "" {
		return
	}
	schedulerSection := node.Scheduler
	if schedulerSection == "" {
		schedulerSection = "schedule"
	}
	data.SchedulerSect = schedulerSection

	scheduleEntries, err := s.store.ListScheduleEntries(schedulerSection)
	if err != nil {
		return
	}
	macroSection := node.Macro
	if macroSection == "" {
		macroSection = "macro"
	}
	macroEntries, err := s.store.ListFunctionMacros(macroSection)
	if err != nil {
		return
	}
	macroByNum := make(map[string]string, len(macroEntries))
	for _, m := range macroEntries {
		macroByNum[m.Digits] = m.Command
	}
	functionsSection := node.Functions
	if functionsSection == "" {
		functionsSection = "functions"
	}
	functionsEntries, err := s.store.ListFunctionMacros(functionsSection)
	if err != nil {
		functionsEntries = nil
	}

	rows := make([]automation.Row, 0, len(scheduleEntries))
	for _, se := range scheduleEntries {
		row := automation.Row{MacroNum: se.MacroNum, TimeSpec: se.TimeSpec}
		if dtmf, ok := macroByNum[se.MacroNum]; ok {
			if label, recognized := automation.ParseMacro(dtmf, functionsEntries); recognized {
				row.Label = label
				row.Recognized = true
			} else {
				row.Label = dtmf
			}
		} else {
			row.Label = "(macro " + se.MacroNum + " not found)"
		}
		rows = append(rows, row)
	}
	data.AutomationConnections = rows
}

// handleNodeAutomationConnectionSave adds one connect/disconnect
// automation rule. Weekday checkboxes are a create-time convenience:
// app_rpt's schedule format allows only one day-of-week value (or "*")
// per entry, so selecting several fans out into that many independent
// schedule+macro pairs, each then listed/edited/deleted as its own row —
// there is no native way to group them, so this doesn't pretend to.
func (s *Server) handleNodeAutomationConnectionSave(w http.ResponseWriter, r *http.Request) {
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

	action, ok := automation.ActionByKey(r.FormValue("action"))
	if !ok {
		s.renderNodeEditPage(w, r, number, flash("error", "Pick a valid connect/disconnect action"))
		return
	}
	target := strings.TrimSpace(r.FormValue("target"))
	if action.NeedsTarget && target == "" {
		s.renderNodeEditPage(w, r, number, flash("error", "Enter the node number for this action"))
		return
	}

	minute := strings.TrimSpace(r.FormValue("minute"))
	hour := strings.TrimSpace(r.FormValue("hour"))
	dom := strings.TrimSpace(r.FormValue("dom"))
	month := strings.TrimSpace(r.FormValue("month"))
	for _, v := range []string{minute, hour, dom, month} {
		if !automation.TimeFieldRe.MatchString(v) {
			s.renderNodeEditPage(w, r, number, flash("error", "Minute/hour/day-of-month/month must each be a single number or * — app_rpt's scheduler doesn't support ranges or lists"))
			return
		}
	}
	weekdays := r.Form["weekday"]
	for _, wd := range weekdays {
		if !automation.TimeFieldRe.MatchString(wd) {
			s.renderNodeEditPage(w, r, number, flash("error", "Invalid day-of-week value"))
			return
		}
	}
	if len(weekdays) == 0 {
		weekdays = []string{"*"}
	}

	functionsSection := node.Functions
	if functionsSection == "" {
		functionsSection = "functions"
	}
	digit, err := automation.EnsureFunctionDigit(s.store, functionsSection, action.Command)
	if err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	dtmf := automation.BuildDTMF(digit, target, action.NeedsTarget)

	macroSection := node.Macro
	if macroSection == "" {
		macroSection = "macro"
	}
	schedulerSection := node.Scheduler
	if schedulerSection == "" {
		schedulerSection = "schedule" + number
		if err := s.store.SetNodeScheduler(number, schedulerSection); err != nil {
			s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
			return
		}
	}

	for _, wd := range weekdays {
		macroEntries, err := s.store.ListFunctionMacros(macroSection)
		if err != nil {
			s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
			return
		}
		macroNum := automation.AllocateMacroNumber(macroEntries)
		if err := s.store.SetFunctionMacro(macroSection, macroNum, dtmf); err != nil {
			s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
			return
		}
		timeSpec := minute + " " + hour + " " + dom + " " + month + " " + wd
		if err := s.store.SetScheduleEntry(schedulerSection, macroNum, timeSpec); err != nil {
			s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
			return
		}
	}

	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}

// handleNodeAutomationConnectionDelete removes one connect/disconnect
// automation rule's schedule entry and its own dedicated macro entry, but
// leaves the shared functions-table digit alone — other rows may reuse
// it, and it's indistinguishable from one the operator wired up by hand.
func (s *Server) handleNodeAutomationConnectionDelete(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	macroNum := r.PathValue("macronum")
	node, err := s.store.LoadNode(number)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	schedulerSection := node.Scheduler
	if schedulerSection == "" {
		schedulerSection = "schedule"
	}
	macroSection := node.Macro
	if macroSection == "" {
		macroSection = "macro"
	}
	if err := s.store.DeleteScheduleEntry(schedulerSection, macroNum); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	if err := s.store.DeleteFunctionMacro(macroSection, macroNum); err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	http.Redirect(w, r, "/nodes/"+number, http.StatusSeeOther)
}
