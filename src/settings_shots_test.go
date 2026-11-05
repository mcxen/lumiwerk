package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSettingsShots writes each settings page to LW_SETTINGS_SHOTS when set.
func TestSettingsShots(t *testing.T) {
	dir := os.Getenv("LW_SETTINGS_SHOTS")
	if dir == "" {
		t.Skip("LW_SETTINGS_SHOTS not set")
	}
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 192}
	a.Profiles = loadPrefs()
	a.Settings.Thumb, a.Settings.CacheCap, a.Settings.Workers = 192, 256, 4
	for i, p := range settingsPages {
		a.Settings.Pane = i
		te := ui.NewTester(func(c *ui.Context) { a.settingsBody(c) }, 900, 680)
		te.Frame()
		te.Frame()
		f, err := os.Create(filepath.Join(dir, p.id+".png"))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, te.Image())
		f.Close()
	}
}
