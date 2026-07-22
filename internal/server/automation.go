package server

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"hamvoipconfiggui/internal/config"
)

// automationAction is one of the four connect/disconnect actions the
// "Automation" tab can schedule — deliberately scoped to exactly the
// ilink commands ApplyStandardCommandSet already provisions (see
// standard_commands.go: digit "1"->ilink,1, "2"->ilink,2, "3"->ilink,3,
// "76"->ilink,6 — note digit does NOT equal the ilink number in every
// case), matching this app's own existing "Connect (stay
// connected)/(listen only)/Disconnect" quick actions
// (web/static/js/app.js's data-fill-digits, dashboard.go's
// handleNodeLink) rather than inventing new ilink modes.
type automationAction struct {
	Key         string // form value / stored marker
	Command     string // app_rpt command this action's functions-table digit must map to
	NeedsTarget bool   // whether a target node number is dialed after the digit
	Label       string // friendly name for the UI
}

var automationActions = []automationAction{
	{Key: "connect_stay", Command: "ilink,3", NeedsTarget: true, Label: "Connect (stay connected)"},
	{Key: "connect_listen", Command: "ilink,2", NeedsTarget: true, Label: "Connect (listen only)"},
	{Key: "disconnect_one", Command: "ilink,1", NeedsTarget: true, Label: "Disconnect a specific node"},
	{Key: "disconnect_all", Command: "ilink,6", NeedsTarget: false, Label: "Disconnect all"},
}

func automationActionByKey(key string) (automationAction, bool) {
	for _, a := range automationActions {
		if a.Key == key {
			return a, true
		}
	}
	return automationAction{}, false
}

func automationActionByCommand(command string) (automationAction, bool) {
	for _, a := range automationActions {
		if a.Command == command {
			return a, true
		}
	}
	return automationAction{}, false
}

// ensureFunctionDigit finds the digit in section whose Command already
// equals command, creating one only if genuinely missing. In the common
// case (ApplyStandardCommandSet has already run) this is a read-only
// lookup — automation entries reuse whatever digit the node's own
// functions table already assigns a command to, rather than assuming a
// fixed digit.
func ensureFunctionDigit(store *config.Store, section, command string) (string, error) {
	entries, err := store.ListFunctionMacros(section)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Command == command {
			return e.Digits, nil
		}
	}
	digit := allocateDigit(entries)
	if err := store.SetFunctionMacro(section, digit, command); err != nil {
		return "", err
	}
	return digit, nil
}

// allocateDigit picks an unused 3-digit function key starting at 900, so
// an automation-created entry can never collide with a digit an operator
// picked by hand (this app's Command list lets them type anything).
func allocateDigit(entries []config.FunctionMacro) string {
	used := make(map[string]bool, len(entries))
	for _, e := range entries {
		used[e.Digits] = true
	}
	for n := 900; ; n++ {
		d := strconv.Itoa(n)
		if !used[d] {
			return d
		}
	}
}

// allocateMacroNumber picks the smallest unused macro number >= 1 — never
// 0, which app_rpt reserves as "startupmacro". Scans every entry in the
// node's macro section, hand-authored ones included: the scheduler
// necessarily shares that section/namespace with the existing "Saved
// macros" table, since app_rpt's scheduler references "the node's own
// macro stanza" directly and there's no way to redirect it elsewhere.
func allocateMacroNumber(entries []config.FunctionMacro) string {
	used := make(map[int]bool, len(entries))
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Digits); err == nil {
			used[n] = true
		}
	}
	for n := 1; ; n++ {
		if !used[n] {
			return strconv.Itoa(n)
		}
	}
}

// buildAutomationDTMF builds the DTMF sequence a macro entry "dials" to
// invoke digit's command — a trailing target node number for actions
// that take one, nothing for disconnect-all.
func buildAutomationDTMF(digit, target string, needsTarget bool) string {
	if needsTarget {
		return "*" + digit + target
	}
	return "*" + digit
}

// parseAutomationMacro reverse-parses a macro's raw DTMF value (e.g.
// "*32000") into a friendly automation description, resolving purely
// through the node's real functions-table entries — never assuming a
// digit equals a particular ilink number. Candidate digits are matched
// longest-first, so a real multi-digit prefix (e.g. "76") is never
// mistaken for a shorter one ("7") followed by leftover target digits.
// Anything that doesn't resolve to one of automationActions' known
// commands renders as unrecognized rather than erroring — it may be a
// macro the operator wrote by hand for something else entirely.
func parseAutomationMacro(dtmf string, functionsEntries []config.FunctionMacro) (label string, recognized bool) {
	dtmf = strings.TrimSpace(dtmf)
	if !strings.HasPrefix(dtmf, "*") {
		return "", false
	}
	rest := dtmf[1:]

	candidates := make([]config.FunctionMacro, len(functionsEntries))
	copy(candidates, functionsEntries)
	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i].Digits) > len(candidates[j].Digits)
	})

	for _, fe := range candidates {
		if fe.Digits == "" || !strings.HasPrefix(rest, fe.Digits) {
			continue
		}
		action, ok := automationActionByCommand(fe.Command)
		if !ok {
			continue
		}
		target := rest[len(fe.Digits):]
		switch {
		case action.NeedsTarget && target != "":
			return action.Label + " " + target, true
		case !action.NeedsTarget && target == "":
			return action.Label, true
		}
		// Digit matched but target-presence doesn't fit this action's
		// shape (e.g. disconnect-all with leftover digits) — keep
		// looking rather than returning a misleading label.
	}
	return "", false
}

// automationRow is one entry in the "Scheduled connections" table —
// joining a schedule entry with its macro's DTMF value by macro number.
type automationRow struct {
	MacroNum   string
	Label      string // friendly description, or the raw DTMF if unrecognized
	Recognized bool
	TimeSpec   string
}

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

	rows := make([]automationRow, 0, len(scheduleEntries))
	for _, se := range scheduleEntries {
		row := automationRow{MacroNum: se.MacroNum, TimeSpec: se.TimeSpec}
		if dtmf, ok := macroByNum[se.MacroNum]; ok {
			if label, recognized := parseAutomationMacro(dtmf, functionsEntries); recognized {
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

// timeFieldRe matches app_rpt's own schedule-field syntax: a single
// non-negative integer, or "*" — never a range, list, or step value (see
// config.ScheduleEntry's doc comment). Rejecting anything else here
// means a malformed submission fails loudly instead of silently never
// scheduling.
var timeFieldRe = regexp.MustCompile(`^\*$|^[0-9]+$`)

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

	action, ok := automationActionByKey(r.FormValue("action"))
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
		if !timeFieldRe.MatchString(v) {
			s.renderNodeEditPage(w, r, number, flash("error", "Minute/hour/day-of-month/month must each be a single number or * — app_rpt's scheduler doesn't support ranges or lists"))
			return
		}
	}
	weekdays := r.Form["weekday"]
	for _, wd := range weekdays {
		if !timeFieldRe.MatchString(wd) {
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
	digit, err := ensureFunctionDigit(s.store, functionsSection, action.Command)
	if err != nil {
		s.renderNodeEditPage(w, r, number, flash("error", err.Error()))
		return
	}
	dtmf := buildAutomationDTMF(digit, target, action.NeedsTarget)

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
		macroNum := allocateMacroNumber(macroEntries)
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
