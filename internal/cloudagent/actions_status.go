package cloudagent

import (
	"context"
	"encoding/json"

	"hamvoipconfiggui/internal/system"
)

// actionSystemStatus reports this device's overall Asterisk/system
// status — the read-only heartbeat action, wrapping the exact same
// system.Snapshot call internal/server's dashboard already uses (see
// internal/server/dashboard.go's gatherNodeStatuses).
func (a *Agent) actionSystemStatus(ctx context.Context, _ json.RawMessage) (any, error) {
	return system.Snapshot(ctx, a.asteriskBin), nil
}
