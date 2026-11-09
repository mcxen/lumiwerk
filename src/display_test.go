package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnabledFileTypesAreIndependent(t *testing.T) {
	view := defaultViewPreferences()
	for _, name := range []string{"photo.jpg", "movie.mp4"} {
		if !isEnabledFile(name, filepath.Ext(name), view) {
			t.Fatalf("default filters excluded %s", name)
		}
	}
	for _, name := range []string{"camera.dng", "bundle.zip", "Setup.dmg", "notes.txt"} {
		if isEnabledFile(name, filepath.Ext(name), view) {
			t.Fatalf("default filters included %s", name)
		}
	}
	view.RAW, view.Archives = true, true
	view.Images = false
	if isEnabledFile("photo.jpg", "jpg", view) || !isEnabledFile("camera.dng", "dng", view) {
		t.Fatal("disabling ordinary images also changed RAW visibility")
	}
	view.Archives = false
	if isEnabledFile("bundle.zip", "zip", view) {
		t.Fatal("archive remained visible after its filter was disabled")
	}
	view.AllTypes = true
	if isEnabledFile("notes.txt", "txt", view) {
		t.Fatal("all-types must be applied by the scanner, not bypass the type function")
	}
}

func TestHiddenPathChecksEachPathComponent(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "media")
	for _, path := range []string{".photo.jpg", filepath.Join(".private", "photo.jpg")} {
		if !isHiddenPath(root, filepath.Join(root, path)) {
			t.Fatalf("hidden path was visible: %s", path)
		}
	}
	if isHiddenPath(root, filepath.Join(root, "visible", "photo.jpg")) {
		t.Fatal("ordinary path was treated as hidden")
	}
}

func TestAutoConfigValidationAndPreferences(t *testing.T) {
	// savePrefs writes the real preferences file, so keep this test inside a
	// temporary HOME instead of clobbering the user's configuration slots.
	t.Setenv("HOME", t.TempDir())

	a := &appState{Profiles: prefsFile{View: defaultViewPreferences(), ViewSet: true}}
	a.AutoConfig, a.AutoValue = "scroll", "0"
	a.commitAutoConfig()
	if a.AutoScroll || a.AutoConfig == "" {
		t.Fatal("invalid speed started auto scroll")
	}
	a.AutoValue = "120"
	a.commitAutoConfig()
	if !a.AutoScroll || a.Profiles.View.AutoScrollSpeed != 120 || a.AutoConfig != "" {
		t.Fatalf("auto scroll state = %#v", a)
	}
	a.AutoConfig, a.AutoValue = "play", "2.5"
	a.commitAutoConfig()
	if !a.AutoPlay || a.Profiles.View.AutoPlayInterval != 2.5 {
		t.Fatal("auto play interval was not applied")
	}
}

func TestAutoTickWaitsForInterval(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	due, advance := nextAutoTick(now, time.Time{}, 2)
	if advance || !due.Equal(now.Add(2*time.Second)) {
		t.Fatalf("first tick due=%v advance=%v", due, advance)
	}
	if _, advance := nextAutoTick(now.Add(time.Second), due, 2); advance {
		t.Fatal("autoplay advanced before its interval")
	}
	if _, advance := nextAutoTick(now.Add(2*time.Second), due, 2); !advance {
		t.Fatal("autoplay did not advance at its interval")
	}
}

func TestPreferencesKeepExplicitFalseValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prefs := prefsFile{View: defaultViewPreferences(), ViewSet: true}
	prefs.View.Images = false
	prefs.View.AutoPlaySelected = false
	prefs.View.InternalPlayer = false
	prefs.View.Theme = "dark"
	savePrefs(prefs)
	loaded := loadPrefs()
	if !loaded.ViewSet || loaded.View.Images || loaded.View.AutoPlaySelected || loaded.View.InternalPlayer {
		t.Fatalf("explicit false preferences were lost: %#v", loaded)
	}
	if loaded.View.Theme != "dark" || loaded.View.AutoScrollSpeed != 60 {
		t.Fatalf("theme/default speed not restored: %#v", loaded.View)
	}
}

