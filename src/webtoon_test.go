package main

import (
	"archive/zip"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestWebtoonLayoutStacksPages(t *testing.T) {
	ys, hs, total := webtoonLayout([]float32{2, 0, 1}, 100)
	if ys[0] != 0 || hs[0] != 200 || ys[1] != 200 || hs[1] != 100*webtoonDefaultAspect || total != 200+140+100 {
		t.Fatalf("layout ys=%v hs=%v total=%v", ys, hs, total)
	}
	for y, want := range map[float32]int{0: 0, 199: 0, 200: 1, 339: 1, 340: 2, 9999: 2} {
		if got := webtoonPageAt(ys, hs, y); got != want {
			t.Fatalf("page at %v = %d, want %d", y, got, want)
		}
	}
	if webtoonPageAt(nil, nil, 5) != -1 {
		t.Fatal("no pages, no current page")
	}
}

// writeCBZPages makes an archive of n pages named p01.png...; names sort in
// reading order.
func writeCBZPages(t *testing.T, path string, n int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for i := 1; i <= n; i++ {
		w, err := zw.Create("p0" + string(rune('0'+i)) + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(pngBytes(t, 40, 80, color.White)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// waitFor polls cond, which a background scan makes true.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func browseState(t *testing.T, dir string) *appState {
	t.Helper()
	hook := func(fn func()) { fn() }
	prev := headlessUpdate.Load()
	headlessUpdate.Store(&hook)
	t.Cleanup(func() { headlessUpdate.Store(prev) })
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, Cursor: -1, SidebarWidth: sidebarDefaultWidth, FolderFirst: true}
	a.Profiles.View.AllTypes = true
	a.openFolder(dir)
	a.scanFolder(dir, a.scanSeq)
	return a
}

// TestNextImageCrossesIntoTheNextArchive reads a series split over archives:
// past the last page of one volume the arrow opens the first page of the
// next, and back from the first page lands on the previous volume's last.
func TestNextImageCrossesIntoTheNextArchive(t *testing.T) {
	isolateArchiveCache(t)
	dir, err := os.MkdirTemp("/tmp", "lwv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	writeCBZPages(t, filepath.Join(dir, "v1.cbz"), 2)
	writeCBZPages(t, filepath.Join(dir, "v2.cbz"), 3)
	v1, v2 := filepath.Join(dir, "v1.cbz"), filepath.Join(dir, "v2.cbz")

	a := browseState(t, dir)
	a.openFolder(v1)
	a.scanFolder(v1, a.scanSeq)
	a.openLargeImage(0)
	a.nextImage()
	if a.LargeIndex != 1 || a.Root != v1 {
		t.Fatalf("second page: index=%d root=%s", a.LargeIndex, a.Root)
	}
	a.nextImage() // past the last page of v1
	waitFor(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.InLargeView })
	if a.Root != v2 || !a.InLargeView || a.LargeIndex != 0 {
		t.Fatalf("after the last page: root=%s large=%v index=%d, want v2 page 0", a.Root, a.InLargeView, a.LargeIndex)
	}
	if got := a.currentItem().Name; got != "p01.png" {
		t.Fatalf("first page of the next archive = %s", got)
	}
	a.prevImage() // before the first page of v2
	waitFor(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.InLargeView })
	if a.Root != v1 || !a.InLargeView || a.LargeIndex != 1 {
		t.Fatalf("before the first page: root=%s large=%v index=%d, want v1 last page", a.Root, a.InLargeView, a.LargeIndex)
	}
	// No archive after the last: stay put and say so.
	a.openFolder(v2)
	a.scanFolder(v2, a.scanSeq)
	a.openLargeImage(2)
	a.nextImage()
	if a.Root != v2 || a.LargeIndex != 2 || !strings.Contains(a.HUD, "没有更多") {
		t.Fatalf("end of series: root=%s index=%d hud=%q", a.Root, a.LargeIndex, a.HUD)
	}
}

// TestWebtoonColumnFollowsScrolling renders the continuous column and checks
// the reading position tracks the scroll, toggling keeps the page, and the
// bar offers the switch.
func TestWebtoonColumnFollowsScrolling(t *testing.T) {
	isolateArchiveCache(t)
	dir, err := os.MkdirTemp("/tmp", "lww")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	cbz := filepath.Join(dir, "long.cbz")
	writeCBZPages(t, cbz, 5)

	a := browseState(t, dir)
	a.openFolder(cbz)
	a.scanFolder(cbz, a.scanSeq)
	for i := range a.Items { // pages are 40x80: aspect 2
		a.Items[i].InfoLoaded = true
		a.Items[i].Info = mediaInfo{Width: 40, Height: 80}
	}
	a.openLargeImage(1)
	te := ui.NewTester(a.view, 1000, 700)
	te.Frame()
	if _, ok := te.Find("纵向连续"); !ok {
		t.Fatalf("the bar must offer the continuous mode: %v", te.Texts())
	}
	if err := te.Click("纵向连续"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	te.Frame()
	if !a.webtoon() {
		t.Fatal("clicking the toggle must turn the column on")
	}
	// Page 1 starts at the top after the jump (width capped at 920, aspect 2).
	pageH := float32(webtoonMaxWidth) * 2
	if abs32(a.webtoonScroll.Y-pageH) > 2 {
		t.Fatalf("scroll = %v, want the second page's top %v", a.webtoonScroll.Y, pageH)
	}
	// Scroll into the fourth page: the current page follows.
	a.webtoonScroll.Y = 3*pageH + 10
	te.Frame()
	if a.LargeIndex != 3 {
		t.Fatalf("reading page = %d, want 3 at scroll %v", a.LargeIndex, a.webtoonScroll.Y)
	}
	// Stepping with the keys moves a whole page.
	a.prevImage()
	te.Frame()
	te.Frame()
	if abs32(a.webtoonScroll.Y-2*pageH) > 2 || a.LargeIndex != 2 {
		t.Fatalf("prev: scroll=%v index=%d, want page 2 at %v", a.webtoonScroll.Y, a.LargeIndex, 2*pageH)
	}
	a.toggleWebtoon()
	if a.webtoon() || !a.InLargeView || a.currentItem().Name != "p03.png" {
		t.Fatalf("leaving the column must keep the page: webtoon=%v item=%v", a.webtoon(), a.currentItem())
	}
}
