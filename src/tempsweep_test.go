package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sweepTempLeftovers removes lumiwerk-* temporaries older than an hour from
// the temp directory, files and directories both, leaving anything younger
// and anything another app owns alone.
func TestSweepTempLeftovers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	// The sweep reads the temp directory through the osTempDir variable so
	// tests can point it at their own directory.
	origTempDir := osTempDir
	t.Cleanup(func() { osTempDir = origTempDir })
	osTempDir = func() string { return dir }

	old := filepath.Join(dir, "lumiwerk-thumb-old.jpg")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	new := filepath.Join(dir, "lumiwerk-thumb-new.jpg")
	if err := os.WriteFile(new, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other-app.tmp")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldDir := filepath.Join(dir, "lumiwerk-sharp-old")
	if err := os.Mkdir(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldDir, past, past); err != nil {
		t.Fatal(err)
	}

	sweepTempLeftovers()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old temp survived: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old temp dir survived: %v", err)
	}
	if _, err := os.Stat(new); err != nil {
		t.Fatalf("fresh temp was swept: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("another app's file was swept: %v", err)
	}
}
