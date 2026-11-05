package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestUpdateBadge shows the 新 badge on 高级 and the version badge in 更新
// only while the updater reports a newer version.
func TestUpdateBadge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	probe := updateProbe
	t.Cleanup(func() { updateProbe = probe })
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 192}
	a.Profiles = loadPrefs()
	a.Settings.Thumb, a.Settings.CacheCap, a.Settings.Workers = 192, 256, 4
	a.setPage("advanced")
	for _, v := range []string{"", "9.9.9"} {
		updateProbe = func() string { return v }
		te := ui.NewTester(func(c *ui.Context) { a.settingsBody(c) }, 900, 680)
		te.Frame()
		te.Frame()
		if got := te.HasText("新") && te.HasText("v9.9.9"); got != (v != "") {
			t.Fatalf("update %q: badges shown = %v, texts %v", v, got, te.Texts())
		}
	}
}
