package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameSwapAndCase(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "A.txt")
	b := filepath.Join(dir, "B.txt")
	if err := os.WriteFile(a, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyRenameMappings([]pathPair{{From: a, To: b}, {From: b, To: a}}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(a); err != nil || string(data) != "b" {
		t.Fatalf("swap left A=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(b); err != nil || string(data) != "a" {
		t.Fatalf("swap left B=%q err=%v", data, err)
	}
	lower := filepath.Join(dir, "a.txt")
	if err := applyRenameMappings([]pathPair{{From: a, To: lower}}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == "a.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("case-only rename did not land on a.txt")
	}
}

func TestRenameRejectsOverlapAndRollsBack(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	err := applyRenameMappings([]pathPair{
		{From: parent, To: filepath.Join(dir, "p2")},
		{From: child, To: filepath.Join(dir, "c2")},
	})
	if err == nil || !strings.Contains(err.Error(), "子项目") {
		t.Fatalf("overlap err = %v", err)
	}

	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	blocker := filepath.Join(dir, "notdir")
	if err := os.WriteFile(a, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = applyRenameMappings([]pathPair{
		{From: a, To: filepath.Join(dir, "a2.txt")},
		{From: b, To: filepath.Join(blocker, "b.txt")},
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	if _, statErr := os.Stat(a); statErr != nil {
		t.Fatalf("rollback lost a: %v", statErr)
	}
	if _, statErr := os.Stat(b); statErr != nil {
		t.Fatalf("rollback lost b: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "a2.txt")); !os.IsNotExist(statErr) {
		t.Fatal("partial rename remained")
	}
}

func TestFlowUniqueAndQuickRename(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(original, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := flowUnique(original, true, nil); filepath.Base(got) != "a_copy.txt" {
		t.Fatalf("in-place unique = %s", got)
	}
	if got := flowUnique(original, false, nil); filepath.Base(got) != "a_2.txt" {
		t.Fatalf("external unique = %s", got)
	}
	second := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(second, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := quickRenameMappings([]string{original, second}, "Roll", "{folder}_{index:02}")
	if filepath.Base(got[0].To) != "Roll_01.txt" || filepath.Base(got[1].To) != "Roll_02.txt" {
		t.Fatalf("quick rename = %#v", got)
	}
}

func TestCopyDirectoryAndRejectSelf(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "folder")
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	applied, err := perform(fileOperation{op: opCopy, paths: []string{src}, dest: dst})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "folder", "nested", "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := perform(fileOperation{op: opCopy, paths: []string{src}, dest: filepath.Join(src, "nested")}); err == nil {
		t.Fatal("copy into itself should fail")
	}
	if len(applied.created) != 1 {
		t.Fatalf("created = %#v", applied.created)
	}
}

func TestWheelZoomAndSeek(t *testing.T) {
	if got := wheelZoomFactor(1, false, false); got != 1.1 {
		t.Fatalf("notch = %v", got)
	}
	if got := wheelZoomFactor(40, true, false); got != 1.1 {
		t.Fatalf("precise = %v", got)
	}
	if got := wheelZoomFactor(80, true, true); got != 1 {
		t.Fatalf("momentum = %v", got)
	}
	left, right := edgeZones(1000)
	if left != 150 || right != 850 {
		t.Fatalf("edges = %v %v", left, right)
	}
	if got := seekPreview(10, 250, 1000, 40); got != 20 {
		t.Fatalf("seek = %v", got)
	}
}

func TestReplaceAndMerge(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dstDir := filepath.Join(dir, "dst")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dstDir, "src.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasNameConflict([]string{src}, dstDir) {
		t.Fatal("expected conflict")
	}
	applied, err := perform(fileOperation{op: opCopy, paths: []string{src}, dest: dstDir, replace: true})
	if err != nil {
		t.Fatal(err)
	}
	if !applied.replaced {
		t.Fatal("expected replaced")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "new" {
		t.Fatalf("replaced content = %q err=%v", data, err)
	}
	if _, ok := inverseOf(applied); ok {
		t.Fatal("replace should not be undoable")
	}

	folder := filepath.Join(dir, "folder")
	nested := filepath.Join(dstDir, "folder")
	if err := os.MkdirAll(filepath.Join(folder, "keep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "keep", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := perform(fileOperation{op: opCopy, paths: []string{folder}, dest: dstDir, replace: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(nested, "old.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(nested, "keep", "a.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestSoundtrackArgsFollowSeek(t *testing.T) {
	args := soundtrackArgs("/tmp/a.mp4", 1.25)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-vn") || !strings.Contains(joined, "-ss 1.250") || !strings.Contains(joined, "f32le") || !strings.HasSuffix(joined, "-") {
		t.Fatalf("args = %s", joined)
	}
	if strings.Contains(strings.Join(soundtrackArgs("/tmp/a.mp4", 0), " "), "-ss") {
		t.Fatal("zero seek should not pass -ss")
	}
}
