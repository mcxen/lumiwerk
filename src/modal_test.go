package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRenameModalEscapeDoesNotCommit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	src := filepath.Join(root, "a.jpg")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &appState{Chosen: 0, LargeIndex: -1, Thumb: 128, Root: root}
	a.Items = []mediaItem{{Path: src, Name: "a.jpg", Ext: "jpg"}}
	a.Visible = []int{0}
	a.Renaming = true
	a.RenameInline = false
	a.RenamePath = src
	a.RenameVal = "b.jpg"
	te := ui.NewTester(a.view, 720, 480)
	te.Frame()
	if !te.HasText("重命名") {
		t.Fatalf("modal missing: %v", te.Texts())
	}
	te.Key(0, ui.KeyEscape)
	te.Frame()
	if a.Renaming {
		t.Fatal("escape left rename open")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("escape renamed the file")
	}
}

func TestRenameModalOKCommits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	src := filepath.Join(root, "a.jpg")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &appState{Chosen: 0, LargeIndex: -1, Thumb: 128, Root: root}
	a.Items = []mediaItem{{Path: src, Name: "a.jpg", Ext: "jpg"}}
	a.Visible = []int{0}
	a.Selected.Add(src)
	a.Renaming = true
	a.RenameInline = false
	a.RenamePath = src
	a.RenameVal = "b.jpg"
	te := ui.NewTester(a.view, 720, 480)
	te.Frame()
	if err := te.Click("确定"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if _, err := os.Stat(filepath.Join(root, "b.jpg")); err != nil {
		t.Fatalf("ok did not rename: %v", err)
	}
}

func TestFolderModalEscapeDoesNotCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, Root: root, NewFolder: true, FolderName: "fresh"}
	te := ui.NewTester(a.view, 720, 480)
	te.Frame()
	if !te.HasText("新建文件夹") {
		t.Fatalf("modal missing: %v", te.Texts())
	}
	te.Key(0, ui.KeyEscape)
	te.Frame()
	if a.NewFolder {
		t.Fatal("escape left folder dialog open")
	}
	if _, err := os.Stat(filepath.Join(root, "fresh")); !os.IsNotExist(err) {
		t.Fatal("escape created a folder")
	}
}

func TestFolderModalOKCreates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, Root: root, NewFolder: true, FolderName: "fresh"}
	te := ui.NewTester(a.view, 720, 480)
	te.Frame()
	if err := te.Click("确定"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if _, err := os.Stat(filepath.Join(root, "fresh")); err != nil {
		t.Fatalf("ok did not create: %v", err)
	}
}

func TestZipModalEscapeClearsPassword(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, ZipAsk: true, ZipPass: "secret-pass"}
	te := ui.NewTester(a.view, 720, 480)
	te.Frame()
	if !te.HasText("加密压缩 ZIP") {
		t.Fatalf("modal missing: %v", te.Texts())
	}
	te.Key(0, ui.KeyEscape)
	te.Frame()
	if a.ZipAsk {
		t.Fatal("escape left zip dialog open")
	}
	if a.ZipPass != "" {
		t.Fatal("dismiss left a password")
	}
}
