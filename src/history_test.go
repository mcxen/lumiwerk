package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The persisted history: saving, loading, dead-entry pruning and the
// restore decision. These tests isolate HOME first — historyPath writes
// under the user config directory.
func TestHistoryRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	for _, d := range []string{a, b} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	saveHistory(historyFile{History: []string{a, b}, Cursor: 1})
	got := loadHistory()
	if len(got.History) != 2 || got.Cursor != 1 {
		t.Fatalf("round trip = %v cursor %d", got.History, got.Cursor)
	}
	if got.CursorPath() != b {
		t.Fatalf("cursor path = %s, want %s", got.CursorPath(), b)
	}
}

func TestHistoryDropsDeadFolders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	live := filepath.Join(root, "live")
	if err := os.Mkdir(live, 0o755); err != nil {
		t.Fatal(err)
	}
	saveHistory(historyFile{History: []string{filepath.Join(root, "gone"), live}, Cursor: 0})
	got := loadHistory()
	if len(got.History) != 1 || got.History[0] != live || got.Cursor != 0 {
		t.Fatalf("pruned = %v cursor %d, want [%s] cursor 0", got.History, got.Cursor, live)
	}
}

func TestHistoryClampsBadCursor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// A live folder with an out-of-range cursor clamps to the newest entry.
	live := filepath.Join(t.TempDir(), "live")
	if err := os.Mkdir(live, 0o755); err != nil {
		t.Fatal(err)
	}
	got := loadHistoryFrom(t, historyFile{History: []string{live}, Cursor: 9})
	if got.Cursor != 0 {
		t.Fatalf("cursor = %d, want clamped to 0", got.Cursor)
	}
	// A cursor on a pruned entry falls back to the newest survivor.
	gone := filepath.Join(t.TempDir(), "gone")
	got = loadHistoryFrom(t, historyFile{History: []string{gone, live}, Cursor: 0})
	if got.Cursor != 0 || got.History[0] != live {
		t.Fatalf("after prune = %v cursor %d, want [%s] cursor 0", got.History, got.Cursor, live)
	}
	// An out-of-range cursor with an empty trail is -1.
	got = loadHistoryFrom(t, historyFile{History: nil, Cursor: 3})
	if got.Cursor != -1 {
		t.Fatalf("empty history cursor = %d, want -1", got.Cursor)
	}
	got = loadHistoryFrom(t, historyFile{History: nil, Cursor: -5})
	if got.Cursor != -1 {
		t.Fatalf("very negative cursor = %d, want -1", got.Cursor)
	}
}

// loadHistoryFrom writes h and loads it back, isolating HOME.
func loadHistoryFrom(t *testing.T, h historyFile) historyFile {
	t.Helper()
	saveHistory(h)
	return loadHistory()
}

func TestRestoreSessionDefaultsAndOptOut(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	p := loadPrefs()
	if !p.restoreSession() {
		t.Fatal("a fresh profile should restore the last session")
	}
	p.RestoreOff = true
	savePrefs(p)
	loaded := loadPrefs()
	if loaded.restoreSession() {
		t.Fatal("RestoreOff should persist and disable restore")
	}
	t.Setenv("LUMIWERK_NO_RESTORE", "1")
	p.RestoreOff = false
	if p.restoreSession() {
		t.Fatal("LUMIWERK_NO_RESTORE=1 should disable restore")
	}
}

// Session restore must remember the last folder as soon as it is opened,
// without waiting for the sidebar's 20s dwell. Tabs are not persisted yet.
func TestSessionPathPersistsImmediately(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := filepath.Join(t.TempDir(), "photos")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}

	a := &appState{Chosen: -1, LargeIndex: -1, Cursor: -1, Thumb: defaultThumb, LayoutMode: defaultLayout}
	a.openFolder(dir)

	got := loadHistory()
	if got.CursorPath() != dir {
		t.Fatalf("history after open = %v cursor %d path %q, want %s", got.History, got.Cursor, got.CursorPath(), dir)
	}
	if len(got.Recent) != 0 {
		t.Fatalf("sidebar recent should still wait for dwell, got %q", got.Recent)
	}

	// A second folder replaces the restore target immediately.
	a.openFolder(other)
	got = loadHistory()
	if got.CursorPath() != other {
		t.Fatalf("after navigate = %v cursor %d path %q, want %s", got.History, got.Cursor, got.CursorPath(), other)
	}

	// Dwell still gates Recent only.
	a.recordVisit(other, a.dwellSeq)
	got = loadHistory()
	if len(got.Recent) != 1 || got.Recent[0] != other {
		t.Fatalf("recent after dwell = %q", got.Recent)
	}
	if got.CursorPath() != other {
		t.Fatalf("cursor after dwell = %q", got.CursorPath())
	}

	// Fresh prefs keep restore on by default.
	p := loadPrefs()
	if !p.restoreSession() {
		t.Fatal("restore session should default on")
	}
}
