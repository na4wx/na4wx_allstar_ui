package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"
)

// actionFunc is one relayed action's implementation: decode params,
// call exactly one specific internal/* function, and return its result
// (marshaled back to the cloud as Data) or an error.
type actionFunc func(ctx context.Context, params json.RawMessage) (any, error)

// actions returns this Agent's action registry — a fixed map literal,
// never built via reflection or any other "call this internal method by
// name" dynamic dispatch. Each entry is individually written out and
// reviewed; a relayed action name that isn't a key here can never
// reach any internal/* call. See this package's doc comment for why
// that property matters.
func (a *Agent) actions() map[string]actionFunc {
	return map[string]actionFunc{
		"system.status": a.actionSystemStatus,
	}
}

// dispatch looks up action in the registry and runs it, turning an
// unrecognized name into an error result rather than panicking or
// silently dropping the call.
func (a *Agent) dispatch(ctx context.Context, action string, params json.RawMessage) (any, error) {
	fn, ok := a.actions()[action]
	if !ok {
		return nil, fmt.Errorf("unknown action %q", action)
	}
	return fn(ctx, params)
}
