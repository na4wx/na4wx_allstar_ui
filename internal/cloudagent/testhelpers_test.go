package cloudagent

import (
	"path/filepath"
	"testing"

	"hamvoipconfiggui/internal/config"
	"hamvoipconfiggui/internal/sounds"
	"hamvoipconfiggui/internal/soundschedule"
	"hamvoipconfiggui/internal/wxtone"
)

// newTestAgent builds an Agent for tests that don't care about the
// sounds/soundschedule/wxtone/skywarnplus/sa818 dependencies -- each
// gets a working, empty store backed by its own temp dir/file, since a
// nil *Store would panic the moment a test path touched it. Tests that
// do care about one of these (e.g. actions_sounds_test.go) build their
// own Agent directly via New() instead.
func newTestAgent(t *testing.T, settingsPath string, store *config.Store, asteriskBin string) *Agent {
	t.Helper()
	return New(
		settingsPath,
		store,
		asteriskBin,
		sounds.New(t.TempDir(), t.TempDir(), "sox"),
		soundschedule.New(filepath.Join(t.TempDir(), "sound-schedule.json")),
		wxtone.New(filepath.Join(t.TempDir(), "wx-tones.json")),
		"", // skywarnDir -- not installed in these tests
		"818-prog",
		filepath.Join(t.TempDir(), "sa818-last.json"),
	)
}
