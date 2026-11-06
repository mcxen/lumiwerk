package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderNameRenameSingleUsesParent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "旅行")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "DSC_0001.jpg")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := folderNameRenameMappings([]string{src})
	if len(got) != 1 {
		t.Fatalf("mappings = %d", len(got))
	}
	if filepath.Base(got[0].To) != "旅行.jpg" {
		t.Fatalf("got %s, want 旅行.jpg", filepath.Base(got[0].To))
	}
}

func TestFolderNameRenameBatchPadsByCount(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "旅行")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	got := folderNameRenameMappings(paths)
	want := []string{"旅行_001.jpg", "旅行_002.jpg", "旅行_003.jpg"}
	for i := range got {
		if filepath.Base(got[i].To) != want[i] {
			t.Fatalf("[%d] = %s, want %s", i, filepath.Base(got[i].To), want[i])
		}
	}
}

func TestFolderNameRenameConflictGetsSuffix(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "旅行")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "旅行.jpg")
	if err := os.WriteFile(blocker, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "DSC.jpg")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := folderNameRenameMappings([]string{src})
	if filepath.Base(got[0].To) != "旅行_2.jpg" {
		t.Fatalf("got %s, want 旅行_2.jpg", filepath.Base(got[0].To))
	}
}

func TestFolderNameRenameTwoPhaseNoOverwrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "相册")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "b.jpg")
	b := filepath.Join(dir, "a.jpg")
	if err := os.WriteFile(a, []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	mappings := folderNameRenameMappings([]string{b, a})
	if err := applyRenameMappings(mappings); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "相册_001.jpg"))
	if err != nil || string(data) != "B" {
		t.Fatalf("001 = %q err=%v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(dir, "相册_002.jpg"))
	if err != nil || string(data) != "A" {
		t.Fatalf("002 = %q err=%v", data, err)
	}
}

func TestFolderNamePadWidth(t *testing.T) {
	if folderNamePadWidth(5) != 3 || folderNamePadWidth(12) != 3 || folderNamePadWidth(100) != 3 {
		t.Fatalf("pad width unexpected: 5=%d 12=%d 100=%d", folderNamePadWidth(5), folderNamePadWidth(12), folderNamePadWidth(100))
	}
	if folderNamePadWidth(1000) != 4 {
		t.Fatalf("1000 → %d, want 4", folderNamePadWidth(1000))
	}
}
