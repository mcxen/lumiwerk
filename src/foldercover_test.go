package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

func solidImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	return img
}

func TestCoverLayersStayPut(t *testing.T) {
	for n := 1; n <= 4; n++ {
		a := coverLayers(n)
		b := coverLayers(n)
		if len(a) != n {
			t.Fatalf("n=%d has %d layers", n, len(a))
		}
		if fmt.Sprintf("%+v", a) != fmt.Sprintf("%+v", b) {
			t.Fatal("fan changed between calls")
		}
		if a[n-1].angle != 0 || a[n-1].opacity != 1 {
			t.Fatalf("front = %+v", a[n-1])
		}
		for i := 1; i < n; i++ {
			if a[i].scale <= a[i-1].scale {
				t.Fatalf("n=%d scales not increasing: %+v", n, a)
			}
			if a[i].opacity <= a[i-1].opacity {
				t.Fatalf("n=%d opacity not increasing: %+v", n, a)
			}
		}
		for i := 0; i < n-1; i++ {
			deg := math.Abs(a[i].angle)
			if deg < 3 || deg > 6 {
				t.Fatalf("behind angle %v", a[i].angle)
			}
		}
		if math.Abs(a[n-1].angle) != 0 {
			t.Fatal(a[n-1].angle)
		}
	}
}

func TestFolderCoverKeyTracksContent(t *testing.T) {
	a := []folderCandidate{{rel: "a.jpg", mtime: 10}, {rel: "b.png", mtime: 20}}
	b := []folderCandidate{{rel: "a.jpg", mtime: 10}, {rel: "b.png", mtime: 21}}
	c := []folderCandidate{{rel: "a.jpg", mtime: 10}, {rel: "c.png", mtime: 20}}
	if folderCoverDigest(a) != folderCoverDigest(append([]folderCandidate(nil), a...)) {
		t.Fatal("digest moved")
	}
	if folderCoverDigest(a) == folderCoverDigest(b) || folderCoverDigest(a) == folderCoverDigest(c) {
		t.Fatal("digest ignored a change")
	}
	key := folderCoverKey("/pics/roll", folderCoverDigest(a), 600)
	if !strings.Contains(key, "/pics/roll") || !strings.Contains(key, folderCoverDigest(a)) || !strings.Contains(key, "#cover#") {
		t.Fatalf("key = %s", key)
	}
	if folderCoverKey("/pics/roll", folderCoverDigest(a), 600) == folderCoverKey("/pics/roll", folderCoverDigest(a), 900) {
		t.Fatal("size band missing from the key")
	}
}