func TestMissingPreferencesShowEveryFileType(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prefs := loadPrefs()
	if !prefs.View.AllTypes || !prefs.View.InternalPlayer {
		t.Fatalf("missing preferences did not default to all types: %#v", prefs.View)
	}
}

func TestFilterDropsHiddenMediaAndInvalidSelection(t *testing.T) {
	a := &appState{Chosen: 1}
	a.Items = []mediaItem{{Name: "photo.jpg", Ext: "jpg"}, {Name: "movie.mp4", Ext: "mp4", IsVideo: true}}
	a.Selected.Add("/old-selection")
	a.FilterMode = 1
	a.applyFilterLocked()
	if len(a.Visible) != 1 || a.Visible[0] != 0 || a.Chosen != -1 {
		t.Fatalf("visible=%v chosen=%d", a.Visible, a.Chosen)
	}
	a.FilterMode = 0
	a.applyFilterLocked()
	if len(a.Visible) != 2 {
		t.Fatal("restoring the filter did not restore all media")
	}
}

func TestMediaCountExcludesFoldersAndOtherFiles(t *testing.T) {
	a := &appState{Items: []mediaItem{
		{Path: "/folder", IsDir: true}, {Path: "/photo.jpg", Ext: "jpg"},
		{Path: "/movie.mp4", Ext: "mp4"}, {Path: "/notes.txt", Ext: "txt"},
	}}
	a.Visible = []int{0, 1, 2, 3}
	if got := a.mediaCount(); got != 2 {
		t.Fatalf("media count = %d", got)
	}
}

func TestOldTypeDefaultsMigrateToAllTypes(t *testing.T) {
	base := viewPreferences{Theme: "system", AutoScrollSpeed: 60, AutoPlayInterval: 2}
	for name, view := range map[string]viewPreferences{
		"every type on": {Images: true, RAW: true, Videos: true, Archives: true, Installers: true},
		"media only":    {Images: true, Videos: true},
	} {
		t.Setenv("HOME", t.TempDir())
		view.Theme, view.AutoScrollSpeed, view.AutoPlayInterval = base.Theme, base.AutoScrollSpeed, base.AutoPlayInterval
		savePrefs(prefsFile{ViewSet: true, View: view})
		loaded := loadPrefs()
		if !loaded.View.AllTypes || loaded.TypeFilterRev != typeFilterRevAll {
			t.Fatalf("%s: did not move to 全部: %#v rev %d", name, loaded.View, loaded.TypeFilterRev)
		}
	}

	// A view the user customized is kept, and a later 自定义 choice is not
	// turned back into 全部 on the next launch.
	t.Setenv("HOME", t.TempDir())
	savePrefs(prefsFile{ViewSet: true, View: viewPreferences{Images: true, Archives: true, Theme: "system"}})
	kept := loadPrefs()
	if kept.View.AllTypes || !kept.View.Archives || kept.View.Videos {
		t.Fatalf("custom types were rewritten: %#v", kept.View)
	}
	kept.View.AllTypes = false
	kept.View.Images, kept.View.Videos = true, true
	savePrefs(kept)
	if again := loadPrefs(); again.View.AllTypes {
		t.Fatalf("explicit 自定义 was reset: %#v", again.View)
	}
}

func TestScanVisibilityFixture(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"photo.jpg", "camera.dng", "movie.mp4", "bundle.zip", "notes.txt", ".hidden.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	view := defaultViewPreferences()
	view.RAW, view.Archives = true, true
	for _, name := range []string{"photo.jpg", "camera.dng", "movie.mp4", "bundle.zip"} {
		if !isEnabledFile(name, filepath.Ext(name), view) || isHiddenPath(root, filepath.Join(root, name)) {
			t.Fatalf("enabled scan excluded %s", name)
		}
	}
	if !isHiddenPath(root, filepath.Join(root, ".hidden.jpg")) {
		t.Fatal("hidden fixture was included")
	}
}
