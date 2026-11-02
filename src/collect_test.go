package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectSubfolderFilesFlattens(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "A")
	b := filepath.Join(root, "B", "nested")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, "one.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "one.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "B", "skip-dir-only"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local)
	got, err := collectSubfolderFiles([]string{filepath.Join(root, "A"), filepath.Join(root, "B")}, root, when)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got.folder) != "CollectedFiles_20261031_120000" || got.copied != 3 || got.failed != 0 {
		t.Fatalf("result = %+v", got)
	}
	first, err := os.ReadFile(filepath.Join(got.folder, "one.txt"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(got.folder, "one_2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) == string(second) || (string(first) != "one" && string(first) != "two") {
		t.Fatalf("flattened = %q %q", first, second)
	}
	if _, err := collectSubfolderFiles([]string{filepath.Join(root, "A"), filepath.Join(root, "missing")}, root, when); err == nil {
		t.Fatal("mixed selection should fail")
	}
}