func TestListFolderCoverCandidates(t *testing.T) {
	root := t.TempDir()
	writeSolidPNG(t, filepath.Join(root, "b.JPG"), color.RGBA{1, 2, 3, 255})
	writeSolidPNG(t, filepath.Join(root, "a.png"), color.RGBA{4, 5, 6, 255})
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSolidPNG(t, filepath.Join(root, ".hidden.png"), color.RGBA{7, 8, 9, 255})
	sub := filepath.Join(root, "album")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSolidPNG(t, filepath.Join(sub, "c.png"), color.RGBA{9, 9, 9, 255})
	deep := filepath.Join(sub, "nested")
	if err := os.Mkdir(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSolidPNG(t, filepath.Join(deep, "skip.png"), color.RGBA{1, 1, 1, 255})

	got, err := listFolderCoverCandidates(root, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, c := range got {
		rels = append(rels, c.rel)
	}
	if strings.Join(rels, ",") != "a.png,b.JPG,album/c.png" {
		t.Fatalf("order = %v", rels)
	}
	if got[1].video {
		t.Fatal("jpg marked as video")
	}
	hidden, err := listFolderCoverCandidates(root, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) == 0 || hidden[0].rel != ".hidden.png" {
		t.Fatalf("hidden = %+v", hidden)
	}
	if _, err := listFolderCoverCandidates(filepath.Join(root, "a.png"), false, nil); err == nil {
		t.Fatal("a file scanned as a folder")
	}
	empty := t.TempDir()
	none, err := listFolderCoverCandidates(empty, false, nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty = %v %v", none, err)
	}
}

func TestComposeFolderCoverStack(t *testing.T) {
	red := color.RGBA{210, 30, 30, 255}
	green := color.RGBA{20, 180, 40, 255}
	blue := color.RGBA{30, 60, 210, 255}
	yellow := color.RGBA{220, 190, 20, 255}
	one := composeFolderCover([]coverSource{{img: solidImage(80, 50, green)}}, 256)
	if one == nil || one.Bounds().Dx() != 256 {
		t.Fatal("single cover missing")
	}
	assertCornersClear(t, one)
	cx, cy := 128, 128
	if c := one.RGBAAt(cx, cy); !nearColor(c, green, 8) || c.A < 250 {
		t.Fatalf("single center = %+v", c)
	}
	if !hasFolderBlue(one) {
		t.Fatal("single image has no folder badge")
	}
	two := composeFolderCover([]coverSource{
		{img: solidImage(90, 60, green)},
		{img: solidImage(70, 90, red)},
	}, 256)
	assertCornersClear(t, two)
	if c := two.RGBAAt(cx, cy); !nearColor(c, green, 12) {
		t.Fatalf("front is not on top: %+v", c)
	}
	if !hasRedPeek(two) {
		t.Fatal("back card does not peek out")
	}
	four := composeFolderCover([]coverSource{
		{img: solidImage(100, 70, green)},
		{img: solidImage(60, 90, yellow)},
		{img: solidImage(80, 80, blue)},
		{img: solidImage(70, 50, red)},
	}, 512)
	assertCornersClear(t, four)
	if c := four.RGBAAt(256, 256); !nearColor(c, green, 12) {
		t.Fatalf("four front = %+v", c)
	}
	again := composeFolderCover([]coverSource{
		{img: solidImage(100, 70, green)},
		{img: solidImage(60, 90, yellow)},
		{img: solidImage(80, 80, blue)},
		{img: solidImage(70, 50, red)},
	}, 512)
	if !bytes.Equal(four.Pix, again.Pix) {
		t.Fatal("stack changed between draws")
	}
	if composeFolderCover(nil, 128) != nil {
		t.Fatal("empty stack produced an image")
	}
}

func TestFolderCoverLoadAndCache(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	pics := filepath.Join(root, "pics")
	if err := os.Mkdir(pics, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pics, "note.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSolidPNG(t, filepath.Join(pics, "a.png"), color.RGBA{20, 180, 40, 255})
	sub := filepath.Join(pics, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSolidPNG(t, filepath.Join(sub, "b.png"), color.RGBA{210, 30, 30, 255})
	bad := filepath.Join(root, "bad")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "x.png"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	hook := func(fn func()) { fn() }
	prev := headlessUpdate.Load()
	headlessUpdate.Store(&hook)
	t.Cleanup(func() { headlessUpdate.Store(prev) })

	a := &appState{
		Root:  root,
		Thumb: 128,
		Items: []mediaItem{
			{Path: empty, Name: "empty", IsDir: true},
			{Path: pics, Name: "pics", IsDir: true},
			{Path: bad, Name: "bad", IsDir: true},
		},
	}
	a.loadFolderCover(root, 0)
	waitCover(t, a, 0, false)
	if a.Items[0].Bitmap != nil || a.Items[0].CoverKey != "" {
		t.Fatal("empty folder grew a cover")
	}
	a.loadFolderCover(root, 2)
	waitCover(t, a, 2, false)
	if a.Items[2].Bitmap != nil {
		t.Fatal("undecodable folder grew a cover")
	}

	a.loadFolderCover(root, 1)
	waitCover(t, a, 1, true)
	key := a.Items[1].CoverKey
	if !strings.Contains(key, pics) || a.Items[1].Bitmap == nil {
		t.Fatalf("cover key %q bitmap %v", key, a.Items[1].Bitmap != nil)
	}
	if !thumbLRU.Has(key) {
		t.Fatal("cover was not stored in the thumbnail cache")
	}
	t.Cleanup(func() { thumbLRU.Delete(key) })

	// A second claim for the same folder does not start another task.
	a.Items[1].Decoded = false
	a.Items[1].Loading = true
	before := a.Items[1].LoadGen
	a.loadFolderCover(root, 1)
	if a.Items[1].LoadGen != before {
		t.Fatal("started a second cover while one was marked loading")
	}
}

func TestFolderCoverStopTracksTheViewport(t *testing.T) {
	hook := func(fn func()) { fn() }
	prev := headlessUpdate.Load()
	headlessUpdate.Store(&hook)
	t.Cleanup(func() { headlessUpdate.Store(prev) })

	now := time.Now()
	a := &appState{Items: []mediaItem{{
		Path: "/d", IsDir: true, LoadGen: 2, Loading: true, Seen: now.UnixNano(),
	}}}
	atomic.StoreInt64(&a.frameNow, now.Add(time.Millisecond).UnixNano())
	if a.folderCoverStop(0, "/d", 2) {
		t.Fatal("a card built this frame was abandoned")
	}
	if !a.Items[0].Loading {
		t.Fatal("visible claim was cleared")
	}

	a.Items[0].Seen = now.Add(-time.Second).UnixNano()
	atomic.StoreInt64(&a.frameNow, now.UnixNano())
	if !a.folderCoverStop(0, "/d", 2) {
		t.Fatal("a card off screen kept its claim")
	}
	if a.Items[0].Loading {
		t.Fatal("abandoned claim stayed loading")
	}
}

func TestShelvedSkipsFolderCover(t *testing.T) {
	a := &appState{
		shelved: true,
		Items:   []mediaItem{{Path: t.TempDir(), Name: "d", IsDir: true}},
	}
	a.loadFolderCover("/tmp", 0)
	if a.Items[0].Loading || a.Items[0].LoadGen != 0 {
		t.Fatalf("shelved window started a cover: loading=%v gen=%d", a.Items[0].Loading, a.Items[0].LoadGen)
	}
}

func TestSweepAndZoomOfFolderCover(t *testing.T) {
	bm := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))
	key := folderCoverKey("/tmp/folder", "abc", 384)
	thumbLRU.Put(key, bm, 384)
	t.Cleanup(func() { thumbLRU.Delete(key) })
	now := time.Now()
	old := now.Add(-time.Minute).UnixNano()
	a := &appState{Items: []mediaItem{{
		Path: "/tmp/folder", IsDir: true, Bitmap: bm, Decoded: true, ThumbEdge: 384, CoverKey: key, Seen: old,
	}}}
	if got := a.sweepThumbsAt(now); got != 0 || a.Items[0].Bitmap == nil {
		t.Fatalf("cached cover released, n=%d", got)
	}
	thumbLRU.Delete(key)
	if got := a.sweepThumbsAt(now); got != 1 || a.Items[0].Bitmap != nil || a.Items[0].Decoded || a.Items[0].CoverKey != "" {
		t.Fatalf("evicted cover kept: n=%d %+v", got, a.Items[0])
	}

	empty := mediaItem{Path: "/e", IsDir: true, Decoded: true}
	coarse := mediaItem{Path: "/c", IsDir: true, Decoded: true, Bitmap: bm, ThumbEdge: 384}
	sharp := mediaItem{Path: "/s", IsDir: true, Decoded: true, Bitmap: bm, ThumbEdge: 1024}
	z := &appState{Thumb: 512, Items: []mediaItem{empty, coarse, sharp}, thumbDue: time.Unix(1, 0)}
	if !z.applyThumbSettle(z.thumbDue) {
		t.Fatal("settle did not run")
	}
	if !z.Items[0].Decoded || z.Items[1].Decoded || !z.Items[2].Decoded || z.Items[1].Bitmap == nil {
		t.Fatalf("settle = decoded %v %v %v", z.Items[0].Decoded, z.Items[1].Decoded, z.Items[2].Decoded)
	}
}

func waitCover(t *testing.T, a *appState, index int, wantBitmap bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		decoded := a.Items[index].Decoded
		loading := a.Items[index].Loading
		has := a.Items[index].Bitmap != nil
		a.mu.Unlock()
		if decoded && !loading && has == wantBitmap {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("cover %d did not settle (want bitmap %v)", index, wantBitmap)
}

func writeSolidPNG(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	if c.A == 0 {
		c = color.RGBA{10, 20, 30, 255}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, solidImage(24, 16, c)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertCornersClear(t *testing.T, img *image.RGBA) {
	t.Helper()
	b := img.Bounds()
	pts := []image.Point{
		{b.Min.X, b.Min.Y},
		{b.Max.X - 1, b.Min.Y},
		{b.Min.X, b.Max.Y - 1},
		{b.Max.X - 1, b.Max.Y - 1},
	}
	for _, p := range pts {
		if a := img.RGBAAt(p.X, p.Y).A; a != 0 {
			t.Fatalf("corner %v alpha %d", p, a)
		}
	}
}

func nearColor(c color.RGBA, want color.RGBA, tol int) bool {
	return absInt(int(c.R)-int(want.R)) <= tol &&
		absInt(int(c.G)-int(want.G)) <= tol &&
		absInt(int(c.B)-int(want.B)) <= tol
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func hasFolderBlue(img *image.RGBA) bool {
	b := img.Bounds()
	for y := b.Dy() * 3 / 4; y < b.Dy(); y++ {
		for x := b.Dx() * 3 / 4; x < b.Dx(); x++ {
			c := img.RGBAAt(x, y)
			if c.B > 160 && c.B > c.R+40 && c.A > 200 {
				return true
			}
		}
	}
	return false
}

func hasRedPeek(img *image.RGBA) bool {
	b := img.Bounds()
	n := 0
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := img.RGBAAt(x, y)
			if c.R > 160 && c.G < 80 && c.A > 200 {
				n++
			}
		}
	}
	return n > 30
}
