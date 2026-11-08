package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestListDragRowsUsesSelection(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jpg")
	b := filepath.Join(dir, "b.jpg")
	os.WriteFile(a, []byte("a"), 0o644)
	os.WriteFile(b, []byte("b"), 0o644)
	st := &appState{Chosen: 1, LargeIndex: -1, Root: dir}
	st.Items = []mediaItem{
		{Path: a, Name: "a.jpg", Ext: "jpg"},
		{Path: b, Name: "b.jpg", Ext: "jpg"},
	}
	st.Visible = []int{0, 1}
	st.Selected.Add(a)
	st.Selected.Add(b)
	got, _ := st.listDragRows([]int{1}).([]string)
	if len(got) != 2 {
		t.Fatalf("drag rows %v", got)
	}
}

func TestReceiveFolderDropMovesIntoFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "shot.jpg")
	if err := os.WriteFile(src, []byte("pic"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &appState{Chosen: -1, LargeIndex: -1, Root: root, Thumb: 128}
	a.receiveFolderDrop([]string{src}, album, true)
	dest := filepath.Join(album, "shot.jpg")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source still in place")
	}
}

func TestReceiveFolderDropRefusesSelf(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o755); err != nil {
		t.Fatal(err)
	}
	a := &appState{Chosen: -1, LargeIndex: -1, Root: root}
	a.receiveFolderDrop([]string{album}, album, true)
	if _, err := os.Stat(album); err != nil {
		t.Fatal("folder was consumed")
	}
}

func TestDetailListDropMovesFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "shot.jpg")
	if err := os.WriteFile(src, []byte("pic"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &appState{
		Chosen: -1, LargeIndex: -1, Thumb: 128, LayoutMode: 1,
		Root: root, SidebarWidth: sidebarDefaultWidth,
		Items: []mediaItem{
			{Path: album, Name: "album", IsDir: true, InfoLoaded: true, ModTime: 1},
			{Path: src, Name: "shot.jpg", Ext: "jpg", Size: 3, InfoLoaded: true, ModTime: 1},
		},
		Visible:  []int{0, 1},
		Profiles: prefsFile{View: defaultViewPreferences(), ViewSet: true},
	}
	te := ui.NewTester(func(c *ui.Context) { a.detailView(c) }, 720, 400)
	te.Frame()
	te.Frame()
	ar, ok := te.Find("album")
	if !ok {
		t.Fatalf("no album row: %v", te.Texts())
	}
	sr, ok := te.Find("shot.jpg")
	if !ok {
		t.Fatalf("no file row: %v", te.Texts())
	}
	te.Press(sr.X+8, sr.Y+sr.H/2)
	te.Move(sr.X+8, sr.Y+sr.H/2+8)
	te.Move(ar.X+8, ar.Y+ar.H/2)
	te.Release(ar.X+8, ar.Y+ar.H/2)
	te.Frame()
	dest := filepath.Join(album, "shot.jpg")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("list drop did not move: %v texts=%v", err, te.Texts())
	}
}
