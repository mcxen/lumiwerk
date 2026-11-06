package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsViewPageCustomTypesAndSort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 192, Sort: "name", FolderFirst: true}
	a.Profiles = loadPrefs()
	a.Profiles.View.AllTypes = false
	a.Profiles.View.Images = true
	a.Settings.Thumb = 256
	a.Settings.Layout = 0
	a.Settings.Pane = 1 // view
	te := ui.NewTester(func(c *ui.Context) { a.settingsBody(c) }, 900, 720)
	te.Frame()
	te.Frame()
	if te.Image() == nil {
		t.Fatal("expected rendered settings view page")
	}
}
