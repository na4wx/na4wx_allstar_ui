package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"

	"hamvoipconfiggui/internal/wxtone"
)

type wxToneListParams struct {
	Node string `json:"node"`
}

// actionWXToneList wraps wxtone.Store.ListForNode.
func (a *Agent) actionWXToneList(_ context.Context, params json.RawMessage) (any, error) {
	var p wxToneListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	return a.wxTones.ListForNode(p.Node)
}

// actionWXToneSave wraps wxtone.Store.Save, params decoded directly as a
// wxtone.Entry. Note this only persists the mapping (mirroring
// internal/server/wxtone.go's own handleNodeWXToneSave) -- it does not
// call applyWXTone immediately the way the local handler does, since
// that method lives on *server.Server and reaches into config.Store's
// telemetry-section rewriting/system.AsteriskReloadRpt in a way this
// package doesn't yet mirror. A newly relayed mapping takes effect on
// this device's own next poll tick (see internal/server's
// StartWXTonePoller equivalent, which still runs locally regardless of
// whether the mapping was created locally or via the cloud relay).
func (a *Agent) actionWXToneSave(_ context.Context, params json.RawMessage) (any, error) {
	var e wxtone.Entry
	if err := json.Unmarshal(params, &e); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.wxTones.Save(e); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type wxToneDeleteParams struct {
	ID string `json:"id"`
}

// actionWXToneDelete wraps wxtone.Store.Delete.
func (a *Agent) actionWXToneDelete(_ context.Context, params json.RawMessage) (any, error) {
	var p wxToneDeleteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.wxTones.Delete(p.ID); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
