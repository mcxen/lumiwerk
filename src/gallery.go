package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// galleryActive reports whether the large view is drawn as the immersive
// gallery: black stage, no chrome, a thin progress bar and a thumbnail
// strip that shows near the bottom edge or with H.
func (a *appState) galleryActive() bool { return a.Gallery && a.InLargeView && !a.Editor.Open }

// toggleGallery enters the gallery at the chosen item, or leaves it.
func (a *appState) toggleGallery() {
	if a.Gallery {
		a.Gallery = false
		if a.InLargeView {
			a.closeLargeImage()
		}
		return
	}
	a.Gallery = true
	if a.InLargeView || len(a.Visible) == 0 {
		return
	}
	for i := max(a.Chosen, 0); i < len(a.Visible); i++ {
		if !a.Items[a.Visible[i]].IsDir {
			a.openLargeImage(i)
			return
		}
	}
}

func (a *appState) galleryView(c *ui.Context) {
	box := ui.Box(c).Fill().Background(ui.RGB(0, 0, 0))
	show := a.galleryPeek
	if b := box.LayoutBounds(); b.H > 1 {
		if _, y, over := box.PointerPosition(); over && y > b.Y+b.H-110 {
			show = true
		}
	}
	box.Children(func() {
		a.largeImageView(c)
		ui.Column(c).Absolute().Left(0).Right(0).Bottom(0).Children(func() {
			if show {
				a.galleryStrip(c, 56)
			}
			a.galleryProgress(c, show)
		})
	})
}

// galleryProgress is a thin bar of the position in the folder; click or
// drag along it to jump. The count shows only while the strip is up.
func (a *appState) galleryProgress(c *ui.Context, label bool) {
	t := c.Theme()
	n := len(a.Visible)
	if n == 0 {
		return
	}
	frac := float32(a.LargeIndex+1) / float32(n)
	row := ui.Row(c).FillWidth().Height(14).AlignItems(ui.Center).Gap(8).Padding(0, 8)
	row.Children(func() {
		track := ui.Row(c).Grow(1).Height(3).Clip().Radius(2).Background(ui.RGBA(255, 255, 255, 0.18))
		if b := track.LayoutBounds(); b.W > 1 && track.Pressed() {
			if x, _, _ := track.PointerPosition(); true {
				i := min(n-1, max(0, int((x-b.X)/b.W*float32(n))))
				if i != a.LargeIndex && !a.Items[a.Visible[i]].IsDir {
					a.openLargeImage(i)
				}
			}
		}
		track.Children(func() {
			ui.Box(c).Width(0).FillHeight().Radius(2).Background(t.Accent).Grow(frac).Shrink(0)
			ui.Box(c).Grow(1 - frac)
		})
		if label {
			ui.Text(c, fmt.Sprintf("%d / %d", a.LargeIndex+1, n)).FontSize(10).TextColor(ui.RGBA(255, 255, 255, 0.7)).SingleLine()
		}
	})
}

// galleryStrip draws only the thumbnails that fit, centred on the current
// item; no scroll container, so off-screen items never load.
func (a *appState) galleryStrip(c *ui.Context, size float32) {
	t := c.Theme()
	const gap, pad = 4, 4
	strip := ui.Row(c).Height(size + pad*2).FillWidth().Clip().Justify(ui.Center).Gap(gap).Padding(pad).
		Background(ui.RGBA(0, 0, 0, 0.6))
	span := float32(1200)
	if b := strip.LayoutBounds(); b.W > 1 {
		span = b.W
	}
	n := int((span-pad*2)/(size+gap)) + 1
	first := max(0, min(a.LargeIndex-n/2, len(a.Visible)-n))
	last := min(len(a.Visible), first+n)
	root := a.Root
	strip.Children(func() {
		for i := first; i < last; i++ {
			item := a.visibleItem(i)
			if item == nil {
				continue
			}
			cell := ui.Box(c).Key(a.itemKey(i)).Width(size).Height(size).Shrink(0).Clip().Radius(6).
				Background(t.Surface).Justify(ui.Center).AlignItems(ui.Center)
			if i == a.LargeIndex {
				cell.Border(2, t.Accent)
			} else {
				cell.Opacity(0.7)
			}
			cell.Children(func() {
				paintMediaThumb(c, item, size, ui.Cover)
				if item.IsVideo {
					ui.Icon(c, lucidePlay).Absolute().Right(3).Bottom(3).FontSize(12).TextColor(ui.RGB(255, 255, 255))
				}
			})
			if cell.Clicked() && !item.IsDir {
				a.openLargeImage(i)
			}
			if !item.Decoded && !item.Failed && !item.Loading {
				a.requestThumb(root, a.Visible[i])
			}
		}
	})
}

// galleryIcon is lucide's gallery-vertical-end: a large frame over a strip.
var galleryIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="14" x="3" y="2" rx="2"/><path d="M3 21h2"/><path d="M9 21h2"/><path d="M15 21h2"/><path d="M21 21h0"/></svg>`))
