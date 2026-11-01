package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// navHarness runs background results on the test goroutine.
type navHarness struct {
	mu    sync.Mutex
	queue []func()
}

func newNavHarness(t *testing.T) *navHarness {
	h := &navHarness{}
	headlessUpdate = func(fn func()) { h.mu.Lock(); h.queue = append(h.queue, fn); h.mu.Unlock() }
	t.Cleanup(func() { headlessUpdate = nil })
	return h
}

// drain runs queued updates until cond holds.
func (h *navHarness) drain(t *testing.T, te *ui.Tester, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		q := h.queue
		h.queue = nil
		h.mu.Unlock()
		for _, fn := range q {
			fn()
		}
		if te != nil {
			te.Frame()
		}
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

func writePNG(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
}

// navTree makes root/{a.png, b.png, c.png, child/x.png}.
func navTree(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	for _, n := range []string{"a.png", "b.png", "c.png"} {
		writePNG(t, filepath.Join(root, n))
	}
	if err := os.Mkdir(filepath.Join(root, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(root, "child", "x.png"))
	return root
}

func navApp(t *testing.T, root string) (*appState, *ui.Tester, *navHarness) {
	h := newNavHarness(t)
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, Cursor: -1}
	a.Profiles = loadPrefs()
	te := ui.NewTester(a.view, 1100, 760)
	a.openFolder(root)
	h.drain(t, te, func() bool { return len(a.Visible) == 4 })
	return a, te, h
}

func (a *appState) visibleIndexOf(path string) int {
	for vi, ii := range a.Visible {
		if a.Items[ii].Path == path {
			return vi
		}
	}
	return -1
}

func chosenPath(a *appState) string {
	if it := a.visibleItem(a.Chosen); it != nil {
		return it.Path
	}
	return ""
}

func TestDoubleClickFolderEnters(t *testing.T) {
	root := navTree(t)
	a, te, h := navApp(t, root)
	r, ok := te.Find("child")
	if !ok {
		t.Fatal("folder card not shown")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	te.ClickAt(x, y)
	te.ClickAt(x, y)
	child := filepath.Join(root, "child")
	h.drain(t, te, func() bool { return a.Root == child && len(a.Visible) == 1 })
}

func TestEnterAndCmdDownOpenFolder(t *testing.T) {
	root := navTree(t)
	child := filepath.Join(root, "child")
	for _, key := range []struct {
		mods ui.Modifiers
		k    ui.Key
	}{{0, ui.KeyEnter}, {ui.Cmd, ui.KeyDown}} {
		a, te, h := navApp(t, root)
		a.Profiles.EnterOpens = true
		a.Chosen = a.visibleIndexOf(child)
		a.Selected.Clear()
		a.Selected.Add(child)
		te.Frame()
		te.Key(key.mods, key.k)
		h.drain(t, te, func() bool { return a.Root == child })
	}
}

func TestCmdUpSelectsChild(t *testing.T) {
	root := navTree(t)
	child := filepath.Join(root, "child")
	a, te, h := navApp(t, root)
	a.openFolder(child)
	h.drain(t, te, func() bool { return a.Root == child && len(a.Visible) == 1 })
	a.Chosen = 0
	te.Frame()
	te.Key(ui.Cmd, ui.KeyUp)
	h.drain(t, te, func() bool { return a.Root == root && len(a.Visible) == 4 })
	if chosenPath(a) != child || !a.Selected.Has(child) {
		t.Fatalf("after going up, chosen %q, want %q", chosenPath(a), child)
	}
}

func TestSelectionSurvivesRescan(t *testing.T) {
	root := navTree(t)
	a, te, h := navApp(t, root)
	b := filepath.Join(root, "b.png")
	a.Chosen = a.visibleIndexOf(b)
	a.Selected.Clear()
	a.Selected.Add(b)
	te.Frame()

	// A new file sorted before b shifts indexes; b stays chosen.
	writePNG(t, filepath.Join(root, "a0.png"))
	a.refresh()
	h.drain(t, te, func() bool { return len(a.Visible) == 5 })
	if chosenPath(a) != b || !a.Selected.Has(b) {
		t.Fatalf("after rescan chosen %q", chosenPath(a))
	}

	// Deleting b selects its next neighbour.
	os.Remove(b)
	a.refresh()
	h.drain(t, te, func() bool { return len(a.Visible) == 4 })
	c := filepath.Join(root, "c.png")
	if chosenPath(a) != c || !a.Selected.Has(c) || a.Selected.Has(b) || a.Selected.Len() != 1 {
		t.Fatalf("after delete chosen %q, selected %d", chosenPath(a), a.Selected.Len())
	}

	// Deleting the last item selects the previous one.
	last := chosenPath(a)
	a.Chosen = len(a.Visible) - 1
	last = chosenPath(a)
	a.Selected.Clear()
	a.Selected.Add(last)
	os.RemoveAll(last)
	a.refresh()
	h.drain(t, te, func() bool { return len(a.Visible) == 3 })
	if a.Chosen != len(a.Visible)-1 || chosenPath(a) == "" || !a.Selected.Has(chosenPath(a)) {
		t.Fatalf("after deleting the last item chosen %d %q", a.Chosen, chosenPath(a))
	}
}

func TestDoubleClickFolderEntersInFlowLayouts(t *testing.T) {
	for _, layout := range []int{2, 3} {
		root := navTree(t)
		a, te, h := navApp(t, root)
		a.LayoutMode = layout
		te.Frame()
		r, ok := te.Find("child")
		if !ok {
			t.Fatalf("layout %d: folder card not shown", layout)
		}
		x, y := r.X+r.W/2, r.Y-30
		te.ClickAt(x, y)
		te.Frame()
		if chosenPath(a) != filepath.Join(root, "child") {
			t.Fatalf("layout %d: click chose %q", layout, chosenPath(a))
		}
		te.ClickAt(x, y)
		child := filepath.Join(root, "child")
		h.drain(t, te, func() bool { return a.Root == child })
	}
}

// Cmd+Up still goes up after a click put the focus in the grid.
func TestCmdUpWithGridFocus(t *testing.T) {
	root := navTree(t)
	child := filepath.Join(root, "child")
	a, te, h := navApp(t, root)
	a.openFolder(child)
	h.drain(t, te, func() bool { return a.Root == child && len(a.Visible) == 1 })
	r, ok := te.Find("x.png")
	if !ok {
		t.Fatal("x.png not shown")
	}
	te.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	te.Frame()
	te.Key(ui.Cmd, ui.KeyUp)
	h.drain(t, te, func() bool { return a.Root == root && len(a.Visible) == 4 })
	if chosenPath(a) != child {
		t.Fatalf("chosen %q after Cmd+Up", chosenPath(a))
	}
	// Back returns into the child; going back up again selects it.
	te.Key(ui.Cmd, ui.KeyBracketLeft)
	h.drain(t, te, func() bool { return a.Root == child })
}

func TestSelectionAfterRenameUndoAndDelete(t *testing.T) {
	root := navTree(t)
	a, te, h := navApp(t, root)
	b := filepath.Join(root, "b.png")
	a.chooseOnly(a.visibleIndexOf(b))
	te.Frame()

	// Rename b to a name that sorts last among files: still chosen.
	if err := a.performTracked(fileOperation{op: opRename, paths: []string{b}, newName: "zz.png"}); err != nil {
		t.Fatal(err)
	}
	zz := filepath.Join(root, "zz.png")
	h.drain(t, te, func() bool { return a.visibleIndexOf(zz) >= 0 && chosenPath(a) == zz })
	if chosenPath(a) != zz || !a.Selected.Has(zz) || a.Selected.Len() != 1 {
		t.Fatalf("after rename chosen %q", chosenPath(a))
	}

	// Undo: back to b, still chosen.
	a.undo()
	h.drain(t, te, func() bool { return a.visibleIndexOf(b) >= 0 && chosenPath(a) == b })
	if chosenPath(a) != b || !a.Selected.Has(b) {
		t.Fatalf("after undo chosen %q", chosenPath(a))
	}
	a.redo()
	h.drain(t, te, func() bool { return a.visibleIndexOf(zz) >= 0 && chosenPath(a) == zz })
	if chosenPath(a) != zz {
		t.Fatalf("after redo chosen %q", chosenPath(a))
	}

	// Delete a (first): the next item takes the selection.
	first := filepath.Join(root, "a.png")
	a.chooseOnly(a.visibleIndexOf(first))
	next := a.visibleItem(a.Chosen + 1).Path
	if err := a.performTracked(fileOperation{op: opDelete, paths: []string{first}, permanent: true}); err != nil {
		t.Fatal(err)
	}
	h.drain(t, te, func() bool { return a.visibleIndexOf(first) < 0 })
	if chosenPath(a) != next || !a.Selected.Has(next) || a.Selected.Len() != 1 {
		t.Fatalf("after delete chosen %q, want %q", chosenPath(a), next)
	}

	// A delete undone moves the file back from the Trash: it is reselected.
	writePNG(t, first)
	a.finishOperation(fileOperation{op: opMove, mappings: []pathPair{{From: "/trash/a.png", To: first}}})
	h.drain(t, te, func() bool { return a.visibleIndexOf(first) >= 0 })
	if chosenPath(a) != first || !a.Selected.Has(first) {
		t.Fatalf("after restoring chosen %q", chosenPath(a))
	}
}

func TestMultiSelectionSurvivesRescan(t *testing.T) {
	root := navTree(t)
	a, te, h := navApp(t, root)
	b, c := filepath.Join(root, "b.png"), filepath.Join(root, "c.png")
	a.chooseOnly(a.visibleIndexOf(b))
	a.Selected.Add(c)
	writePNG(t, filepath.Join(root, "a0.png"))
	a.refresh()
	h.drain(t, te, func() bool { return len(a.Visible) == 5 })
	if a.Selected.Len() != 2 || !a.Selected.Has(b) || !a.Selected.Has(c) || chosenPath(a) != b {
		t.Fatalf("selection after rescan: %d, chosen %q", a.Selected.Len(), chosenPath(a))
	}
}
