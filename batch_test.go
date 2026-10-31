package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPreviewRenames(t *testing.T) {
	paths := []string{"/tmp/a IMG.png", "/tmp/b IMG.png"}
	got := previewRenames(paths, renameRule{Mode: "format", Pattern: "{name}_{num:3}"}, time.Unix(0, 0))
	if filepath.Base(got["/tmp/a IMG.png"]) != "a IMG_001.png" || filepath.Base(got["/tmp/b IMG.png"]) != "b IMG_002.png" {
		t.Fatalf("unexpected preview: %v", got)
	}
}

func TestApplyRenames(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(src, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyRenames(map[string]string{src: filepath.Join(dir, "new.txt")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("old file remains")
	}
}
