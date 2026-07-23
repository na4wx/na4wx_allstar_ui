package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hamvoipconfiggui/internal/automation"
	"hamvoipconfiggui/internal/config"
)

// scheduleSections resolves a node's scheduler/macro/functions section
// names, falling back to the bare defaults -- mirroring
// populateNodeAutomation's own resolution exactly (see this package's
// design note on never trusting a client-supplied section name).
func scheduleSections(n *config.Node) (scheduler, macro, functions string) {
	scheduler = n.Scheduler
	if scheduler == "" {
		scheduler = "schedule"
	}
	macro = n.Macro
	if macro == "" {
		macro = "macro"
	}
	functions = n.Functions
	if functions == "" {
		functions = "functions"
	}
	return scheduler, macro, functions
}

type scheduleListParams struct {
	Number string `json:"number"`
}

// automationRowResult mirrors automation.Row with proper JSON tags --
// that struct has none of its own (its only prior caller was Go-side
// template rendering), so this action wraps it rather than leaking
// capitalized Go field names into the relay's otherwise all-camelCase
// wire format (same discipline as functionMacroResult/telemetryEntryResult).
type automationRowResult struct {
	MacroNum   string `json:"macroNum"`
	Label      string `json:"label"`
	Recognized bool   `json:"recognized"`
	TimeSpec   string `json:"timeSpec"`
}

// actionScheduleList wraps populateNodeAutomation's own read logic —
// joins the node's schedule entries with their macros' DTMF values,
// resolving each into a friendly label via automation.ParseMacro.
func (a *Agent) actionScheduleList(_ context.Context, params json.RawMessage) (any, error) {
	var p scheduleListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	node, err := a.store.LoadNode(p.Number)
	if err != nil {
		return nil, err
	}
	schedulerSection, macroSection, functionsSection := scheduleSections(node)

	scheduleEntries, err := a.store.ListScheduleEntries(schedulerSection)
	if err != nil {
		return nil, err
	}
	macroEntries, err := a.store.ListFunctionMacros(macroSection)
	if err != nil {
		return nil, err
	}
	macroByNum := make(map[string]string, len(macroEntries))
	for _, m := range macroEntries {
		macroByNum[m.Digits] = m.Command
	}
	functionsEntries, err := a.store.ListFunctionMacros(functionsSection)
	if err != nil {
		functionsEntries = nil
	}

	rows := make([]automationRowResult, 0, len(scheduleEntries))
	for _, se := range scheduleEntries {
		row := automationRowResult{MacroNum: se.MacroNum, TimeSpec: se.TimeSpec}
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
	return rows, nil
}

type scheduleSaveConnectionParams struct {
	Number   string   `json:"number"`
	Action   string   `json:"action"`
	Target   string   `json:"target"`
	Minute   string   `json:"minute"`
	Hour     string   `json:"hour"`
	DOM      string   `json:"dom"`
	Month    string   `json:"month"`
	Weekdays []string `json:"weekdays"`
}

// actionScheduleSaveConnection ports handleNodeAutomationConnectionSave's
// own write sequence exactly, using the shared internal/automation
// package: ensure a functions-table digit for the action's command,
// ensure a scheduler section exists, then for each selected weekday
// allocate a macro number and write both the macro and schedule
// entries.
func (a *Agent) actionScheduleSaveConnection(_ context.Context, params json.RawMessage) (any, error) {
	var p scheduleSaveConnectionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}

	act, ok := automation.ActionByKey(p.Action)
	if !ok {
		return nil, fmt.Errorf("pick a valid connect/disconnect action")
	}
	target := strings.TrimSpace(p.Target)
	if act.NeedsTarget && target == "" {
		return nil, fmt.Errorf("enter the node number for this action")
	}

	minute, hour, dom, month := strings.TrimSpace(p.Minute), strings.TrimSpace(p.Hour), strings.TrimSpace(p.DOM), strings.TrimSpace(p.Month)
	for _, v := range []string{minute, hour, dom, month} {
		if !automation.TimeFieldRe.MatchString(v) {
			return nil, fmt.Errorf("minute/hour/day-of-month/month must each be a single number or * — app_rpt's scheduler doesn't support ranges or lists")
		}
	}
	weekdays := p.Weekdays
	for _, wd := range weekdays {
		if !automation.TimeFieldRe.MatchString(wd) {
			return nil, fmt.Errorf("invalid day-of-week value")
		}
	}
	if len(weekdays) == 0 {
		weekdays = []string{"*"}
	}

	node, err := a.store.LoadNode(p.Number)
	if err != nil {
		return nil, err
	}

	functionsSection := node.Functions
	if functionsSection == "" {
		functionsSection = "functions"
	}
	digit, err := automation.EnsureFunctionDigit(a.store, functionsSection, act.Command)
	if err != nil {
		return nil, err
	}
	dtmf := automation.BuildDTMF(digit, target, act.NeedsTarget)

	macroSection := node.Macro
	if macroSection == "" {
		macroSection = "macro"
	}
	schedulerSection := node.Scheduler
	if schedulerSection == "" {
		schedulerSection = "schedule" + p.Number
		if err := a.store.SetNodeScheduler(p.Number, schedulerSection); err != nil {
			return nil, err
		}
	}

	for _, wd := range weekdays {
		macroEntries, err := a.store.ListFunctionMacros(macroSection)
		if err != nil {
			return nil, err
		}
		macroNum := automation.AllocateMacroNumber(macroEntries)
		if err := a.store.SetFunctionMacro(macroSection, macroNum, dtmf); err != nil {
			return nil, err
		}
		timeSpec := minute + " " + hour + " " + dom + " " + month + " " + wd
		if err := a.store.SetScheduleEntry(schedulerSection, macroNum, timeSpec); err != nil {
			return nil, err
		}
	}

	return map[string]bool{"ok": true}, nil
}

type scheduleDeleteConnectionParams struct {
	Number   string `json:"number"`
	MacroNum string `json:"macroNum"`
}

// actionScheduleDeleteConnection mirrors
// handleNodeAutomationConnectionDelete: deletes the schedule entry and
// its own dedicated macro entry, leaving the shared functions-table
// digit alone.
func (a *Agent) actionScheduleDeleteConnection(_ context.Context, params json.RawMessage) (any, error) {
	var p scheduleDeleteConnectionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	node, err := a.store.LoadNode(p.Number)
	if err != nil {
		return nil, err
	}
	schedulerSection, macroSection, _ := scheduleSections(node)
	if err := a.store.DeleteScheduleEntry(schedulerSection, p.MacroNum); err != nil {
		return nil, err
	}
	if err := a.store.DeleteFunctionMacro(macroSection, p.MacroNum); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
