package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"

	"hamvoipconfiggui/internal/config"
)

// functionMacroKind is a closed enum -- "functions" or "macro" -- never
// a raw rpt.conf section string from the client (see this package's
// design note on that discipline, also applied by telemetrySection).
// An unrecognized kind is rejected outright rather than falling back to
// treating it as a section name.
type functionMacroKind string

const (
	functionMacroKindFunctions functionMacroKind = "functions"
	functionMacroKindMacro     functionMacroKind = "macro"
)

// functionMacroSection resolves kind to the node's own section name,
// falling back to the bare "functions"/"macro" default -- mirroring how
// the local app resolves these same two fields.
func functionMacroSection(n *config.Node, kind functionMacroKind) (string, error) {
	switch kind {
	case functionMacroKindFunctions:
		if n.Functions != "" {
			return n.Functions, nil
		}
		return "functions", nil
	case functionMacroKindMacro:
		if n.Macro != "" {
			return n.Macro, nil
		}
		return "macro", nil
	default:
		return "", fmt.Errorf("kind must be %q or %q", functionMacroKindFunctions, functionMacroKindMacro)
	}
}

type configListFunctionMacrosParams struct {
	Number string            `json:"number"`
	Kind   functionMacroKind `json:"kind"`
}

// functionMacroResult mirrors config.FunctionMacro with proper JSON
// tags -- that Go struct has none of its own (its callers so far have
// all been Go-side), so this action wraps it rather than leaking
// capitalized Go field names into the relay's otherwise all-camelCase
// wire format.
type functionMacroResult struct {
	Digits  string `json:"digits"`
	Command string `json:"command"`
}

// actionConfigListFunctionMacros wraps config.Store.ListFunctionMacros.
func (a *Agent) actionConfigListFunctionMacros(_ context.Context, params json.RawMessage) (any, error) {
	var p configListFunctionMacrosParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	node, err := a.store.LoadNode(p.Number)
	if err != nil {
		return nil, err
	}
	section, err := functionMacroSection(node, p.Kind)
	if err != nil {
		return nil, err
	}
	macros, err := a.store.ListFunctionMacros(section)
	if err != nil {
		return nil, err
	}
	out := make([]functionMacroResult, 0, len(macros))
	for _, m := range macros {
		out = append(out, functionMacroResult{Digits: m.Digits, Command: m.Command})
	}
	return out, nil
}

type configSaveFunctionMacroParams struct {
	Number  string            `json:"number"`
	Kind    functionMacroKind `json:"kind"`
	Digits  string            `json:"digits"`
	Command string            `json:"command"`
}

// actionConfigSaveFunctionMacro wraps config.Store.SetFunctionMacro.
func (a *Agent) actionConfigSaveFunctionMacro(_ context.Context, params json.RawMessage) (any, error) {
	var p configSaveFunctionMacroParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	node, err := a.store.LoadNode(p.Number)
	if err != nil {
		return nil, err
	}
	section, err := functionMacroSection(node, p.Kind)
	if err != nil {
		return nil, err
	}
	if err := a.store.SetFunctionMacro(section, p.Digits, p.Command); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type configDeleteFunctionMacroParams struct {
	Number string            `json:"number"`
	Kind   functionMacroKind `json:"kind"`
	Digits string            `json:"digits"`
}

// actionConfigDeleteFunctionMacro wraps config.Store.DeleteFunctionMacro.
func (a *Agent) actionConfigDeleteFunctionMacro(_ context.Context, params json.RawMessage) (any, error) {
	var p configDeleteFunctionMacroParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	node, err := a.store.LoadNode(p.Number)
	if err != nil {
		return nil, err
	}
	section, err := functionMacroSection(node, p.Kind)
	if err != nil {
		return nil, err
	}
	if err := a.store.DeleteFunctionMacro(section, p.Digits); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
