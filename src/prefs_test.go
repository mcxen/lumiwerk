package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestLoadPrefsFillsProfilesWithoutFile checks that a first launch (no prefs
// file yet) still ends up with the nine configuration slots, so the next save
// cannot write "profiles": null and drop a folder or a slot.
func TestLoadPrefsFillsProfilesWithoutFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	p := loadPrefs()
	if len(p.Profiles) != 9 {
		t.Fatalf("profiles = %d, want 9", len(p.Profiles))
	}
	if p.View.Theme == "" || p.QuickRename == "" {
		t.Fatalf("defaults missing: theme=%q quickRename=%q", p.View.Theme, p.QuickRename)
	}
}

// TestSavePrefsNeverWritesNullProfiles guards the round trip: saving defaults
// must write nine slots, and reading that file back must keep them.
func TestSavePrefsNeverWritesNullProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	savePrefs(prefsFile{})

	data, err := os.ReadFile(prefsPath())
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Profiles []profile `json:"profiles"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Profiles) != 9 {
		t.Fatalf("saved profiles = %d, want 9 (file: %s)", len(raw.Profiles), data)
	}

	p := loadPrefs()
	if len(p.Profiles) != 9 {
		t.Fatalf("reloaded profiles = %d, want 9", len(p.Profiles))
	}
}

// TestSavePrefsKeepsPreviousGeneration checks that a second save keeps the
// first state in prefs.json.bak, so an accidental overwrite can be undone.
func TestSavePrefsKeepsPreviousGeneration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	savePrefs(prefsFile{QuickRename: "first"})
	savePrefs(prefsFile{QuickRename: "second"})

	raw, err := os.ReadFile(prefsPath() + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	var bak prefsFile
	if err := json.Unmarshal(raw, &bak); err != nil {
		t.Fatal(err)
	}
	if bak.QuickRename != "first" {
		t.Fatalf("backup quickRename = %q, want first", bak.QuickRename)
	}

	current, err := os.ReadFile(prefsPath())
	if err != nil {
		t.Fatal(err)
	}
	var now prefsFile
	if err := json.Unmarshal(current, &now); err != nil {
		t.Fatal(err)
	}
	if now.QuickRename != "second" {
		t.Fatalf("current quickRename = %q, want second", now.QuickRename)
	}
}

// TestLoadPrefsKeepsConfiguredProfile checks that normalization keeps a root
// the user configured in the startup slot.
func TestLoadPrefsKeepsConfiguredProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	savePrefs(prefsFile{Profiles: []profile{{Root: "/tmp/example"}}})

	p := loadPrefs()
	if len(p.Profiles) != 9 {
		t.Fatalf("profiles = %d, want 9", len(p.Profiles))
	}
	if p.Profiles[0].Root != "/tmp/example" {
		t.Fatalf("root = %q, want /tmp/example", p.Profiles[0].Root)
	}
}

// TestCloseActionDefaultsToTray checks that a preferences file written
// before this setting still hides the window on close, and that an
// explicit quit opt-out survives a save.
func TestCloseActionDefaultsToTray(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	p := loadPrefs()
	if p.CloseAction != closeToTray || !p.hidesOnClose() {
		t.Fatalf("default close = %q hides=%v", p.CloseAction, p.hidesOnClose())
	}

	savePrefs(prefsFile{CloseAction: closeQuit})
	quit := loadPrefs()
	if quit.CloseAction != closeQuit || quit.hidesOnClose() {
		t.Fatalf("reloaded close = %q hides=%v", quit.CloseAction, quit.hidesOnClose())
	}

	savePrefs(prefsFile{CloseAction: "somewhere-else"})
	unknown := loadPrefs()
	if unknown.CloseAction != closeToTray || !unknown.hidesOnClose() {
		t.Fatalf("unknown close = %q, want tray", unknown.CloseAction)
	}
}

// TestGeneralSettingsPersistsCloseAction checks the general pane offers both
// close behaviors and that choosing quit is written to preferences.
func TestGeneralSettingsPersistsCloseAction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1}
	a.Profiles = loadPrefs()

	te := ui.NewTester(func(c *ui.Context) {
		a.settingsGeneralPage(c, settingsPaletteFor(c))
	}, 900, 700)
	te.Frame()
	if !te.HasText("关闭窗口时") || !te.HasText("隐藏到托盘") || !te.HasText("直接退出") {
		t.Fatalf("close setting missing: %q", te.Texts())
	}
	if err := te.Click("直接退出"); err != nil {
		t.Fatalf("quit option not clickable: %v", err)
	}
	te.Frame()
	if a.Profiles.CloseAction != closeQuit || a.Profiles.hidesOnClose() {
		t.Fatalf("close action = %q, want quit", a.Profiles.CloseAction)
	}
	if got := loadPrefs().CloseAction; got != closeQuit {
		t.Fatalf("saved close action = %q, want quit", got)
	}
}

// TestThumbCacheMiBRoundTrip checks the thumbnail cache budget survives a
// save and load, defaults when absent, and is clamped to the slider range.
func TestThumbCacheMiBRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got := loadPrefs().ThumbCacheMiB; got != thumbBudget>>20 {
		t.Fatalf("default = %d, want %d", got, thumbBudget>>20)
	}
	p := loadPrefs()
	p.ThumbCacheMiB = 512
	savePrefs(p)
	if got := loadPrefs().ThumbCacheMiB; got != 512 {
		t.Fatalf("saved = %d, want 512", got)
	}
	for in, want := range map[int]int{10: 64, 5000: 1024, 0: thumbBudget >> 20} {
		p.ThumbCacheMiB = in
		savePrefs(p)
		if got := loadPrefs().ThumbCacheMiB; got != want {
			t.Fatalf("stored %d loaded as %d, want %d", in, got, want)
		}
	}
}

func TestMigrateViewInstallersWhenAbsent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Dir(prefsPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"viewSet":true,"view":{"images":true,"raw":true,"videos":true,"archives":true,"theme":"system"}}`)
	if err := os.WriteFile(prefsPath(), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := loadPrefs()
	if !loaded.View.AllTypes || !loaded.View.Images || !loaded.View.Videos {
		t.Fatalf("old all-type view should become 全部: %#v", loaded.View)
	}
}
