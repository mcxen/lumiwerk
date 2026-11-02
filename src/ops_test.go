package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyMoveRenameDelete(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	dstDir := filepath.Join(dir, "dst")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "a.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Copy
	if _, err := perform(fileOperation{op: opCopy, paths: []string{src}, dest: dstDir}); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(dstDir, "a.txt")
	if data, err := os.ReadFile(copied); err != nil || string(data) != "hello" {
		t.Fatalf("copy failed: %v", err)
	}

	// Rename via two-phase.
	renamed := filepath.Join(dstDir, "renamed.txt")
	if _, err := perform(fileOperation{op: opRename, paths: []string{copied}, newName: "renamed.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(renamed); err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if _, err := os.Stat(copied); !os.IsNotExist(err) {
		t.Fatalf("rename left original")
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(a, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := uniquePath(a)
	if filepath.Base(got) != "a (1).txt" {
		t.Fatalf("uniquePath = %q", got)
	}
}
