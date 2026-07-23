package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"

	"hamvoipconfiggui/internal/soundschedule"
)

type soundScheduleListParams struct {
	Node string `json:"node"`
}

// actionSoundScheduleList wraps soundschedule.Store.ListForNode.
func (a *Agent) actionSoundScheduleList(_ context.Context, params json.RawMessage) (any, error) {
	var p soundScheduleListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	return a.soundSchedule.ListForNode(p.Node)
}

// actionSoundScheduleSave wraps soundschedule.Store.Save, params decoded
// directly as a soundschedule.Entry.
func (a *Agent) actionSoundScheduleSave(_ context.Context, params json.RawMessage) (any, error) {
	var e soundschedule.Entry
	if err := json.Unmarshal(params, &e); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.soundSchedule.Save(e); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type soundScheduleDeleteParams struct {
	ID string `json:"id"`
}

// actionSoundScheduleDelete wraps soundschedule.Store.Delete.
func (a *Agent) actionSoundScheduleDelete(_ context.Context, params json.RawMessage) (any, error) {
	var p soundScheduleDeleteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.soundSchedule.Delete(p.ID); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
