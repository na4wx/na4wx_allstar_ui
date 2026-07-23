package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"

	"hamvoipconfiggui/internal/config"
)

// actionConfigListNodes wraps config.Store.ListNodes — every node
// number currently configured on this device.
func (a *Agent) actionConfigListNodes(_ context.Context, _ json.RawMessage) (any, error) {
	return a.store.ListNodes()
}

type configLoadNodeParams struct {
	Number string `json:"number"`
}

// actionConfigLoadNode wraps config.Store.LoadNode.
func (a *Agent) actionConfigLoadNode(_ context.Context, params json.RawMessage) (any, error) {
	var p configLoadNodeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	return a.store.LoadNode(p.Number)
}

// actionConfigSaveNode wraps config.Store.SaveNode, params decoded
// directly as a config.Node (its JSON tags exist for exactly this).
// Matches internal/server's own handleNodeSave: also calls
// EnsureNodeExtensions afterward so a node created or edited remotely
// gets the same extensions.conf dialplan entries a local save gives it
// — skipping that step here would leave a cloud-created node unable to
// dial in.
//
// This deliberately does not replicate handleNodeCreate's full "new
// node" setup wizard (radio device provisioning, command-set cloning,
// IAX2 peer defaults, SHARI audio preset) — that guided, derived-defaults
// flow is a separate, larger feature than plain field-level node CRUD,
// and is not in scope for this phase. A node created this way needs
// those set up separately, the same as any node whose section was
// created by hand.
func (a *Agent) actionConfigSaveNode(_ context.Context, params json.RawMessage) (any, error) {
	var n config.Node
	if err := json.Unmarshal(params, &n); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.store.SaveNode(&n); err != nil {
		return nil, err
	}
	if err := a.store.EnsureNodeExtensions(n.Number); err != nil {
		return nil, fmt.Errorf("save succeeded but dialplan sync failed: %w", err)
	}
	return a.store.LoadNode(n.Number)
}

type configDeleteNodeParams struct {
	Number string `json:"number"`
}

