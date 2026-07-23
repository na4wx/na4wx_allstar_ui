package cloudagent

import (
	"context"
	"encoding/json"
	"testing"

	"hamvoipconfiggui/internal/soundschedule"
)

func TestActionSoundScheduleSaveListDelete(t *testing.T) {
	a := newTestAgent(t, t.TempDir()+"/settings.json", nil, "asterisk")

	entry := soundschedule.Entry{Node: "2000", File: "test-clip", Minute: "0", Hour: "*", DayOfMonth: "*", Month: "*"}
	params, _ := json.Marshal(entry)
	if _, err := a.dispatch(context.Background(), "soundSchedule.save", params); err != nil {
		t.Fatalf("save error = %v", err)
	}

	listParams, _ := json.Marshal(map[string]string{"node": "2000"})
	result, err := a.dispatch(context.Background(), "soundSchedule.list", listParams)
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	entries, ok := result.([]soundschedule.Entry)
	if !ok {
		t.Fatalf("result type = %T, want []soundschedule.Entry", result)
	}
	if len(entries) != 1 || entries[0].File != "test-clip" {
		t.Fatalf("entries = %+v", entries)
	}

	deleteParams, _ := json.Marshal(map[string]string{"id": entries[0].ID})
	if _, err := a.dispatch(context.Background(), "soundSchedule.delete", deleteParams); err != nil {
		t.Fatalf("delete error = %v", err)
	}
	result, err = a.dispatch(context.Background(), "soundSchedule.list", listParams)
	if err != nil {
		t.Fatalf("list after delete error = %v", err)
	}
	if entries := result.([]soundschedule.Entry); len(entries) != 0 {
		t.Fatalf("entries after delete = %+v, want empty", entries)
	}
}
