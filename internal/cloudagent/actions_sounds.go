package cloudagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// actionSoundsListAll wraps sounds.Store.ListAll -- custom files first,
// then the read-only stock library, matching every other sound picker
// in this app.
func (a *Agent) actionSoundsListAll(_ context.Context, _ json.RawMessage) (any, error) {
	return a.sounds.ListAll()
}

type soundsUploadParams struct {
	Name       string `json:"name"`
	DataBase64 string `json:"dataBase64"` // the uploaded file's raw bytes, base64-encoded -- see protocol.go's doc comment on why this stays simple JSON rather than a separate binary framing
}

// actionSoundsUpload wraps sounds.Store.Upload. The relayed file is
// small (a station-ID clip, a courtesy tone) and arrives as one JSON
// message rather than a stream, so it's decoded fully into memory
// before handing it to Upload — consistent with how small this app
// expects any single sound file to be.
func (a *Agent) actionSoundsUpload(ctx context.Context, params json.RawMessage) (any, error) {
	var p soundsUploadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(p.DataBase64)
	if err != nil {
		return nil, fmt.Errorf("bad dataBase64: %w", err)
	}
	output, err := a.sounds.Upload(ctx, p.Name, strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, output)
	}
	return map[string]bool{"ok": true}, nil
}

type soundsDeleteParams struct {
	Name string `json:"name"`
}

// actionSoundsDelete wraps sounds.Store.DeleteCustom.
func (a *Agent) actionSoundsDelete(_ context.Context, params json.RawMessage) (any, error) {
	var p soundsDeleteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("bad params: %w", err)
	}
	if err := a.sounds.DeleteCustom(p.Name); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
