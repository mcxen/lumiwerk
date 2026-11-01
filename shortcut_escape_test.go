package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Regression for the double-Shortcut consumption bug: MyGo delivers a
// pressed key to the FIRST Shortcut(0, key) query of a build; the old
// modal/large if-else chain queried Escape twice, so the modal query
// consumed every Escape and the large view never closed.
func TestEscapeClosesLargeImageView(t *testing.T) {
	a := newLargeState()
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	if !a.InLargeView {
		t.Fatal("setup: large view not open")
	}
	te.Key(0, ui.KeyEscape)
	te.Frame()
	if a.InLargeView {
		t.Fatal("Escape did not close large view")
	}
}

// Same for a video item, whose large view hosts the player.
func TestEscapeClosesVideoLargeView(t *testing.T) {
	a := newLargeState()
	a.Items = []mediaItem{{Path: "/tmp/v.mp4", Name: "v.mp4", IsVideo: true, Ext: "mp4", InfoLoaded: true}}
	a.Visible = []int{0}
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	if !a.InLargeView || !a.videoLargeView() {
		t.Fatalf("setup: video large view not open (InLargeView=%v)", a.InLargeView)
	}
	te.Key(0, ui.KeyEscape)
	te.Frame()
	if a.InLargeView {
		t.Fatal("Escape did not close video large view")
	}
}

// An open modal still takes priority: Escape closes it, not the view.
func TestEscapeClosesModalFirst(t *testing.T) {
	a := newLargeState()
	a.InfoAsk = true
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	te.Key(0, ui.KeyEscape)
	te.Frame()
	if a.InfoAsk {
		t.Fatal("Escape did not close the info modal")
	}
	if !a.InLargeView {
		t.Fatal("Escape closed the large view together with the modal")
	}
}

// Escape with nothing open changes nothing.
func TestEscapeIdleInBrowserIsNoop(t *testing.T) {
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 192, FolderFirst: true}
	a.Root = "/tmp"
	a.Items = []mediaItem{{Path: "/tmp/a.png", Name: "a.png", InfoLoaded: true}}
	a.Visible = []int{0}
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	te.Key(0, ui.KeyEscape)
	te.Frame()
	if a.InLargeView {
		t.Fatal("large view opened by Escape?!")
	}
}

// Enter still closes the image large view (its own single query).
func TestEnterClosesLargeImageView(t *testing.T) {
	a := newLargeState()
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	te.Key(0, ui.KeyEnter)
	te.Frame()
	if a.InLargeView {
		t.Fatal("Enter did not close large view")
	}
}

// Return enters a selected folder when 打开/关闭 is the chosen Enter
// behavior (enterOpens).
func TestReturnOpensSelectedFolder(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	a := &appState{Chosen: 0, LargeIndex: -1, Thumb: 192, FolderFirst: true}
	a.Root = base
	a.Items = []mediaItem{{Path: sub, Name: "sub", IsDir: true, Decoded: true, InfoLoaded: true}}
	a.Visible = []int{0}
	a.Profiles.EnterOpens = true
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	te.Key(0, ui.KeyEnter)
	te.Frame()
	if a.Root != sub {
		t.Fatalf("Enter did not enter folder; root=%q", a.Root)
	}
}

// With the default (rename) behavior, Enter starts a rename instead.
func TestReturnStartsRenameByDefault(t *testing.T) {
	a := &appState{Chosen: 0, LargeIndex: -1, Thumb: 192, FolderFirst: true}
	a.Root = "/tmp"
	a.Items = []mediaItem{{Path: "/tmp/a.png", Name: "a.png", InfoLoaded: true}}
	a.Visible = []int{0}
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	te.Key(0, ui.KeyEnter)
	te.Frame()
	if !a.Renaming {
		t.Fatalf("Enter did not start rename; Renaming=%v root=%q", a.Renaming, a.Root)
	}
}
