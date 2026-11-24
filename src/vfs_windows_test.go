//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"
)

func TestSHFileOpStructLayout(t *testing.T) {
	var op shFileOpStruct
	if got := unsafe.Sizeof(op); got != 56 {
		t.Fatalf("sizeof(shFileOpStruct) = %d, want 56 on 64-bit Windows", got)
	}
	if off := unsafe.Offsetof(op.pFrom); off != 16 {
		t.Fatalf("offsetof(pFrom) = %d, want 16", off)
	}
	if off := unsafe.Offsetof(op.fFlags); off != 32 {
		t.Fatalf("offsetof(fFlags) = %d, want 32", off)
	}
	if off := unsafe.Offsetof(op.fAnyOperationsAborted); off != 36 {
		t.Fatalf("offsetof(fAnyOperationsAborted) = %d, want 36", off)
	}
}

func TestSHFileOpFromDoubleNull(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jpg")
	b := filepath.Join(dir, "b.jpg")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	buf, absList, err := shFileOpFrom([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(absList) != 2 {
		t.Fatalf("absList = %v", absList)
	}
	if len(buf) < 2 || buf[len(buf)-1] != 0 || buf[len(buf)-2] != 0 {
		t.Fatalf("pFrom buffer not double-NUL terminated: tail=%v", buf[max(0, len(buf)-4):])
	}
	decoded := string(utf16.Decode(buf[:len(buf)-2]))
	parts := strings.Split(decoded, "\x00")
	if len(parts) != 2 || parts[0] != absList[0] || parts[1] != absList[1] {
		t.Fatalf("decoded parts = %q", parts)
	}
	if _, _, err := shFileOpFrom([]string{filepath.Join(dir, "missing.jpg")}); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestWindowsUntrashScriptEscapesQuotes(t *testing.T) {
	s := windowsUntrashScript(`C:\Pics\it's here.jpg`)
	if !strings.Contains(s, `'C:\Pics\it''s here.jpg'`) {
		t.Fatalf("single quote not escaped in script: %s", s)
	}
}

func TestWindowsDeleteInverseUsesUntrash(t *testing.T) {
	inv, ok := inverseOf(fileOperation{op: opDelete, paths: []string{`C:\Pics\a.jpg`}})
	if !ok || inv.op != opUntrash || len(inv.paths) != 1 {
		t.Fatalf("inverseOf(opDelete) = %#v ok=%v", inv, ok)
	}
	redo, ok := inverseOf(inv)
	if !ok || redo.op != opDelete || len(redo.paths) != 1 {
		t.Fatalf("inverseOf(opUntrash) = %#v ok=%v", redo, ok)
	}
}
