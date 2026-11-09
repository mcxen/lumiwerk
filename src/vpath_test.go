package main

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func pngBytes(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// isolateArchiveCache keeps extracted members and prefs inside the test.
func isolateArchiveCache(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cache := t.TempDir()
	prev := archiveCacheRoot
	archiveCacheRoot = func() string { return cache }
	t.Cleanup(func() { archiveCacheRoot = prev })
	setArchivePasswords(nil)
}

// makeCBZ writes a comic archive: two pages at the root, one in a chapter
// folder that has no entry of its own, plus the junk archivers add.
func makeCBZ(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "comic.cbz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	files := map[string][]byte{
		"p01.png":          pngBytes(t, 40, 30, color.RGBA{R: 255, A: 255}),
		"p02.png":          pngBytes(t, 40, 30, color.RGBA{G: 255, A: 255}),
		"ch2/p03.png":      pngBytes(t, 20, 20, color.RGBA{B: 255, A: 255}),
		"__MACOSX/._p01":   []byte("junk"),
		"ch2/.DS_Store":    []byte("junk"),
		"notes/readme.txt": []byte("hello"),
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(files[n]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func childNames(entries []scanEntry) string {
	var out []string
	for _, e := range entries {
		n := e.Name
		if e.IsDir {
			n += "/"
		}
		out = append(out, n)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestSplitArchivePath(t *testing.T) {
	isolateArchiveCache(t)
	dir := t.TempDir()
	cbz := makeCBZ(t, dir)

	if _, _, ok := splitArchivePath(filepath.Join(dir, "missing.png")); ok {
		t.Fatal("a missing path beside real files is not an archive path")
	}
	if _, _, ok := splitArchivePath(cbz); ok {
		t.Fatal("the archive itself is a real file, not a virtual path")
	}
	archive, inner, ok := splitArchivePath(filepath.Join(cbz, "ch2", "p03.png"))
	if !ok || archive != cbz || inner != "ch2/p03.png" {
		t.Fatalf("split = %q %q %v", archive, inner, ok)
	}
	if a, in, ok := archiveLocation(cbz); !ok || a != cbz || in != "" {
		t.Fatalf("archive root location = %q %q %v", a, in, ok)
	}
	// A plain .txt file never becomes an archive.
	txt := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := splitArchivePath(filepath.Join(txt, "inner")); ok {
		t.Fatal("below a non-archive file is not an archive path")
	}
}

func TestArchiveChildrenSynthesizesFolders(t *testing.T) {
	isolateArchiveCache(t)
	cbz := makeCBZ(t, t.TempDir())

	root, err := scanDir(cbz)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := childNames(root), "ch2/,notes/,p01.png,p02.png"; got != want {
		t.Fatalf("root = %s, want %s", got, want)
	}
	sub, err := scanDir(filepath.Join(cbz, "ch2"))
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(sub); got != "p03.png" {
		t.Fatalf("ch2 = %s (junk must be hidden)", got)
	}
	for _, e := range root {
		if e.Info == nil {
			t.Fatalf("%s has no FileInfo; the scanner skips such files", e.Name)
		}
	}
	if !isArchiveDir(cbz) || !isArchiveDir(filepath.Join(cbz, "ch2")) || isArchiveDir(filepath.Join(cbz, "p01.png")) {
		t.Fatal("isArchiveDir must accept the archive and its folders only")
	}
}

func TestRealPathExtractsOnceAndTracksChanges(t *testing.T) {
	isolateArchiveCache(t)
	cbz := makeCBZ(t, t.TempDir())
	virtual := filepath.Join(cbz, "ch2", "p03.png")

	real1, err := realPath(virtual)
	if err != nil {
		t.Fatal(err)
	}
	if real1 == virtual {
		t.Fatal("a member must resolve to an extracted copy")
	}
	data, err := os.ReadFile(real1)
	if err != nil || !bytes.Equal(data, pngBytes(t, 20, 20, color.RGBA{B: 255, A: 255})) {
		t.Fatalf("extracted bytes differ: %v", err)
	}
	info1, _ := os.Stat(real1)
	real2, _ := realPath(virtual)
	info2, _ := os.Stat(real2)
	if real1 != real2 || !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatal("second lookup must reuse the cached file")
	}
	if w, h := probeImageSize(virtual); w != 20 || h != 20 {
		t.Fatalf("probe through the archive = %dx%d", w, h)
	}
	if _, err := realPath(filepath.Join(cbz, "nope.png")); err == nil {
		t.Fatal("a missing member must fail")
	}
	if real, _ := realPath("/tmp/ordinary.png"); real != "/tmp/ordinary.png" {
		t.Fatalf("ordinary paths pass through, got %q", real)
	}
}

func TestBrowseTarThroughBsdtar(t *testing.T) {
	if _, err := os.Stat("/usr/bin/bsdtar"); err != nil {
		t.Skip(err)
	}
	isolateArchiveCache(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(src, "set"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "set", "a.png"), pngBytes(t, 8, 6, color.White), 0o644); err != nil {
		t.Fatal(err)
	}
	tarball := filepath.Join(dir, "pics.tar.gz")
	if out, err := exec.Command("/usr/bin/bsdtar", "-czf", tarball, "-C", src, "set").CombinedOutput(); err != nil {
		t.Fatalf("bsdtar: %v %s", err, out)
	}
	if !isBrowsableArchive(tarball) {
		t.Fatal(".tar.gz must be browsable")
	}
	top, err := scanDir(tarball)
	if err != nil || childNames(top) != "set/" {
		t.Fatalf("top = %v err=%v", childNames(top), err)
	}
	if w, h := probeImageSize(filepath.Join(tarball, "set", "a.png")); w != 8 || h != 6 {
		t.Fatalf("probe tar member = %dx%d", w, h)
	}
}

func TestEncryptedZipMemberUsesPasswordBook(t *testing.T) {
	if _, err := os.Stat("/usr/bin/zip"); err != nil {
		t.Skip(err)
	}
	isolateArchiveCache(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "s.png"), pngBytes(t, 10, 12, color.White), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/usr/bin/zip", "-P", "pw2", "locked.zip", "s.png")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("zip: %v %s", err, out)
	}
	locked := filepath.Join(dir, "locked.zip")
	member := filepath.Join(locked, "s.png")

	if names, err := scanDir(locked); err != nil || childNames(names) != "s.png" {
		t.Fatalf("names stay listable without a password: %v %v", childNames(names), err)
	}
	if _, err := realPath(member); err == nil {
		t.Fatal("no saved password fits: the member must not open")
	}
	setArchivePasswords([]string{"wrong", "pw2"})
	if w, h := probeImageSize(member); w != 10 || h != 12 {
		t.Fatalf("with the book, member probe = %dx%d", w, h)
	}
}

func TestArchiveMembersAreReadOnly(t *testing.T) {
	isolateArchiveCache(t)
	cbz := makeCBZ(t, t.TempDir())
	for _, op := range []fileOperation{
		{op: opDelete, paths: []string{filepath.Join(cbz, "p01.png")}},
		{op: opMove, paths: []string{filepath.Join(cbz, "p01.png")}, dest: t.TempDir()},
		{op: opCopy, paths: []string{t.TempDir()}, dest: filepath.Join(cbz, "ch2")},
	} {
		if _, err := perform(op); err == nil || !strings.Contains(err.Error(), "只读") {
			t.Fatalf("op %v err = %v, want read-only refusal", op.op, err)
		}
	}
}

// TestBrowseCBZInTheWindow drives the real path: open the archive, list its
// pages, step into a chapter, open a page in the large view and go back up.
func TestBrowseCBZInTheWindow(t *testing.T) {
	isolateArchiveCache(t)
	dir, err := os.MkdirTemp("/tmp", "lwa")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	cbz := makeCBZ(t, dir)

	hook := func(fn func()) { fn() }
	prev := headlessUpdate.Load()
	headlessUpdate.Store(&hook)
	t.Cleanup(func() { headlessUpdate.Store(prev) })

	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, Cursor: -1, SidebarWidth: sidebarDefaultWidth, FolderFirst: true}
	a.Profiles.View.AllTypes = true
	a.openFolder(dir)
	a.scanFolder(dir, a.scanSeq)
	idx := -1
	for i, it := range a.Items {
		if it.Name == "comic.cbz" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("archive missing from listing: %v", a.Items)
	}
	// Opening an archive enters it instead of handing it to Finder.
	vi := -1
	for i, ix := range a.Visible {
		if ix == idx {
			vi = i
		}
	}
	a.openItem(vi)
	if a.Root != cbz {
		t.Fatalf("root = %q, want the archive %q", a.Root, cbz)
	}
	a.scanFolder(cbz, a.scanSeq)
	names := map[string]bool{}
	for _, it := range a.Items {
		names[it.Name] = true
	}
	for _, want := range []string{"p01.png", "p02.png", "ch2", "notes"} {
		if !names[want] {
			t.Fatalf("archive listing lacks %q: %v", want, names)
		}
	}
	if names["__MACOSX"] {
		t.Fatal("junk folder shown")
	}

	te := ui.NewTester(a.view, 1000, 700)
	te.Frame()
	if !te.HasText("p01.png") && !te.HasText("p02.png") {
		t.Fatalf("pages not rendered: %v", te.Texts())
	}

	a.openFolder(filepath.Join(cbz, "ch2"))
	a.scanFolder(a.Root, a.scanSeq)
	if len(a.Items) != 1 || a.Items[0].Name != "p03.png" {
		t.Fatalf("chapter items = %v", a.Items)
	}
	a.openLargeImage(0)
	te.Frame()
	if !a.InLargeView {
		t.Fatal("a page inside the archive must open in the large view")
	}
	if w, h := probeImageSize(a.Items[0].Path); w != 20 || h != 20 {
		t.Fatalf("large page size = %dx%d", w, h)
	}
}
