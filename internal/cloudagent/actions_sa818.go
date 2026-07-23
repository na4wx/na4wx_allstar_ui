package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hamvoipconfiggui/internal/sa818"
)

// sa818ProgramResult is what actionSA818Program returns -- the tool's
// own success flag and raw output are both surfaced, mirroring
// internal/server/sa818.go's handleSystemSA818Apply (a non-nil err
// means the tool itself couldn't run; ok=false with err=nil means it
// ran but the module rejected the settings — two different failure
// modes the caller needs to tell apart).
type sa818ProgramResult struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
}

// actionSA818Program wraps sa818.Program, params decoded directly as
// sa818.Settings, and records the attempt via sa818.SaveLast the same
// way the local System page's own handler does — so "last sent"
// displays agree regardless of which UI triggered the send.
func (a *Agent) actionSA818Program(ctx context.Context, params json.RawMessage) (any, error) {
	var settings sa818.Settings
	if err := json.Unmarshal(params, &settings); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}

	output, ok, err := sa818.Program(ctx, a.sa818Tool, settings)

	if a.sa818StatePath != "" {
		last := &sa818.LastApplied{
			Settings:  settings,
			Tool:      a.sa818Tool,
			AppliedAt: time.Now(),
			Success:   ok,
			Output:    output,
		}
		_ = sa818.SaveLast(a.sa818StatePath, last)
	}

	if err != nil {
		return nil, fmt.Errorf("could not run %s: %w", a.sa818Tool, err)
	}
	return sa818ProgramResult{OK: ok, Output: output}, nil
}
