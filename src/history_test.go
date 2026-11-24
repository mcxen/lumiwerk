package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
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

func TestHistoryRemoveRemapPruneAndBackup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	d1 := filepath.Join(root, "alpha")
	d2 := filepath.Join(root, "beta")
	d2Sub := filepath.Join(d2, "sub")
	d1New := filepath.Join(root, "alpha-renamed")
	for _, d := range []string{d1, d2, d2Sub, d1New} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	a := &appState{
		Chosen:     -1,
		LargeIndex: -1,
		History:    []string{d1, d2, d2Sub},
		Cursor:     2,
		Recent:     []string{d1, d2, d2Sub},
		Visited: map[string]int64{
			d1:                       100,
			d2:                       200,
			d2Sub:                    300,
			filepath.Join(root, "z"): 50, // unreferenced -> pruned on save
		},
	}
	a.persistBrowsingHistory()
	// Second save rotates history.json to history.json.bak.
	a.Visited[d1] = 150
	a.persistBrowsingHistory()
	if _, err := os.Stat(historyPath() + ".bak"); err != nil {
		t.Fatalf("expected history.json.bak: %v", err)
	}
	loaded := loadHistory()
	if _, ok := loaded.Visited[filepath.Join(root, "z")]; ok {
		t.Fatalf("stale visited entry survived save/load: %+v", loaded.Visited)
	}

	// Single entry removal drops d2 from Recent + Visited while keeping History.
	a.removeHistoryEntry(d2)
	if !reflect.DeepEqual(a.Recent, []string{d1, d2Sub}) {
		t.Fatalf("Recent after remove = %q, want [d1 d2Sub]", a.Recent)
	}
	if _, ok := a.Visited[d2]; ok {
		t.Fatal("Visited still holds removed entry d2")
	}
	if !reflect.DeepEqual(a.History, []string{d1, d2, d2Sub}) {
		t.Fatalf("History mutated by removeHistoryEntry: %q", a.History)
	}

	// Rename/move remaps History, Recent, and Visited.
	a.remapHistoryPaths([]pathPair{{From: d1, To: d1New}})
	if a.History[0] != d1New || a.Recent[0] != d1New || a.Visited[d1New] != 150 {
		t.Fatalf("remap failed: hist=%q recent=%q visited=%v", a.History, a.Recent, a.Visited)
	}

	// Deleting d2 prunes both d2 and d2Sub across History, Recent, and Visited, keeping Cursor valid.
	a.pruneHistoryPaths([]string{d2})
	if !reflect.DeepEqual(a.Recent, []string{d1New}) || !reflect.DeepEqual(a.History, []string{d1New}) {
		t.Fatalf("prune failed: hist=%q recent=%q", a.History, a.Recent)
	}
	if a.Cursor != 0 || a.Visited[d2Sub] != 0 {
		t.Fatalf("cursor=%d visited=%v after prune", a.Cursor, a.Visited)
	}
}

func TestSidebarHistoryDisambiguatesAndRemovesEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	treeRoot := t.TempDir()
	histRoot := t.TempDir()
	dirA := filepath.Join(histRoot, "parentA", "photos")
	dirB := filepath.Join(histRoot, "parentB", "photos")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	a := &appState{
		Root:         treeRoot,
		Chosen:       -1,
		LargeIndex:   -1,
		Cursor:       -1,
		Thumb:        defaultThumb,
		LayoutMode:   defaultLayout,
		SidebarShown: true,
		Recent:       []string{dirA, dirB},
		Visited:      map[string]int64{dirA: time.Now().Unix()},
	}
	te := ui.NewTester(func(c *ui.Context) { a.sidebarView(c) }, 320, 640)
	te.Frame()
	te.Frame()

	// Duplicate folder names show parent directory suffixes.
	if !te.HasText("parentA") || !te.HasText("parentB") {
		t.Fatalf("expected parent disambiguation labels in sidebar: %q", te.Texts())
	}

	box, ok := te.Find("parentA")
	if !ok {
		t.Fatal("parentA label not found")
	}
	te.Move(box.X+box.W/2, box.Y+box.H/2)
	te.Frame()
	if err := te.Click("从历史中移除"); err != nil {
		t.Fatalf("remove button click failed: %v", err)
	}
	te.Frame()

	if !reflect.DeepEqual(a.Recent, []string{dirB}) {
		t.Fatalf("Recent after clicking hover remove = %q, want [%s]", a.Recent, dirB)
	}
	if a.Root != treeRoot {
		t.Fatalf("clicking hover remove navigated Root to %q, want %q", a.Root, treeRoot)
	}
}
