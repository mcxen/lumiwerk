package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchiveNamesAndZipArgs(t *testing.T) {
	if got := archiveBaseName("roll.tar.gz"); got != "roll" {
		t.Fatalf("tar base = %q", got)
	}
	if got := archiveBaseName("photo.JPEG"); got != "photo" {
		t.Fatalf("photo base = %q", got)
	}
	if !isArchiveName("a.CBZ") || !isArchiveName("a.tar.bz2") || isArchiveName("a.jpg") {
		t.Fatal("archive detection mismatch")
	}

	dir := t.TempDir()
	photo := filepath.Join(dir, "photo.jpg")
	other := filepath.Join(filepath.Dir(dir), "other.jpg")
	when := time.Date(2026, 10, 31, 1, 2, 3, 0, time.Local)
	dest, err := zipDestination([]string{photo, filepath.Join(dir, "b.jpg")}, when)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dest) != "Archive_20261031_010203.zip" {
		t.Fatalf("multi dest = %s", dest)
	}
	// Items from different folders (a recursive listing) zip from the folder
	// above them, each stored under its path below it.
	mixed, err := zipDestination([]string{photo, other}, when)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(mixed) != filepath.Dir(dir) {
		t.Fatalf("mixed dest = %s, want a file in %s", mixed, filepath.Dir(dir))
	}
	mcwd, margs, err := zipArgs([]string{photo, other}, mixed, "")
	if err != nil {
		t.Fatal(err)
	}
	if mcwd != filepath.Dir(dir) || margs[len(margs)-2] != filepath.Join(filepath.Base(dir), "photo.jpg") || margs[len(margs)-1] != "other.jpg" {
		t.Fatalf("mixed zip cwd %s args %v", mcwd, margs)
	}

	if err := os.WriteFile(filepath.Join(dir, "photo.zip"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest, err = zipDestination([]string{photo}, when)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dest) != "photo_2.zip" {
		t.Fatalf("collision dest = %s", dest)
	}
	cwd, args, err := zipArgs([]string{photo}, dest, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if cwd != dir {
		t.Fatalf("cwd = %s", cwd)
	}
	joined := strings.Join(args, " ")
	want := "-r -y -P secret " + dest + " photo.jpg"
	if joined != want {
		t.Fatalf("args = %s", joined)
	}
}

func TestCompressExtractRoundTrip(t *testing.T) {
	if _, err := os.Stat("/usr/bin/zip"); err != nil {
		t.Skip(err)
	}
	if _, err := os.Stat("/usr/bin/bsdtar"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	folder := filepath.Join(dir, "roll")
	if err := os.MkdirAll(filepath.Join(folder, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "nested", "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(plain, []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}

	applied, err := perform(fileOperation{op: opCompress, paths: []string{plain}})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(applied.created[0]) != "note.zip" {
		t.Fatalf("zip = %v", applied.created)
	}
	inverse, ok := inverseOf(applied)
	if !ok || inverse.op != opDelete || inverse.paths[0] != applied.created[0] {
		t.Fatalf("inverse = %#v ok=%v", inverse, ok)
	}
	extracted, err := perform(fileOperation{op: opExtract, paths: applied.created})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(extracted.created[0], "note.txt"))
	if err != nil || string(got) != "note" {
		t.Fatalf("extracted = %q err=%v", got, err)
	}

	zipPath, err := compressPaths([]string{folder}, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(zipPath) != "roll.zip" {
		t.Fatalf("folder zip = %s", zipPath)
	}
	dest, err := extractOne(zipPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dest) != "roll_2" {
		t.Fatalf("extract collision = %s", dest)
	}
	nested, err := os.ReadFile(filepath.Join(dest, "roll", "nested", "a.txt"))
	if err != nil || string(nested) != "hello" {
		t.Fatalf("nested = %q err=%v", nested, err)
	}
}

func TestEncryptedZipPromptsAndDropSource(t *testing.T) {
	if _, err := os.Stat("/usr/bin/zip"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(src, []byte("top"), 0o644); err != nil {
		t.Fatal(err)
	}
	applied, err := perform(fileOperation{op: opCompress, paths: []string{src}, password: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	zipPath := applied.created[0]
	if !zipEncrypted(zipPath) {
		t.Fatal("expected encryption flag")
	}
	if _, err := perform(fileOperation{op: opExtract, paths: []string{zipPath}}); !errorsIsPassword(err) {
		t.Fatalf("missing password err = %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "secret")); len(matches) != 0 {
		t.Fatalf("failed extract left %v", matches)
	}
	extracted, err := perform(fileOperation{op: opExtract, paths: []string{zipPath}, password: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(extracted.created[0], "secret.txt"))
	if err != nil || string(got) != "top" {
		t.Fatalf("decrypted = %q err=%v", got, err)
	}

	extra := filepath.Join(dir, "gone.txt")
	if err := os.WriteFile(extra, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := discardPath
	discardPath = os.RemoveAll
	t.Cleanup(func() { discardPath = prev })
	dropped, err := perform(fileOperation{op: opCompress, paths: []string{extra}, dropSource: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Fatalf("source remained: %v", err)
	}
	if _, ok := inverseOf(dropped); ok {
		t.Fatal("deleting the source must not be undoable")
	}
}

func errorsIsPassword(err error) bool {
	return err != nil && strings.Contains(err.Error(), "需要密码")
}

func TestExtractTriesSavedPasswordsInOrder(t *testing.T) {
	if _, err := os.Stat("/usr/bin/zip"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(src, []byte("top"), 0o644); err != nil {
		t.Fatal(err)
	}
	applied, err := perform(fileOperation{op: opCompress, paths: []string{src}, password: "third"})
	if err != nil {
		t.Fatal(err)
	}
	zipPath := applied.created[0]

	// No saved password fits: the caller must be asked.
	_, err = perform(fileOperation{op: opExtract, paths: []string{zipPath}, passwords: []string{"one", "two"}})
	if !errorsIsPassword(err) {
		t.Fatalf("no match err = %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "secret")); len(matches) != 0 {
		t.Fatalf("failed attempts left %v", matches)
	}

	// The third saved password opens it without asking.
	got, err := perform(fileOperation{op: opExtract, paths: []string{zipPath}, passwords: []string{"one", "two", "third"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(got.created[0], "secret.txt"))
	if err != nil || string(data) != "top" {
		t.Fatalf("decrypted = %q err=%v", data, err)
	}
}

func TestZipPasswordCandidatesOrderAndDedupe(t *testing.T) {
	got := zipPasswordCandidates(prefsFile{ZipPassword: " main ", ZipPasswords: []string{"a", "main", "", "b", "a"}})
	want := []string{"main", "a", "b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}
