package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"

	"hamvoipconfiggui/internal/config"
)

// requireRawConfigEdit gates every rawconfig.* action (reads included —
// see the Go app's plan doc's Security section for why this is treated
// as this app's single most powerful, least-guarded capability, and
// gets its own explicit opt-in on top of Cloud Sync itself).
func (a *Agent) requireRawConfigEdit() error {
	settings, err := a.settings.Load()
	if err != nil {
		return err
	}
	if !settings.AllowRawConfigEdit {
		return errCapabilityDisabled("Remote raw config editing")
	}
	return nil
}

func requireAllowedRawConfigFile(name string) error {
	if !config.IsAllowedRawConfigFile(name) {
		return fmt.Errorf("%q is not one of this app's editable config files", name)
	}
	return nil
}

// rawConfigKV is one section's key/value line, JSON-friendly.
type rawConfigKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type rawConfigSection struct {
	Name string        `json:"name"`
	Keys []rawConfigKV `json:"keys"`
}

type rawConfigFileResult struct {
	Sections []rawConfigSection `json:"sections"`
}

// actionRawConfigListFiles wraps config.AllowedRawConfigFiles -- the
// list itself isn't sensitive (it's just file names), so this one is
// not gated behind AllowRawConfigEdit; the cloud UI needs it to know
// what to even offer before the operator has necessarily turned the
// capability on.
func (a *Agent) actionRawConfigListFiles(_ context.Context, _ json.RawMessage) (any, error) {
	return config.AllowedRawConfigFiles, nil
}

type rawConfigFileParams struct {
	File string `json:"file"`
}

// actionRawConfigGetFile wraps config.Store.RawFile, returning every
// section's key/value pairs in file order -- the same shape
// internal/server's own /config page builds for its template.
func (a *Agent) actionRawConfigGetFile(_ context.Context, params json.RawMessage) (any, error) {
	if err := a.requireRawConfigEdit(); err != nil {
		return nil, err
	}
	var p rawConfigFileParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := requireAllowedRawConfigFile(p.File); err != nil {
		return nil, err
	}
	f, err := a.store.RawFile(p.File)
	if err != nil {
		return nil, err
	}
	result := rawConfigFileResult{}
	for _, sec := range f.Sections() {
		var keys []rawConfigKV
		for _, kv := range f.SectionKeys(sec) {
			keys = append(keys, rawConfigKV{Key: kv.Key, Value: kv.Value})
		}
		result.Sections = append(result.Sections, rawConfigSection{Name: sec, Keys: keys})
	}
	return result, nil
}

type rawConfigSetKeyParams struct {
	File    string `json:"file"`
	Section string `json:"section"`
	Index   int    `json:"index"`
	Value   string `json:"value"`
}

// actionRawConfigSetKey wraps asteriskconf.File.SetNthKeyInSection +
// config.Store.SaveRaw -- addresses one key/value line by its position
// within the section (see SetNthKeyInSection's own doc comment for why
// position rather than key name), matching internal/server's own raw
// config editor.
func (a *Agent) actionRawConfigSetKey(_ context.Context, params json.RawMessage) (any, error) {
	if err := a.requireRawConfigEdit(); err != nil {
		return nil, err
	}
	var p rawConfigSetKeyParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := requireAllowedRawConfigFile(p.File); err != nil {
		return nil, err
	}
	f, err := a.store.RawFile(p.File)
	if err != nil {
		return nil, err
	}
	if ok := f.SetNthKeyInSection(p.Section, p.Index, p.Value); !ok {
		return nil, fmt.Errorf("no key at position %d in section %q", p.Index, p.Section)
	}
	if err := a.store.SaveRaw(p.File, f); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type rawConfigAddKeyParams struct {
	File    string `json:"file"`
	Section string `json:"section"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

// actionRawConfigAddKey wraps asteriskconf.File.Set (adds if absent) +
// config.Store.SaveRaw.
func (a *Agent) actionRawConfigAddKey(_ context.Context, params json.RawMessage) (any, error) {
	if err := a.requireRawConfigEdit(); err != nil {
		return nil, err
	}
	var p rawConfigAddKeyParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := requireAllowedRawConfigFile(p.File); err != nil {
		return nil, err
	}
	f, err := a.store.RawFile(p.File)
	if err != nil {
		return nil, err
	}
	f.Set(p.Section, p.Key, p.Value)
	if err := a.store.SaveRaw(p.File, f); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

type rawConfigAddSectionParams struct {
	File    string `json:"file"`
	Section string `json:"section"`
}

// actionRawConfigAddSection wraps asteriskconf.File.EnsureSection +
// config.Store.SaveRaw.
func (a *Agent) actionRawConfigAddSection(_ context.Context, params json.RawMessage) (any, error) {
	if err := a.requireRawConfigEdit(); err != nil {
		return nil, err
	}
	var p rawConfigAddSectionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := requireAllowedRawConfigFile(p.File); err != nil {
		return nil, err
	}
	f, err := a.store.RawFile(p.File)
	if err != nil {
		return nil, err
	}
	f.EnsureSection(p.Section)
	if err := a.store.SaveRaw(p.File, f); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
