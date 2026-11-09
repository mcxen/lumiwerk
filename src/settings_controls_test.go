package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSettingsSelectPopupRowsAreCompact opens the shared dropdown and checks
// each option is one compact row: the earlier build stacked the check mark
// over the name and spread the rows about 100pt apart.
func TestSettingsSelectPopupRowsAreCompact(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	value := "auto"
	options := []string{"auto", "github", "mirror"}
	names := map[string]string{"auto": "自动", "github": "GitHub", "mirror": "tvv.tw 镜像"}
	te := ui.NewTester(func(c *ui.Context) {
		pal := settingsPaletteFor(c)
		ui.Column(c).Padding(20).Children(func() {
			settingsPopup(c, "sel", &value, options, func(v string) string { return names[v] }, setCtrlW, pal)
		})
	}, 400, 300)
	te.Frame()
	trig, ok := te.Find("自动")
	if !ok {
		t.Fatalf("trigger missing: %v", te.Texts())
	}
	te.ClickAt(trig.X+trig.W/2, trig.Y+trig.H/2)
	te.Frame()
	te.Frame()
	a, okA := te.Find("GitHub")
	b, okB := te.Find("tvv.tw 镜像")
	if !okA || !okB {
		t.Fatalf("options missing: %v", te.Texts())
	}
	if dir := os.Getenv("LW_CARD_SHOTS"); dir != "" {
		f, _ := os.Create(filepath.Join(dir, "popup.png"))
		_ = png.Encode(f, te.Image())
		f.Close()
	}
	t.Logf("auto=%v github=%v mirror=%v", trig, a, b)
	if gap := b.Y - a.Y; gap < settingsMenuItemH-2 || gap > settingsMenuItemH+6 {
		t.Fatalf("option rows are %.0fpt apart, want about %d", gap, settingsMenuItemH)
	}
	te.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	te.Frame()
	if value != "mirror" {
		t.Fatalf("clicking an option kept %q", value)
	}
}
