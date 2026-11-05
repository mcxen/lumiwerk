package main

import (
	"os"
	"strconv"

	"github.com/egoist/mygo/ui"
)

// galleryVariant picks the gallery layout: 1 bottom filmstrip, 2 side
// column, 3 immersive. LUMIWERK_GALLERY overrides; default 1.
func galleryVariant() int {
	if v, err := strconv.Atoi(os.Getenv("LUMIWERK_GALLERY")); err == nil && v >= 1 && v <= 3 {
		return v
	}
	return 1
}

var galleryMode = galleryVariant()

// galleryActive reports whether the large view is drawn as a gallery.
func (a *appState) galleryActive() bool { return a.Gallery && a.InLargeView && !a.Editor.Open }

// galleryImmersive hides tabs, toolbar and status bar (variant 3).
func (a *appState) galleryImmersive() bool { return a.galleryActive() && galleryMode == 3 }

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
	at := max(a.Chosen, 0)
	for i := at; i < len(a.Visible); i++ {
		if !a.Items[a.Visible[i]].IsDir {
			a.openLargeImage(i)
			return
		}
	}
}

func (a *appState) galleryView(c *ui.Context) {
	switch galleryMode {
	case 2:
		ui.Row(c).Fill().Children(func() {
			a.galleryStrip(c, true, 96)
			ui.Box(c).Width(1).FillHeight().Background(c.Theme().Border)
			ui.Box(c).Grow(1).FillHeight().Children(func() { a.largeImageView(c) })
		})
	case 3:
		box := ui.Box(c).Fill().Background(ui.RGB(0, 0, 0))
		show := a.galleryPeek
		if b := box.LayoutBounds(); b.H > 1 {
			if x, y, over := box.PointerPosition(); over {
				_ = x
				show = show || y > b.Y+b.H-110
			}
		}
		box.Children(func() {
			a.largeImageView(c)
			if show {
				ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Background(ui.RGBA(0, 0, 0, 0.7)).
					Children(func() { a.galleryStrip(c, false, 72) })
			}
		})
	default:
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).FillWidth().Grow(1).Children(func() { a.largeImageView(c) })
			ui.Divider(c)
			a.galleryStrip(c, false, 56)
		})
	}
}

// galleryStrip draws only the thumbnails that fit, centred on the current
// item; no scroll container, so off-screen items never load.
func (a *appState) galleryStrip(c *ui.Context, vertical bool, size float32) {
	t := c.Theme()
	const gap, pad = 4, 4
	var strip ui.Element
	if vertical {
		strip = ui.Column(c).Width(size + pad*2).FillHeight().Clip()
	} else {
		strip = ui.Row(c).Height(size + pad*2).FillWidth().Clip().Justify(ui.Center)
	}
	strip.Gap(gap).Padding(pad)
	span := float32(1200)
	if b := strip.LayoutBounds(); b.W > 1 {
		span = b.W
		if vertical {
			span = b.H
		}
	}
	n := int((span-pad*2)/(size+gap)) + 1
	first := a.LargeIndex - n/2
	first = max(0, min(first, len(a.Visible)-n))
	last := min(len(a.Visible), first+n)
	root := a.Root
	strip.Children(func() {
		for i := first; i < last; i++ {
			item := a.visibleItem(i)
			if item == nil {
				continue
			}
			cell := ui.Box(c).Key(a.itemKey(i)).Width(size).Height(size).Shrink(0).Clip().Radius(3).
				Background(t.Surface).Justify(ui.Center).AlignItems(ui.Center)
			if i == a.LargeIndex {
				cell.Border(2, t.Accent)
			} else {
				cell.Opacity(0.75)
			}
			cell.Children(func() { paintMediaThumb(c, item, size, ui.Cover) })
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
