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
