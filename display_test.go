package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnabledFileTypesAreIndependent(t *testing.T) {
	view := defaultViewPreferences()
	for _, name := range []string{"photo.jpg", "camera.dng", "movie.mp4", "bundle.zip"} {
		if !isEnabledFile(name, filepath.Ext(name), view) {
			t.Fatalf("default filters excluded %s", name)
		}
	}
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

func TestMissingPreferencesUseVisibleMediaDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prefs := loadPrefs()
	if !prefs.View.Images || !prefs.View.RAW || !prefs.View.Videos || !prefs.View.Archives || !prefs.View.InternalPlayer {
		t.Fatalf("missing preferences did not get defaults: %#v", prefs.View)
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
	for _, name := range []string{"photo.jpg", "camera.dng", "movie.mp4", "bundle.zip"} {
		if !isEnabledFile(name, filepath.Ext(name), view) || isHiddenPath(root, filepath.Join(root, name)) {
			t.Fatalf("default scan excluded %s", name)
		}
	}
	if !isHiddenPath(root, filepath.Join(root, ".hidden.jpg")) {
		t.Fatal("hidden fixture was included")
	}
}
