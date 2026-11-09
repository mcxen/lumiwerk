package main

// Vertical continuous reading ("webtoon" mode) and stepping between archives.
//
// In webtoon mode the large view shows every picture of the folder or archive
// as one tall column, fitted to the width, scrolled with the wheel. Like the
// waterfall layout it builds only the pages near the window: a fixed-height
// spacer gives the scrollbar its range and each visible page is an absolutely
// positioned box. LargeIndex follows the page under the reading line, so the
// title, the counter and closing the view all stay in step.

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	// webtoonMaxWidth keeps lines of art readable on a wide window.
	webtoonMaxWidth = 920
	// webtoonDefaultAspect (height / width) stands in for a page whose size
	// is not known yet; the layout corrects itself as sizes arrive.
	webtoonDefaultAspect = 1.4
	// webtoonKeep is how many screens of decoded pages stay in memory on
	// each side of the window.
	webtoonKeep = 3
	// webtoonReadLine is where in the window (from the top) a page counts
	// as "the current one".
	webtoonReadLine = 0.4
)

func (a *appState) webtoon() bool { return a.Profiles.Webtoon }

func (a *appState) toggleWebtoon() {
	a.Profiles.Webtoon = !a.Profiles.Webtoon
	savePrefs(a.Profiles)
	a.dropLargeBitmaps("")
	if a.Profiles.Webtoon {
		a.webtoonJump(a.LargeIndex)
		a.HUD = "纵向连续阅读"
	} else {
		a.HUD = "单页查看"
		if a.LargeIndex >= 0 {
			a.openLargeImage(a.LargeIndex)
		}
	}
	a.HUDUntil = time.Now().Add(1200 * time.Millisecond)
}

// webtoonJump scrolls the column so visible index i starts at the top.
func (a *appState) webtoonJump(i int) {
	a.webtoonJumpTo = i
	a.webtoonJumping = true
}

// webtoonLayout stacks pages of the given aspect ratios (height / width) at
// the given width. It returns each page's top and height and the total.
func webtoonLayout(aspects []float32, width float32) (ys, hs []float32, total float32) {
	ys = make([]float32, len(aspects))
	hs = make([]float32, len(aspects))
	for i, ar := range aspects {
		if ar <= 0 {
			ar = webtoonDefaultAspect
		}
		ys[i] = total
		hs[i] = width * ar
		total += hs[i]
	}
	return ys, hs, total
}

// webtoonPageAt returns the page covering y, or the last page when y is past
// the end. ys must be ascending.
func webtoonPageAt(ys, hs []float32, y float32) int {
	if len(ys) == 0 {
		return -1
	}
	i, _ := slices.BinarySearchFunc(ys, y, func(top, target float32) int {
		switch {
		case top < target:
			return -1
		case top > target:
			return 1
		}
		return 0
	})
	// BinarySearch returns the insertion point: the page that starts at or
	// after y. The page covering y is the one before, unless it starts at y.
	if i < len(ys) && ys[i] == y {
		return i
	}
	if i > 0 {
		i--
	}
	return i
}

type webtoonPageRef struct {
	visible int // index into a.Visible
	item    int // index into a.Items
}

// webtoonPages lists the viewable pictures: every visible entry that is a
// plain image (folders and videos do not belong in the column).
func (a *appState) webtoonPages() []webtoonPageRef {
	pages := make([]webtoonPageRef, 0, len(a.Visible))
	for vi, ii := range a.Visible {
		if ii < 0 || ii >= len(a.Items) {
			continue
		}
		it := &a.Items[ii]
		if it.IsDir || it.IsVideo {
			continue
		}
		pages = append(pages, webtoonPageRef{visible: vi, item: ii})
	}
	return pages
}

func (a *appState) webtoonAspect(it *mediaItem) float32 {
	if it.InfoLoaded && it.Info.Width > 0 && it.Info.Height > 0 {
		return float32(it.Info.Height) / float32(it.Info.Width)
	}
	if it.Large != nil {
		if w, h := it.Large.Size(); w > 0 && h > 0 {
			return float32(h) / float32(w)
		}
	}
	return webtoonDefaultAspect
}

