package main

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The advanced settings pane shows the cache the app keeps and a button
// that empties it and sweeps the temporaries of crashed runs.
func TestAdvancedSettingsClearsCaches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 192}
	a.Profiles = loadPrefs()
	a.Settings.Pane = 4 // 高级

	// A cached thumbnail and a stale temp file, both gone after the click.
	// The cache is global; other tests' entries must not confuse the count.
	thumbLRU.Clear()
	bmp := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	thumbLRU.Put("stale", bmp, 128)
	dir := t.TempDir()
	origTempDir := osTempDir
	t.Cleanup(func() { osTempDir = origTempDir })
	osTempDir = func() string { return dir }
	stale := filepath.Join(dir, "lumiwerk-thumb-stale.jpg")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, past, past); err != nil {
		t.Fatal(err)
	}

	te := ui.NewTester(a.advancedSettings, 900, 640)
	te.Frame()
	te.Frame()
	if !te.HasText("清理缓存") || !te.HasText("清理") {
		t.Fatalf("cache cleanup section missing: %q", te.Texts())
	}

	if thumbLRU.Len() != 1 {
		t.Fatalf("cache holds %d entries, want 1", thumbLRU.Len())
	}
	if err := te.Click("清理"); err != nil {
		t.Fatalf("cleanup button not clickable: %v", err)
	}
	te.Frame()

	if thumbLRU.Len() != 0 || thumbLRU.Size() != 0 {
		t.Fatalf("cache not cleared: %d entries, %d bytes", thumbLRU.Len(), thumbLRU.Size())
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale temp survived the cleanup: %v", err)
	}
}
