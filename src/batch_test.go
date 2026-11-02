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

func TestBatchRenamePipeline(t *testing.T) {
	paths := []string{"/album/a IMG.png", "/album/b IMG.png"}
	rule := renameRule{Prefix: "P_", Suffix: "_S", Find: "IMG", Replace: "PIC", Pattern: "{folder}_{name}_{index:02}"}
	got := previewRenames(paths, rule, time.Unix(0, 0))
	if filepath.Base(got["/album/a IMG.png"]) != "P_album_a PIC_01_S.png" {
		t.Fatalf("first = %s", filepath.Base(got["/album/a IMG.png"]))
	}
	if filepath.Base(got["/album/b IMG.png"]) != "P_album_b PIC_02_S.png" {
		t.Fatalf("second = %s", filepath.Base(got["/album/b IMG.png"]))
	}
	kept := previewRenames([]string{"/album/a.png"}, renameRule{Pattern: "{name}.{ext}"}, time.Unix(0, 0))
	if filepath.Base(kept["/album/a.png"]) != "a.png" {
		t.Fatalf("ext doubled: %s", filepath.Base(kept["/album/a.png"]))
	}
}