// webtoonView is the scrolling column. It replaces the single-picture stage.
func (a *appState) webtoonView(c *ui.Context) {
	t := c.Theme()
	scroll := ui.Scroll(c).Fill().TrackScroll(&a.webtoonScroll)
	vw, vh := a.largeSize()
	if b := scroll.LayoutBounds(); b.W > 1 && b.H > 1 {
		vw, vh = b.W, b.H
		a.StageW, a.StageH = b.W, b.H
	}
	width := min(vw, webtoonMaxWidth)
	left := (vw - width) / 2

	pages := a.webtoonPages()
	aspects := make([]float32, len(pages))
	for i, p := range pages {
		aspects[i] = a.webtoonAspect(&a.Items[p.item])
	}
	ys, hs, total := webtoonLayout(aspects, width)

	if a.webtoonJumping {
		a.webtoonJumping = false
		for i, p := range pages {
			if p.visible == a.webtoonJumpTo {
				a.webtoonScroll.Y = ys[i]
				break
			}
		}
	}
	y := a.webtoonScroll.Y
	if p := webtoonPageAt(ys, hs, y+vh*webtoonReadLine); p >= 0 && pages[p].visible != a.LargeIndex {
		a.LargeIndex = pages[p].visible
		a.Zoom, a.Rotation, a.Mirror = 1, 0, false
		a.setTitle(fmt.Sprintf("%s (%d/%d)", a.Items[pages[p].item].Name, p+1, len(pages)))
	}

	top, bottom := y-vh*0.5, y+vh*1.5
	keepTop, keepBottom := y-vh*webtoonKeep, y+vh*(1+webtoonKeep)
	edge := largeStageEdge(width)
	scroll.Children(func() {
		ui.Box(c).Width(vw).Height(max(total, vh))
		for i, p := range pages {
			it := &a.Items[p.item]
			inWindow := ys[i]+hs[i] >= top && ys[i] <= bottom
			if !inWindow {
				if it.Large != nil && (ys[i]+hs[i] < keepTop || ys[i] > keepBottom) {
					it.Large = nil
					it.LargeLoading = false
				}
				continue
			}
			if !it.InfoLoaded {
				a.requestInfo(p.item)
			}
			if !it.Decoded && !it.Failed && !it.Loading {
				a.loadThumb(a.Root, p.item)
			}
			a.ensureWebtoonPage(p.item, edge)
			page := ui.Box(c).Absolute().Left(left).Top(ys[i]).Width(width).Height(hs[i]).Background(t.Surface)
			page.Children(func() {
				pic := it.Large
				if pic == nil {
					pic = it.Bitmap
				}
				if pic != nil {
					ui.Image(c, pic).Size(width, hs[i]).Fit(ui.FillBox)
				}
			})
		}
	})
}

// ensureWebtoonPage decodes one page's original at the column width. It is
// ensureNeighborLarge for a page the column shows rather than a neighbor.
func (a *appState) ensureWebtoonPage(itemIndex, edge int) {
	if a.isShelved() || a.Window == nil || itemIndex < 0 || itemIndex >= len(a.Items) {
		return
	}
	item := &a.Items[itemIndex]
	if item.IsVideo || item.IsDir || item.Large != nil || item.LargeLoading {
		return
	}
	item.LargeLoading = true
	path := item.Path
	root := a.Root
	safeGo("webtoonPage", func() {
		bmp, err := decodeLargeBitmap(path, edge)
		a.update(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.Root != root || itemIndex >= len(a.Items) || a.Items[itemIndex].Path != path {
				return
			}
			current := &a.Items[itemIndex]
			current.LargeLoading = false
			if err == nil && a.InLargeView && a.Profiles.Webtoon {
				current.Large = bmp
			}
		})
	})
}

// ---- stepping between archives --------------------------------------------

// siblingArchive returns the archive next to archive in its folder, dir steps
// away (+1 next, -1 previous) in the browser's current sort order, or "".
func (a *appState) siblingArchive(archive string, dir int) string {
	parent := filepath.Dir(archive)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}
	hidden := a.Profiles.View.Hidden
	var list []mediaItem
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !isBrowsableArchive(name) || (!hidden && len(name) > 0 && name[0] == '.') {
			continue
		}
		it := mediaItem{Path: filepath.Join(parent, name), Name: name, Ext: extOf(name)}
		if info, err := e.Info(); err == nil {
			it.Size, it.ModTime = info.Size(), info.ModTime().Unix()
		}
		list = append(list, it)
	}
	slices.SortStableFunc(list, func(x, y mediaItem) int { return a.compareMedia(x, y) })
	for i, it := range list {
		if it.Path == archive {
			if j := i + dir; j >= 0 && j < len(list) {
				return list[j].Path
			}
			return ""
		}
	}
	return ""
}

// stepArchive moves from the archive being read to its neighbor and opens its
// first (dir > 0) or last (dir < 0) picture. It reports whether it moved.
func (a *appState) stepArchive(dir int) bool {
	archive, _, ok := archiveLocation(a.Root)
	if !ok {
		return false
	}
	next := a.siblingArchive(archive, dir)
	if next == "" {
		a.HUD = "没有更多压缩包了"
		a.HUDUntil = time.Now().Add(1200 * time.Millisecond)
		return false
	}
	a.openFolder(next)
	a.pendingLargeEnd = dir
	return true
}

// openLargeAtEnd opens the first or last picture of the folder just scanned,
// the second half of stepArchive.
func (a *appState) openLargeAtEnd(dir int) {
	pick := -1
	for i, ii := range a.Visible {
		if ii < 0 || ii >= len(a.Items) || a.Items[ii].IsDir {
			continue
		}
		pick = i
		if dir > 0 {
			break
		}
	}
	if pick < 0 {
		return
	}
	a.openLargeImage(pick)
	a.HUD = pathName(a.Root)
	a.HUDUntil = time.Now().Add(1500 * time.Millisecond)
}
