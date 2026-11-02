package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Shared element transitions.
//
// One media item keeps one identity, heroMatch{path}, wherever it is shown:
// the thumbnail in the grid, waterfall and justified layouts, the fitted image
// or video in the large view, and the video canvas of the editor. MyGo's
// ElementTransition.Match moves an element that appears from the window box
// the element with the same identity showed in the previous frame, so a
// switch between any two of these views morphs the media box instead of
// cutting. The framework samples the box actually shown, so reversing
// mid-flight starts from where the element is, and the spring keeps its
// velocity. Preferences.ReduceMotion shows the destination at once.
//
// Two things Match does not do are handled here:
//   - Corner radius: heroState hands the radius over, each side starting at
//     the radius the other last showed (thumbnail 8, large view 0).
//   - Paint order: a thumbnail growing back would paint under its later
//     siblings. On close a flyer in the window overlay, placed at the
//     thumbnail's last laid-out box, carries the match while the real
//     thumbnail stays hidden until the flyer lands.

// heroMatch is the shared identity of every presentation of one media item.
type heroMatch struct{ path string }

const (
	heroThumbRadius = 8
	heroNone        = 0
	heroOpening     = 1
	heroClosing     = 2
	heroRectLimit   = 512
)

// heroAnimation is a critically damped spring: no overshoot, a macOS-like
// settle, interruptible without a velocity jump.
var heroAnimation = ui.Animation{Spring: ui.SpringOptions{Duration: 420 * time.Millisecond}}

// heroState tracks the transition between the browser and the large view.
type heroState struct {
	path    string
	phase   int
	started bool
	// radius is the last corner radius the moving element showed, the start
	// value of a reversal.
	radius float32
	// thumbs are the window boxes thumbnails were laid out at, by path.
	thumbs map[string]ui.Rect
}

func heroTransition(path string) ui.ElementTransition {
	// Colors only: in-place layout changes (thumbnail size, reflow) stay
	// instant; Match still animates the box between two presentations.
	return ui.ElementTransition{Animation: &heroAnimation, Colors: true, Match: heroMatch{path}}
}

func (a *appState) heroSet(path string, phase int, radius float32) {
	thumbs := a.Hero.thumbs
	a.Hero = heroState{path: path, phase: phase, radius: radius, thumbs: thumbs}
}

// heroBeginOpen records an open from the browser. Navigation inside the
// large view does not animate the radius: no thumbnail is shown there.
func (a *appState) heroBeginOpen(path string, fromBrowser bool) {
	if !fromBrowser {
		a.heroSet("", heroNone, 0)
		return
	}
	from := float32(heroThumbRadius)
	if a.Hero.phase == heroClosing && a.Hero.path == path {
		from = a.Hero.radius
	}
	a.heroSet(path, heroOpening, from)
}

func (a *appState) heroBeginClose(path string) {
	from := float32(0)
	if a.Hero.phase == heroOpening && a.Hero.path == path {
		from = a.Hero.radius
	}
	a.heroSet(path, heroClosing, from)
}

// heroRadius returns the corner radius of el for the media item at path.
// rest is the settled radius; phase is the direction in which el is the
// destination.
func (a *appState) heroRadius(el ui.Element, path string, rest float32, phase int) float32 {
	h := &a.Hero
	if h.phase != phase || h.path != path {
		return rest
	}
	if !h.started {
		el.Jump("heroRadius", h.radius)
		h.started = true
	}
	r := el.AnimateTo("heroRadius", rest, heroAnimation)
	h.radius = r
	return r
}

// heroBox builds the shared element for path at w×h and draws content in it.
// The caller places the returned box.
func (a *appState) heroBox(c *ui.Context, key, path string, w, h, rest float32, phase int, content func()) ui.Element {
	box := ui.Box(c).Key(key).Size(w, h).Justify(ui.Center).AlignItems(ui.Center)
	if r := a.heroRadius(box, path, rest, phase); r > 0.01 {
		box.Radius(r).Clip()
	}
	box.Transition(heroTransition(path))
	box.Children(content)
	return box
}

