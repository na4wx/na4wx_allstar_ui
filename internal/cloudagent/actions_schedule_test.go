package cloudagent

import (
	"context"
	"encoding/json"
	"testing"
)

func TestActionScheduleSaveConnectionSingleWeekday(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]any{
		"number": "2000", "action": "connect_stay", "target": "2001",
		"minute": "0", "hour": "8", "dom": "*", "month": "*", "weekdays": []string{"1"},
	})
	if _, err := a.dispatch(context.Background(), "schedule.saveConnection", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}

	entries, err := a.store.ListScheduleEntries("schedule2000")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].TimeSpec != "0 8 * * 1" {
		t.Fatalf("schedule entries = %+v, want one entry \"0 8 * * 1\"", entries)
	}

	macros, err := a.store.ListFunctionMacros("macro")
	if err != nil {
		t.Fatal(err)
	}
	if len(macros) != 1 || macros[0].Command != "*9002001" {
		t.Fatalf("macro entries = %+v, want one \"*9002001\" entry (allocated digit 900 + target 2001)", macros)
	}

	functions, err := a.store.ListFunctionMacros("functions")
	if err != nil {
		t.Fatal(err)
	}
	if len(functions) != 1 || functions[0].Digits != "900" || functions[0].Command != "ilink,3" {
		t.Fatalf("functions entries = %+v, want one allocated 900=ilink,3 entry", functions)
	}
}

func TestActionScheduleSaveConnectionMultipleWeekdaysShareOneDigit(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]any{
		"number": "2000", "action": "connect_stay", "target": "2001",
		"minute": "0", "hour": "8", "dom": "*", "month": "*", "weekdays": []string{"1", "3", "5"},
	})
	if _, err := a.dispatch(context.Background(), "schedule.saveConnection", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}

	scheduleEntries, err := a.store.ListScheduleEntries("schedule2000")
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduleEntries) != 3 {
		t.Fatalf("schedule entries = %+v, want 3 (one per weekday)", scheduleEntries)
	}

	macroEntries, err := a.store.ListFunctionMacros("macro")
	if err != nil {
		t.Fatal(err)
	}
	if len(macroEntries) != 3 {
		t.Fatalf("macro entries = %+v, want 3 distinct macro numbers", macroEntries)
	}

	functionsEntries, err := a.store.ListFunctionMacros("functions")
	if err != nil {
		t.Fatal(err)
	}
	if len(functionsEntries) != 1 {
		t.Fatalf("functions entries = %+v, want exactly one shared digit for all 3 weekdays", functionsEntries)
	}
}

func TestActionScheduleSaveConnectionReusesExistingDigit(t *testing.T) {
	a := newConfigTestAgent(t)
	// Pre-provision the node as if ApplyStandardCommandSet already ran.
	if err := a.store.SetFunctionMacro("functions", "3", "ilink,3"); err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]any{
		"number": "2000", "action": "connect_stay", "target": "2001",
		"minute": "0", "hour": "8", "dom": "*", "month": "*", "weekdays": []string{"1"},
	})
	if _, err := a.dispatch(context.Background(), "schedule.saveConnection", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}

	functionsEntries, err := a.store.ListFunctionMacros("functions")
	if err != nil {
		t.Fatal(err)
	}
	if len(functionsEntries) != 1 || functionsEntries[0].Digits != "3" {
		t.Fatalf("functions entries = %+v, want the pre-existing digit 3 reused, not a newly allocated one", functionsEntries)
	}

	macroEntries, err := a.store.ListFunctionMacros("macro")
	if err != nil {
		t.Fatal(err)
	}
	if len(macroEntries) != 1 || macroEntries[0].Command != "*32001" {
		t.Fatalf("macro entries = %+v, want the DTMF built from the reused digit 3", macroEntries)
	}
}

func TestActionScheduleSaveConnectionRejectsInvalidAction(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]any{"number": "2000", "action": "not_a_real_action", "minute": "0", "hour": "8", "dom": "*", "month": "*"})
	if _, err := a.dispatch(context.Background(), "schedule.saveConnection", params); err == nil {
		t.Fatal("dispatch error = nil, want rejection of an unrecognized action")
	}
}

func TestActionScheduleSaveConnectionRejectsInvalidTimeField(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]any{
		"number": "2000", "action": "disconnect_all",
		"minute": "not-a-number", "hour": "8", "dom": "*", "month": "*",
	})
	if _, err := a.dispatch(context.Background(), "schedule.saveConnection", params); err == nil {
		t.Fatal("dispatch error = nil, want rejection of a non-numeric, non-* time field")
	}
}

func TestActionScheduleSaveConnectionRequiresTargetWhenNeeded(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]any{
		"number": "2000", "action": "connect_stay", "target": "",
		"minute": "0", "hour": "8", "dom": "*", "month": "*",
	})
	if _, err := a.dispatch(context.Background(), "schedule.saveConnection", params); err == nil {
		t.Fatal("dispatch error = nil, want rejection of a blank target for an action that needs one")
	}
}

func TestActionScheduleListResolvesLabels(t *testing.T) {
	a := newConfigTestAgent(t)
	saveParams, _ := json.Marshal(map[string]any{
		"number": "2000", "action": "connect_stay", "target": "2001",
		"minute": "0", "hour": "8", "dom": "*", "month": "*", "weekdays": []string{"1"},
	})
	if _, err := a.dispatch(context.Background(), "schedule.saveConnection", saveParams); err != nil {
		t.Fatalf("save: %v", err)
	}

	listParams, _ := json.Marshal(map[string]string{"number": "2000"})
	result, err := a.dispatch(context.Background(), "schedule.list", listParams)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	rows, ok := result.([]automationRowResult)
	if !ok {
		t.Fatalf("result type = %T, want []automationRowResult", result)
	}
	if len(rows) != 1 || !rows[0].Recognized || rows[0].Label != "Connect (stay connected) 2001" {
		t.Fatalf("rows = %+v, want one recognized \"Connect (stay connected) 2001\" row", rows)
	}
}

func TestActionScheduleDeleteConnectionLeavesSharedDigit(t *testing.T) {
	a := newConfigTestAgent(t)
	saveParams, _ := json.Marshal(map[string]any{
		"number": "2000", "action": "connect_stay", "target": "2001",
		"minute": "0", "hour": "8", "dom": "*", "month": "*", "weekdays": []string{"1", "2"},
	})
	if _, err := a.dispatch(context.Background(), "schedule.saveConnection", saveParams); err != nil {
		t.Fatalf("save: %v", err)
	}

	scheduleEntries, err := a.store.ListScheduleEntries("schedule2000")
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduleEntries) != 2 {
		t.Fatalf("schedule entries = %+v, want 2", scheduleEntries)
	}

	deleteParams, _ := json.Marshal(map[string]string{"number": "2000", "macroNum": scheduleEntries[0].MacroNum})
	if _, err := a.dispatch(context.Background(), "schedule.deleteConnection", deleteParams); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}

	remaining, err := a.store.ListScheduleEntries("schedule2000")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining schedule entries = %+v, want 1 after deleting the other", remaining)
	}

	functionsEntries, err := a.store.ListFunctionMacros("functions")
	if err != nil {
		t.Fatal(err)
	}
	if len(functionsEntries) != 1 {
		t.Fatalf("functions entries = %+v, want the shared digit left untouched by the delete", functionsEntries)
	}
}
