package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func isolatedTagStore(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", t.TempDir())
	fileTagStore = &tagStore{}
	t.Cleanup(func() { fileTagStore = &tagStore{} })
}

func TestTagStorePersistsRatingAndClears(t *testing.T) {
	isolatedTagStore(t)
	p := filepath.Join(t.TempDir(), "a.png")
	if err := fileTagStore.set(p, setRatingTags(nil, 4)); err != nil {
		t.Fatal(err)
	}
	// A fresh store instance reads it back from disk.
	fileTagStore = &tagStore{}
	got := fileTagStore.get(p)
	if ratingFor(&mediaItem{Tags: got}) != 4 {
		t.Fatalf("rating = %v", got)
	}
	if err := fileTagStore.set(p, setRatingTags(got, 0)); err != nil {
		t.Fatal(err)
	}
	if got := fileTagStore.get(p); len(got) != 0 {
		t.Fatalf("cleared tags = %v", got)
	}
}

func TestTagStoreFollowsMoveAndFolderRename(t *testing.T) {
	isolatedTagStore(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "old")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(src, "a.png")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fileTagStore.set(file, []string{"评分3"}); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "new")
	if err := movePath(src, dst); err != nil {
		t.Fatal(err)
	}
	if got := fileTagStore.get(filepath.Join(dst, "a.png")); !reflect.DeepEqual(got, []string{"评分3"}) {
		t.Fatalf("moved tags = %v", got)
	}
	if got := fileTagStore.get(file); len(got) != 0 {
		t.Fatalf("old path still tagged: %v", got)
	}
}

func TestTagStoreFollowsRename(t *testing.T) {
	isolatedTagStore(t)
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	if err := os.WriteFile(from, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fileTagStore.set(from, []string{"评分5"}); err != nil {
		t.Fatal(err)
	}
	if err := applyRenameMappings([]pathPair{{From: from, To: to}}); err != nil {
		t.Fatal(err)
	}
	if got := fileTagStore.get(to); !reflect.DeepEqual(got, []string{"评分5"}) {
		t.Fatalf("renamed tags = %v", got)
	}
}

func TestXDGTrashWritesInfoAndAvoidsCollisions(t *testing.T) {
	data := t.TempDir()
	dir := t.TempDir()
	var dests []string
	for i := 0; i < 2; i++ {
		p := filepath.Join(dir, "my file.png")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		dest, err := xdgTrash(p, data)
		if err != nil {
			t.Fatal(err)
		}
		dests = append(dests, dest)
	}
	if dests[0] == dests[1] {
		t.Fatalf("collision: %v", dests)
	}
	info, err := os.ReadFile(filepath.Join(data, "Trash", "info", "my file.png.trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(info), "Path="+strings.ReplaceAll(filepath.Join(dir, "my file.png"), " ", "%20")) {
		t.Fatalf("trashinfo = %q", info)
	}
	if _, err := os.Stat(dests[1]); err != nil {
		t.Fatal(err)
	}
}

func TestParseOpenWithRegistryOutput(t *testing.T) {
	list := "HKEY_CURRENT_USER\\...\\OpenWithList\r\n    a    REG_SZ    Photoshop.exe\r\n    b    REG_SZ    mspaint.exe\r\n    MRUList    REG_SZ    ab\r\n"
	if got := parseOpenWithList(list); !reflect.DeepEqual(got, []string{"Photoshop.exe", "mspaint.exe"}) {
		t.Fatalf("names = %v", got)
	}
	q := "HKEY_CLASSES_ROOT\\Applications\\x.exe\\shell\\open\\command\r\n    (Default)    REG_SZ    \"C:\\Program Files\\X\\x.exe\" \"%1\"\r\n"
	if got := commandExecutable(q); got != `C:\Program Files\X\x.exe` {
		t.Fatalf("exe = %q", got)
	}
	bare := "    (Default)    REG_SZ    C:\\Windows\\system32\\mspaint.exe %1\r\n"
	if got := commandExecutable(bare); got != `C:\Windows\system32\mspaint.exe` {
		t.Fatalf("exe = %q", got)
	}
}
