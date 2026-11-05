package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// renameState is a folder of real files shown in layout, item 1 chosen.
func renameState(t *testing.T, layout int) (*appState, string) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	a := &appState{Chosen: 1, LargeIndex: -1, Thumb: 128, Root: dir, LayoutMode: layout}
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("pic-%d.jpg", i)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		a.Items = append(a.Items, mediaItem{Path: path, Name: name, Ext: "jpg", InfoLoaded: true,
			Info: mediaInfo{Width: 800, Height: 600}, Decoded: true})
		a.Visible = append(a.Visible, i)
	}
	a.Selected.Add(a.Items[1].Path)
	return a, dir
}

func TestInlineRenameCommitsAndCancels(t *testing.T) {
	for _, layout := range []int{0, 1} {
		t.Run(fmt.Sprintf("layout-%d", layout), func(t *testing.T) {
			a, dir := renameState(t, layout)
			te := ui.NewTester(a.view, 1100, 760)
			te.Frame()
			te.Key(0, ui.KeyF2)
			for i := 0; i < 4; i++ {
				te.Frame()
			}
			if !a.Renaming || !a.RenameInline {
				t.Fatalf("F2 renaming=%v inline=%v", a.Renaming, a.RenameInline)
			}
			if active, _ := te.TextInputActive(); !active {
				t.Fatal("inline field has no focus")
			}
			// The name before .jpg is selected: typing replaces it.
			te.Type("t5 g")
			te.Frame()
			if a.RenameVal != "t5 g.jpg" {
				t.Fatalf("typed over selection = %q", a.RenameVal)
			}
			te.Key(0, ui.KeyEnter)
			te.Frame()
			if a.Renaming {
				t.Fatal("Enter left the field open")
			}
			if _, err := os.Stat(filepath.Join(dir, "t5 g.jpg")); err != nil {
				t.Fatalf("not renamed: %v", err)
			}
			if len(a.Undo.past) == 0 {
				t.Fatal("rename not undoable")
			}

			// Escape goes back.
			a.Items[2].Path, a.Items[2].Name = filepath.Join(dir, "pic-2.jpg"), "pic-2.jpg"
			a.chooseOnly(2)
			a.startRename()
			te.Frame()
			te.Frame()
			te.Type("zzz")
			te.Key(0, ui.KeyEscape)
			te.Frame()
			if a.Renaming {
				t.Fatal("Escape left the field open")
			}
			if _, err := os.Stat(filepath.Join(dir, "pic-2.jpg")); err != nil {
				t.Fatal("Escape renamed the file")
			}
		})
	}
}

func TestInlineRenameValidates(t *testing.T) {
	a, dir := renameState(t, 1)
	path := filepath.Join(dir, "pic-1.jpg")
	for name, bad := range map[string]bool{"": true, " ": true, "a/b": true, "pic-0.jpg": true, "pic-1.jpg": false, "PIC-1.jpg": false, "ok.jpg": false} {
		if err := checkRename(path, name); (err != nil) != bad {
			t.Errorf("checkRename(%q) = %v", name, err)
		}
	}
	if got := baseNameEnd("相片.final.jpg"); got != 8 {
		t.Errorf("baseNameEnd = %d", got)
	}
	if got := baseNameEnd(".hidden"); got != 7 {
		t.Errorf("baseNameEnd dotfile = %d", got)
	}
	// An existing name keeps the field open.
	te := ui.NewTester(a.view, 1100, 760)
	te.Frame()
	a.startRename()
	te.Frame()
	te.Frame()
	te.Type("pic-0")
	te.Frame()
	te.Key(0, ui.KeyEnter)
	te.Frame()
	if !a.Renaming {
		t.Fatal("conflicting name closed the field")
	}
}

func TestRenameFallsBackToDialog(t *testing.T) {
	a, _ := renameState(t, 2) // waterfall shows no name
	a.startRename()
	if !a.Renaming || a.RenameInline {
		t.Fatalf("waterfall inline=%v", a.RenameInline)
	}
	a, _ = renameState(t, 1)
	a.InLargeView = true
	a.startRename()
	if a.RenameInline {
		t.Fatal("large view renamed inline")
	}
}

func TestSlowSecondClickRenames(t *testing.T) {
	a, _ := renameState(t, 1)
	te := ui.NewTester(a.view, 1100, 760)
	te.Frame()
	te.Frame()
	r, ok := te.Find("pic-1.jpg")
	if !ok {
		t.Fatal(te.Texts())
	}
	te.Move(r.X+40, r.Y+r.H/2)
	te.Frame()
	te.ClickAt(r.X+40, r.Y+r.H/2)
	te.Frame()
	if a.renameArm == "" {
		t.Fatal("click on the chosen name did not arm a rename")
	}
	a.renameArmAt = time.Now().Add(-time.Millisecond)
	te.Frame()
	if !a.Renaming || !a.RenameInline {
		t.Fatal("slow second click did not start the inline rename")
	}
}
