package cloudagent

import (
	"context"
	"encoding/json"
	"testing"

	"hamvoipconfiggui/internal/wxtone"
)

func TestActionWXToneSaveListDelete(t *testing.T) {
	a := newTestAgent(t, t.TempDir()+"/settings.json", nil, "asterisk")

	entry := wxtone.Entry{
		Node: "2000", CTKey: "ct1",
		NormalType: wxtone.TypeSound, NormalSound: "normal-tone",
		WXType: wxtone.TypeSound, WXSound: "wx-tone",
	}
	params, _ := json.Marshal(entry)
	if _, err := a.dispatch(context.Background(), "wxTone.save", params); err != nil {
		t.Fatalf("save error = %v", err)
	}

	listParams, _ := json.Marshal(map[string]string{"node": "2000"})
	result, err := a.dispatch(context.Background(), "wxTone.list", listParams)
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	entries, ok := result.([]wxtone.Entry)
	if !ok {
		t.Fatalf("result type = %T, want []wxtone.Entry", result)
	}
	if len(entries) != 1 || entries[0].CTKey != "ct1" {
		t.Fatalf("entries = %+v", entries)
	}

	deleteParams, _ := json.Marshal(map[string]string{"id": entries[0].ID})
	if _, err := a.dispatch(context.Background(), "wxTone.delete", deleteParams); err != nil {
		t.Fatalf("delete error = %v", err)
	}
	result, err = a.dispatch(context.Background(), "wxTone.list", listParams)
	if err != nil {
		t.Fatalf("list after delete error = %v", err)
	}
	if entries := result.([]wxtone.Entry); len(entries) != 0 {
		t.Fatalf("entries after delete = %+v, want empty", entries)
	}
}
