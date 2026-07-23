package cloudagent

import (
	"context"
	"encoding/json"
	"testing"

	"hamvoipconfiggui/internal/config"
)

func TestActionConfigListFunctionMacrosDefaultsSection(t *testing.T) {
	a := newConfigTestAgent(t)
	// 2000's fixture has no Functions field set -- should fall back to
	// the bare "functions" section, matching the local app's own
	// resolution.
	if err := a.store.SetFunctionMacro("functions", "1", "ilink,3,2000"); err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]string{"number": "2000", "kind": "functions"})
	result, err := a.dispatch(context.Background(), "config.listFunctionMacros", params)
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	macros, ok := result.([]functionMacroResult)
	if !ok {
		t.Fatalf("result type = %T, want []functionMacroResult", result)
	}
	if len(macros) != 1 || macros[0].Digits != "1" || macros[0].Command != "ilink,3,2000" {
		t.Fatalf("macros = %+v", macros)
	}
}

func TestActionConfigListFunctionMacrosRejectsUnknownKind(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "2000", "kind": "telemetry"})
	if _, err := a.dispatch(context.Background(), "config.listFunctionMacros", params); err == nil {
		t.Fatal("dispatch error = nil, want rejection of an unrecognized kind rather than treating it as a section name")
	}
}

func TestActionConfigSaveFunctionMacro(t *testing.T) {
	a := newConfigTestAgent(t)
	params, _ := json.Marshal(map[string]string{"number": "2000", "kind": "macro", "digits": "1", "command": "ilink,3,2000"})
	if _, err := a.dispatch(context.Background(), "config.saveFunctionMacro", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	macros, err := a.store.ListFunctionMacros("macro")
	if err != nil {
		t.Fatal(err)
	}
	if len(macros) != 1 || macros[0].Digits != "1" || macros[0].Command != "ilink,3,2000" {
		t.Fatalf("macros = %+v, want it saved to the bare \"macro\" section", macros)
	}
}

func TestActionConfigSaveFunctionMacroUsesNodeSection(t *testing.T) {
	a := newConfigTestAgent(t)
	newNode := config.Node{Number: "3000", RXChannel: "SimpleUSB/usb2", Duplex: "1", Functions: "functions3000"}
	saveParams, _ := json.Marshal(newNode)
	if _, err := a.dispatch(context.Background(), "config.saveNode", saveParams); err != nil {
		t.Fatalf("save node: %v", err)
	}

	params, _ := json.Marshal(map[string]string{"number": "3000", "kind": "functions", "digits": "2", "command": "ilink,1"})
	if _, err := a.dispatch(context.Background(), "config.saveFunctionMacro", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	macros, err := a.store.ListFunctionMacros("functions3000")
	if err != nil {
		t.Fatal(err)
	}
	if len(macros) != 1 || macros[0].Digits != "2" {
		t.Fatalf("macros = %+v, want it saved to the node's own functions3000 section", macros)
	}
}

func TestActionConfigDeleteFunctionMacro(t *testing.T) {
	a := newConfigTestAgent(t)
	if err := a.store.SetFunctionMacro("functions", "1", "ilink,3,2000"); err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]string{"number": "2000", "kind": "functions", "digits": "1"})
	if _, err := a.dispatch(context.Background(), "config.deleteFunctionMacro", params); err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	macros, err := a.store.ListFunctionMacros("functions")
	if err != nil {
		t.Fatal(err)
	}
	if len(macros) != 0 {
		t.Fatalf("macros = %+v, want it deleted", macros)
	}
}