// thumbHero styles a browser thumbnail box as the shared element.
func (a *appState) thumbHero(thumb ui.Element, path string) {
	thumb.Radius(heroThumbRadius)
	if b := thumb.LayoutBounds(); b.W > 1 && b.H > 1 {
		if a.Hero.thumbs == nil || len(a.Hero.thumbs) > heroRectLimit {
			a.Hero.thumbs = map[string]ui.Rect{}
		}
		a.Hero.thumbs[path] = b
	}
	if a.Hero.phase == heroClosing && a.Hero.path == path {
		if _, ok := a.Hero.thumbs[path]; ok {
			// The overlay flyer is landing here.
			thumb.Opacity(0)
			return
		}
	}
	thumb.Transition(heroTransition(path))
}

// heroFlyer carries a closing transition above the browser into the box the
// thumbnail was laid out at.
func (a *appState) heroFlyer(c *ui.Context) {
	h := &a.Hero
	if a.InLargeView || h.phase != heroClosing {
		return
	}
	rect, ok := h.thumbs[h.path]
	item := a.itemByPath(h.path)
	if !ok || item == nil || item.Bitmap == nil {
		h.phase = heroNone
		return
	}
	ui.Overlay(c, func() {
		fly := a.heroBox(c, "lw-hero-flyer", h.path, rect.W, rect.H, heroThumbRadius, heroClosing, func() {
			ui.Image(c, item.Bitmap).PassThrough().Fit(ui.Cover).Grow(1)
		})
		fly.Absolute().Left(rect.X).Top(rect.Y).PassThrough().Background(c.Theme().Background)
		if abs32(h.radius-heroThumbRadius) < 0.01 && !fly.Transitioning() {
			h.phase = heroNone
			c.AnimationFrame()
		}
	})
}

func (a *appState) itemByPath(path string) *mediaItem {
	for _, i := range a.Visible {
		if i >= 0 && i < len(a.Items) && a.Items[i].Path == path {
			return &a.Items[i]
		}
	}
	return nil
}

// heroFrame is the fitted size of w×h media in the stage.
func heroFrame(w, h int, stageW, stageH float32) (float32, float32, bool) {
	if w <= 0 || h <= 0 || stageW <= 1 || stageH <= 1 {
		return 0, 0, false
	}
	s := containScale(w, h, stageW, stageH)
	if s <= 0 {
		return 0, 0, false
	}
	return float32(w) * s, float32(h) * s, true
}

// mediaSize is the pixel size of an item, from its metadata or thumbnail.
func mediaSize(item *mediaItem, pic *ui.Bitmap) (int, int) {
	w, h := item.Info.Width, item.Info.Height
	if (w <= 0 || h <= 0) && pic != nil {
		w, h = pic.Size()
	}
	if (w <= 0 || h <= 0) && item.Bitmap != nil {
		w, h = item.Bitmap.Size()
	}
	return w, h
}

// largeHero draws large-view media inside the shared element box. It returns
// false when the media cannot be framed yet or is rotated, so the caller
// keeps its plain Contain presentation.
func (a *appState) largeHero(c *ui.Context, item *mediaItem, pic *ui.Bitmap, content func()) bool {
	if a.Rotation != 0 {
		return false
	}
	iw, ih := mediaSize(item, pic)
	sw, sh := a.largeSize()
	w, h, ok := heroFrame(iw, ih, sw, sh)
	if !ok {
		return false
	}
	a.heroBox(c, "lw-hero", item.Path, w, h, 0, heroOpening, content)
	if a.Hero.phase == heroOpening && a.Hero.path == item.Path && a.Hero.started && a.Hero.radius < 0.01 {
		a.Hero.phase = heroNone
	}
	return true
}