// actionConfigDeleteNode wraps config.Store.DeleteNode.
func (a *Agent) actionConfigDeleteNode(_ context.Context, params json.RawMessage) (any, error) {
	var p configDeleteNodeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.store.DeleteNode(p.Number); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type configSetCourtesyTonesParams struct {
	Number      string `json:"number"`
	UnlinkedCT  string `json:"unlinkedCT"`
	RemoteCT    string `json:"remoteCT"`
	LinkUnkeyCT string `json:"linkUnkeyCT"`
}

// actionConfigSetCourtesyTones wraps config.Store.SetCourtesyToneAssignments
// — the narrow write path for unlinkedct/remotect/linkunkeyct that
// SaveNode's own field allowlist deliberately excludes (see that
// method's own doc comment).
func (a *Agent) actionConfigSetCourtesyTones(_ context.Context, params json.RawMessage) (any, error) {
	var p configSetCourtesyTonesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.store.SetCourtesyToneAssignments(p.Number, p.UnlinkedCT, p.RemoteCT, p.LinkUnkeyCT); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

// telemetrySection resolves a node's telemetry stanza name the same way
// the local app's own populateNodeTelemetry/handleNodeTelemetrySave do:
// the node's own Telemetry field if set, otherwise the bare "telemetry"
// default — never a client-supplied section string (see this package's
// actions_config.go doc note on that discipline).
func telemetrySection(n *config.Node) string {
	if n.Telemetry != "" {
		return n.Telemetry
	}
	return "telemetry"
}

type toneSpecResult struct {
	Freq1      int `json:"freq1"`
	Freq2      int `json:"freq2"`
	DurationMS int `json:"durationMs"`
	Amplitude  int `json:"amplitude"`
}

type telemetryEntryResult struct {
	Key   string          `json:"key"`
	Value string          `json:"value"`
	Tone  *toneSpecResult `json:"tone,omitempty"`
}

type configListTelemetryParams struct {
	Number string `json:"number"`
}

// actionConfigListTelemetry wraps config.Store.ListTelemetryEntries,
// resolving the section from the node itself rather than trusting a
// client-supplied section name. Each entry includes both its raw value
// and, when it parses as a single tone-generator segment, the friendly
// per-field breakdown — mirroring ParseSingleTone's own either/or usage
// in the local app's UI so the client can offer the same friendly
// editor, falling back to raw text otherwise.
func (a *Agent) actionConfigListTelemetry(_ context.Context, params json.RawMessage) (any, error) {
	var p configListTelemetryParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	node, err := a.store.LoadNode(p.Number)
	if err != nil {
		return nil, err
	}
	entries, err := a.store.ListTelemetryEntries(telemetrySection(node))
	if err != nil {
		return nil, err
	}
	out := make([]telemetryEntryResult, 0, len(entries))
	for _, e := range entries {
		r := telemetryEntryResult{Key: e.Key, Value: e.Value}
		if tone, ok := config.ParseSingleTone(e.Value); ok {
			r.Tone = &toneSpecResult{Freq1: tone.Freq1, Freq2: tone.Freq2, DurationMS: tone.DurationMS, Amplitude: tone.Amplitude}
		}
		out = append(out, r)
	}
	return out, nil
}

type configSetTelemetryParams struct {
	Number string `json:"number"`
	Key    string `json:"key"`
	Value  string `json:"value"`
}

// actionConfigSetTelemetry wraps config.Store.SetTelemetryEntry, section
// resolved the same way actionConfigListTelemetry does.
func (a *Agent) actionConfigSetTelemetry(_ context.Context, params json.RawMessage) (any, error) {
	var p configSetTelemetryParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	node, err := a.store.LoadNode(p.Number)
	if err != nil {
		return nil, err
	}
	if err := a.store.SetTelemetryEntry(telemetrySection(node), p.Key, p.Value); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type configCloneNodeConfigParams struct {
	SrcNumber string `json:"srcNumber"`
	DstNumber string `json:"dstNumber"`
}

// actionConfigCloneNodeConfig wraps config.Store.CloneNodeConfig — gives
// dstNumber a complete functions/macro/telemetry/morse set copied from
// srcNumber, in one call.
func (a *Agent) actionConfigCloneNodeConfig(_ context.Context, params json.RawMessage) (any, error) {
	var p configCloneNodeConfigParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.store.CloneNodeConfig(p.SrcNumber, p.DstNumber); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type configApplyStandardCommandSetParams struct {
	Number string `json:"number"`
}

// actionConfigApplyStandardCommandSet wraps
// config.Store.ApplyStandardCommandSet — bootstraps a working
// functions/macro/telemetry/morse set from known-good defaults, the
// same "standard" option offered alongside cloning another node.
func (a *Agent) actionConfigApplyStandardCommandSet(_ context.Context, params json.RawMessage) (any, error) {
	var p configApplyStandardCommandSetParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.store.ApplyStandardCommandSet(p.Number); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type configNormalizeNodeConfigParams struct {
	Number string `json:"number"`
}

// actionConfigNormalizeNodeConfig wraps config.Store.NormalizeNodeConfig
// — repairs a node whose command/tone sections are named for a
// different node (the classic case: created by renaming the shipped
// template's [1998] header). Returns the list of fields that were
// changed, for a confirmation message; an empty list means nothing
// needed repair.
func (a *Agent) actionConfigNormalizeNodeConfig(_ context.Context, params json.RawMessage) (any, error) {
	var p configNormalizeNodeConfigParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	changed, err := a.store.NormalizeNodeConfig(p.Number)
	if err != nil {
		return nil, err
	}
	if changed == nil {
		changed = []string{} // NormalizeNodeConfig returns nil when nothing needed repair -- never marshal that as JSON null
	}
	return map[string]any{"changed": changed}, nil
}
