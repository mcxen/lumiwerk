package main

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

// folderCoverGrace is how long a stack keeps decoding after its card was
// last built. One missed frame is not "scrolled away": thumbAbandoned
// treats any older frame as gone, and a check between frames would cancel
// a cover that is still on screen.
const folderCoverGrace = 200 * time.Millisecond

const (
	// folderCoverMaxLayers is how many prints sit in one stack.
	folderCoverMaxLayers = 4
	// folderCoverDigestN is how many leading media names and mtimes the
	// cache key hashes. The stack is built from the first of these that
	// decode, so an edit to that prefix misses the cache.
	folderCoverDigestN = 8
	// folderCoverDepth walks the folder and one level of subfolders, so a
	// directory of albums still gets a cover without scanning the whole tree.
	folderCoverDepth = 2
	// folderCoverVisitMax bounds a sparse tree that has almost no media.
	folderCoverVisitMax = 500
)

// errCoverStopped means the folder left the screen or the claim was
// superseded. Callers leave the plain icon in place and do not cache.
var errCoverStopped = errors.New("folder cover stopped")

// folderCandidate is one media file inside a folder, in stable name order.
type folderCandidate struct {
	path  string
	rel   string
	mtime int64
	video bool
}

// coverSource is one decoded print. The first entry is the front of the stack.
type coverSource struct {
	img   image.Image
	video bool
}

// coverLayer is one card, back to front. Angles are constants: a repaint
// must not pick a new fan.
type coverLayer struct {
	angle   float64
	scale   float64
	opacity float64
	dx, dy  float64
}

// requestThumb starts a file thumbnail or a folder stack. The view calls
// it only for an item that is not already decoded, failed or loading.
func (a *appState) requestThumb(root string, index int) {
	if a.windowHidden {
		return
	}
	if index < 0 || index >= len(a.Items) {
		return
	}
	if a.Items[index].IsDir {
		a.loadFolderCover(root, index)
		return
	}
	a.loadThumb(root, index)
}

// loadFolderCover builds a stacked preview of the folder on a background
// goroutine. One folder has one in-flight task. No readable media, a
// failed scan or a permission error keeps the plain folder icon and does
// not surface a dialog.
func (a *appState) loadFolderCover(root string, index int) {
	a.mu.Lock()
	if a.shelved || index < 0 || index >= len(a.Items) {
		a.mu.Unlock()
		return
	}
	item := &a.Items[index]
	if !item.IsDir || item.Decoded || item.Failed || item.Loading {
		a.mu.Unlock()
		return
	}
	path := item.Path
	busy := folderCoverBusyKey(path)
	if !thumbLRU.Busy(busy) {
		a.mu.Unlock()
		return
	}
	item.LoadGen++
	gen := item.LoadGen
	item.Loading = true
	edge := thumbBand(thumbEdge(a.Thumb))
	showHidden := a.Profiles.View.Hidden
	a.mu.Unlock()

	safeGo("folderCover", func() {
		defer thumbLRU.Done(busy)
		stop := func() bool { return a.folderCoverStop(index, path, gen) }
		giveUp := func() {
			if a.thumbClaimStale(index, path, gen) {
				a.clearThumbClaim(index, path, gen)
			}
		}
		cands, err := listFolderCoverCandidates(path, showHidden, stop)
		if errors.Is(err, errCoverStopped) {
			giveUp()
			return
		}
		if err != nil {
			logDebugf("folder cover skipped for %s: %v", path, err)
			a.publishFolderCover(root, index, path, gen, edge, "", nil, false)
			return
		}
		if len(cands) == 0 {
			a.publishFolderCover(root, index, path, gen, edge, "", nil, false)
			return
		}
		digest := folderCoverDigest(cands)
		key := folderCoverKey(path, digest, edge)
		if bitmap := thumbLRU.Get(key); bitmap != nil {
			a.publishFolderCover(root, index, path, gen, edge, key, bitmap, false)
			return
		}
		sources, err := decodeFolderSources(cands, edge, stop)
		if errors.Is(err, errCoverStopped) {
			giveUp()
			return
		}
		if len(sources) == 0 {
			logDebugf("folder cover undecodable for %s (%d files)", path, len(cands))
			a.publishFolderCover(root, index, path, gen, edge, "", nil, false)
			return
		}
		img := composeFolderCover(sources, edge)
		if img == nil {
			a.publishFolderCover(root, index, path, gen, edge, "", nil, false)
			return
		}
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		if err := enc.Encode(&buf, img); err != nil {
			logDebugf("folder cover encode failed for %s: %v", path, err)
			a.publishFolderCover(root, index, path, gen, edge, "", nil, false)
			return
		}
		bitmap, err := ui.DecodeBitmap(buf.Bytes())
		if err != nil {
			logDebugf("folder cover bitmap failed for %s: %v", path, err)
			a.publishFolderCover(root, index, path, gen, edge, "", nil, false)
			return
		}
		a.publishFolderCover(root, index, path, gen, edge, key, bitmap, true)
	})
}

// folderCoverStop reports that this claim should not decode another file.
// A superseded claim stops at once. A card that is still being built, or
// was built a moment ago, keeps going. Once it has been off screen, the
// same thumbAbandoned path a file thumbnail uses clears the claim.
func (a *appState) folderCoverStop(index int, path string, gen int) bool {
	if a.thumbClaimStale(index, path, gen) {
		return true
	}
	a.mu.Lock()
	if index < 0 || index >= len(a.Items) || a.Items[index].Path != path || a.Items[index].LoadGen != gen {
		a.mu.Unlock()
		return true
	}
	seen := atomic.LoadInt64(&a.Items[index].Seen)
	a.mu.Unlock()
	if seen == 0 || time.Since(time.Unix(0, seen)) < folderCoverGrace {
		return false
	}
	return a.thumbAbandoned(index, path, gen)
}

func (a *appState) publishFolderCover(root string, index int, path string, gen, edge int, key string, bitmap *ui.Bitmap, store bool) {
	if store && bitmap != nil && key != "" {
		thumbLRU.Put(key, bitmap, edge)
	}
	a.update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.Root != root || index < 0 || index >= len(a.Items) || a.Items[index].Path != path || a.Items[index].LoadGen != gen {
			return
		}
		current := &a.Items[index]
		current.Loading = false
		current.Failed = false
		current.Decoded = true
		current.ThumbEdge = edge
		if bitmap != nil {
			current.Bitmap = bitmap
			current.CoverKey = key
		}
	})
}

func folderCoverDigest(cands []folderCandidate) string {
	h := fnv.New64a()
	for _, cand := range cands {
		_, _ = fmt.Fprintf(h, "%s\x00%d\n", cand.rel, cand.mtime)
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

func listFolderCoverCandidates(root string, hidden bool, stop func() bool) ([]folderCandidate, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("not a directory")
	}
	if stop != nil && stop() {
		return nil, errCoverStopped
	}
	type node struct {
		path  string
		depth int
	}
	queue := []node{{root, 0}}
	seenDir := map[string]bool{root: true}
	var out []folderCandidate
	visited := 0
	for len(queue) > 0 && len(out) < folderCoverDigestN && visited < folderCoverVisitMax {
		if stop != nil && stop() {
			return nil, errCoverStopped
		}
		cur := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(cur.path)
		if err != nil {
			if cur.path == root {
				return nil, err
			}
			continue
		}
		slices.SortFunc(entries, func(a, b os.DirEntry) int {
			an := strings.ToLower(a.Name())
			bn := strings.ToLower(b.Name())
			if an != bn {
				return strings.Compare(an, bn)
			}
			return strings.Compare(a.Name(), b.Name())
		})
		var subs []string
		for _, entry := range entries {
			visited++
			name := entry.Name()
			if !hidden && strings.HasPrefix(name, ".") {
				continue
			}
			path := filepath.Join(cur.path, name)
			if entry.IsDir() {
				if entry.Type()&os.ModeSymlink != 0 {
					continue
				}
				if cur.depth+1 < folderCoverDepth {
					subs = append(subs, path)
				}
				continue
			}
			ext := strings.TrimPrefix(filepath.Ext(name), ".")
			if !isMediaFile(ext) {
				continue
			}
			fi, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil || rel == "." {
				rel = name
			}
			out = append(out, folderCandidate{
				path:  path,
				rel:   filepath.ToSlash(rel),
				mtime: fi.ModTime().UnixNano(),
				video: isVideoFile(ext),
			})
			if len(out) >= folderCoverDigestN || visited >= folderCoverVisitMax {
				break
			}
		}
		for _, sub := range subs {
			clean := filepath.Clean(sub)
			if seenDir[clean] {
				continue
			}
			seenDir[clean] = true
			queue = append(queue, node{clean, cur.depth + 1})
		}
	}
	return out, nil
}

func decodeFolderSources(cands []folderCandidate, edge int, stop func() bool) ([]coverSource, error) {
	out := make([]coverSource, 0, folderCoverMaxLayers)
	for _, cand := range cands {
		if len(out) >= folderCoverMaxLayers {
			break
		}
		if stop != nil && stop() {
			return nil, errCoverStopped
		}
		data, err := withThumbSlot(func() ([]byte, error) {
			if cand.video {
				return videoFrameJPEG(cand.path, edge)
			}
			return thumbBytes(cand.path, false, edge)
		})
		if err != nil || len(data) == 0 {
			continue
		}
		img, _, decErr := image.Decode(bytes.NewReader(data))
		if decErr != nil {
			continue
		}
		out = append(out, coverSource{img: img, video: cand.video})
	}
	return out, nil
}

func withThumbSlot(fn func() ([]byte, error)) ([]byte, error) {
	thumbGate <- struct{}{}
	defer func() { <-thumbGate }()
	return fn()
}

// coverLayers is the fan, back to front. The last card is the front: full
// opacity, no rotation, and larger than the cards behind it. Behind cards
// turn by a fixed 3–6 degrees.
func coverLayers(n int) []coverLayer {
	switch n {
	case 1:
		return []coverLayer{{scale: 0.84, opacity: 1}}
	case 2:
		return []coverLayer{
			{angle: -5, scale: 0.68, opacity: 0.86, dx: -0.08, dy: 0.025},
			{scale: 0.78, opacity: 1, dx: 0.02, dy: -0.01},
		}
	case 3:
		return []coverLayer{
			{angle: 6, scale: 0.58, opacity: 0.76, dx: 0.085, dy: 0.04},
			{angle: -4, scale: 0.68, opacity: 0.9, dx: -0.08, dy: -0.02},
			{scale: 0.78, opacity: 1},
		}
	default:
		if n > folderCoverMaxLayers {
			n = folderCoverMaxLayers
		}
		if n < 4 {
			return nil
		}
		return []coverLayer{
			{angle: -6, scale: 0.50, opacity: 0.7, dx: -0.015, dy: 0.08},
			{angle: 5, scale: 0.58, opacity: 0.8, dx: 0.085, dy: -0.02},
			{angle: -3.5, scale: 0.66, opacity: 0.9, dx: -0.075, dy: 0.02},
			{scale: 0.76, opacity: 1, dx: 0.012, dy: -0.008},
		}
	}
}

// fitCoverLayer shrinks a card until the rotated print and its shadow stay
// inside the square.
func fitCoverLayer(l coverLayer) coverLayer {
	const pad = 0.055
	rad := l.angle * math.Pi / 180
	stretch := math.Abs(math.Cos(rad)) + math.Abs(math.Sin(rad))
	if stretch < 1 {
		stretch = 1
	}
	roomX := 0.5 - pad - math.Abs(l.dx)
	roomY := 0.5 - pad - math.Abs(l.dy)
	room := math.Min(roomX, roomY)
	if room < 0.2 {
		room = 0.2
	}
	if limit := room * 2 / stretch; l.scale > limit {
		l.scale = limit
	}
	if l.opacity < 0 {
		l.opacity = 0
	}
	if l.opacity > 1 {
		l.opacity = 1
	}
	return l
}

// composeFolderCover paints sources onto a transparent square. sources[0]
// is the front print. One print also gets a folder badge; none returns nil
// so the card keeps the plain folder icon.
func composeFolderCover(sources []coverSource, edge int) *image.RGBA {
	if edge < 64 {
		edge = 64
	}
	if edge > 1024 {
		edge = 1024
	}
	n := len(sources)
	if n > folderCoverMaxLayers {
		n = folderCoverMaxLayers
		sources = sources[:n]
	}
	if n == 0 {
		return nil
	}
	layers := coverLayers(n)
	if len(layers) != n {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, edge, edge))
	mask := make([]uint16, edge*edge)
	painted := false
	for back := 0; back < n; back++ {
		src := sources[n-1-back]
		if src.img == nil {
			continue
		}
		paintCoverLayer(dst, mask, src, fitCoverLayer(layers[back]), edge)
		painted = true
	}
	if !painted {
		return nil
	}
	if n == 1 {
		paintFolderBadge(dst)
	}
	return dst
}

func paintCoverLayer(dst *image.RGBA, mask []uint16, src coverSource, layer coverLayer, edge int) {
	maxSide := int(layer.scale * float64(edge))
	sprite := coverSprite(src.img, maxSide, src.video)
	if sprite == nil {
		return
	}
	cx := float64(edge)/2 + layer.dx*float64(edge)
	cy := float64(edge)/2 + layer.dy*float64(edge)
	clear(mask)
	ox := float64(max(2, edge/90))
	oy := float64(max(3, edge/70))
	splatAlpha(mask, edge, sprite, cx+ox, cy+oy, layer.angle, layer.opacity)
	radius := max(2, edge/80)
	boxBlur(mask, edge, edge, radius)
	boxBlur(mask, edge, edge, radius)
	compositeShadow(dst, mask, 0.55)
	splatColor(dst, sprite, cx, cy, layer.angle, layer.opacity)
}

func coverSprite(src image.Image, maxSide int, video bool) *image.RGBA {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw < 1 || sh < 1 {
		return nil
	}
	if maxSide < 12 {
		maxSide = 12
	}
	border := max(2, maxSide/36)
	content := maxSide - 2*border
	if content < 1 {
		content = 1
	}
	scale := math.Min(float64(content)/float64(sw), float64(content)/float64(sh))
	dw := max(1, int(math.Round(float64(sw)*scale)))
	dh := max(1, int(math.Round(float64(sh)*scale)))
	if dw > content {
		dw = content
	}
	if dh > content {
		dh = content
	}
	sprite := image.NewRGBA(image.Rect(0, 0, dw+2*border, dh+2*border))
	borderColor := color.RGBA{255, 255, 255, 255}
	if video {
		borderColor = color.RGBA{62, 62, 62, 255}
	}
	draw.Draw(sprite, sprite.Bounds(), &image.Uniform{borderColor}, image.Point{}, draw.Src)
	inner := image.Rect(border, border, border+dw, border+dh)
	draw.Draw(sprite, inner, &image.Uniform{color.RGBA{206, 206, 206, 255}}, image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(sprite, inner, src, b, draw.Over, nil)
	softenRoundRect(sprite, max(4, min(dw, dh)/18))
	return sprite
}

func softenRoundRect(img *image.RGBA, radius int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	r := float64(radius)
	if r > float64(min(w, h))/2 {
		r = float64(min(w, h)) / 2
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cov := roundRectCoverage(float64(x)+0.5, float64(y)+0.5, float64(w), float64(h), r)
			i := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			if cov <= 0 {
				img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 0, 0, 0, 0
				continue
			}
			if cov >= 1 {
				continue
			}
			a := float64(img.Pix[i+3]) * cov
			img.Pix[i+3] = uint8(a + 0.5)
		}
	}
}

func roundRectCoverage(x, y, w, h, r float64) float64 {
	if r <= 0 {
		if x >= 0 && y >= 0 && x < w && y < h {
			return 1
		}
		return 0
	}
	cx, cy := x, y
	if cx < r {
		cx = r
	} else if cx > w-r {
		cx = w - r
	}
	if cy < r {
		cy = r
	} else if cy > h-r {
		cy = h - r
	}
	sd := r - math.Hypot(x-cx, y-cy)
	t := sd + 0.5
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return t
}

func paintFolderBadge(dst *image.RGBA) {
	side := dst.Bounds().Dx()
	badge := side / 5
	if badge < 34 {
		badge = 34
	}
	if badge > side/3 {
		badge = side / 3
	}
	margin := max(8, side/14)
	x := side - margin - badge
	y := side - margin - badge
	rad := max(4, badge/6)
	fillRoundRect(dst, x+3, y+4, badge, badge, rad, color.RGBA{0, 0, 0, 80})
	fillRoundRect(dst, x-2, y-2, badge+4, badge+4, rad+2, color.RGBA{255, 255, 255, 235})
	tabW := badge * 46 / 100
	tabH := max(6, badge*24/100)
	fillRoundRect(dst, x+3, y+2, tabW, tabH+rad, max(2, rad/2), color.RGBA{132, 196, 238, 255})
	fillRoundRect(dst, x+3, y+tabH, badge-6, badge-tabH-4, rad, color.RGBA{70, 150, 220, 255})
}

func fillRoundRect(img *image.RGBA, x, y, w, h, rad int, c color.RGBA) {
	if w <= 0 || h <= 0 || c.A == 0 {
		return
	}
	b := img.Bounds()
	r := float64(rad)
	if limit := float64(min(w, h)) / 2; r > limit {
		r = limit
	}
	for yy := 0; yy < h; yy++ {
		py := y + yy
		if py < b.Min.Y || py >= b.Max.Y {
			continue
		}
		for xx := 0; xx < w; xx++ {
			px := x + xx
			if px < b.Min.X || px >= b.Max.X {
				continue
			}
			cov := roundRectCoverage(float64(xx)+0.5, float64(yy)+0.5, float64(w), float64(h), r)
			if cov <= 0 {
				continue
			}
			a := uint32(float64(c.A)*cov + 0.5)
			if a > 255 {
				a = 255
			}
			blendOver(img.Pix, img.PixOffset(px, py), uint32(c.R), uint32(c.G), uint32(c.B), a)
		}
	}
}

func splatAlpha(mask []uint16, edge int, src *image.RGBA, cx, cy, deg, opacity float64) {
	if opacity <= 0 {
		return
	}
	splat(src, cx, cy, deg, func(x, y int, _, _, _, a uint32) {
		if x < 0 || y < 0 || x >= edge || y >= edge || a == 0 {
			return
		}
		v := uint32(float64(a)*opacity + 0.5)
		if v > 255 {
			v = 255
		}
		i := y*edge + x
		if uint16(v) > mask[i] {
			mask[i] = uint16(v)
		}
	})
}

func splatColor(dst *image.RGBA, src *image.RGBA, cx, cy, deg, opacity float64) {
	b := dst.Bounds()
	splat(src, cx, cy, deg, func(x, y int, r, g, bcol, a uint32) {
		if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y || a == 0 {
			return
		}
		aa := uint32(float64(a)*opacity + 0.5)
		if aa > 255 {
			aa = 255
		}
		blendOver(dst.Pix, dst.PixOffset(x, y), r, g, bcol, aa)
	})
}

func splat(src *image.RGBA, cx, cy, deg float64, plot func(x, y int, r, g, b, a uint32)) {
	b := src.Bounds()
	sw, sh := float64(b.Dx()), float64(b.Dy())
	if sw < 1 || sh < 1 {
		return
	}
	rad := deg * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	hw, hh := sw/2, sh/2
	extX := math.Abs(cos)*hw + math.Abs(sin)*hh
	extY := math.Abs(sin)*hw + math.Abs(cos)*hh
	x0 := int(math.Floor(cx - extX))
	y0 := int(math.Floor(cy - extY))
	x1 := int(math.Ceil(cx + extX))
	y1 := int(math.Ceil(cy + extY))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			sx := cos*dx + sin*dy + hw
			sy := -sin*dx + cos*dy + hh
			r, g, bcol, a := sampleRGBA(src, sx, sy)
			if a == 0 {
				continue
			}
			plot(x, y, r, g, bcol, a)
		}
	}
}

func sampleRGBA(src *image.RGBA, x, y float64) (r, g, b, a uint32) {
	bounds := src.Bounds()
	w, h := float64(bounds.Dx()), float64(bounds.Dy())
	if x < 0 || y < 0 || x >= w || y >= h {
		return 0, 0, 0, 0
	}
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1, y1 := x0+1, y0+1
	if x1 >= bounds.Dx() {
		x1 = x0
	}
	if y1 >= bounds.Dy() {
		y1 = y0
	}
	fx := x - float64(x0)
	fy := y - float64(y0)
	c00 := src.Pix[src.PixOffset(bounds.Min.X+x0, bounds.Min.Y+y0):]
	c10 := src.Pix[src.PixOffset(bounds.Min.X+x1, bounds.Min.Y+y0):]
	c01 := src.Pix[src.PixOffset(bounds.Min.X+x0, bounds.Min.Y+y1):]
	c11 := src.Pix[src.PixOffset(bounds.Min.X+x1, bounds.Min.Y+y1):]
	lerp := func(p, q byte, t float64) float64 {
		return float64(p) + (float64(q)-float64(p))*t
	}
	ch := func(i int) uint32 {
		top := lerp(c00[i], c10[i], fx)
		bot := lerp(c01[i], c11[i], fx)
		return uint32(top + (bot-top)*fy + 0.5)
	}
	return ch(0), ch(1), ch(2), ch(3)
}

func blendOver(pix []byte, i int, sr, sg, sb, sa uint32) {
	if sa >= 255 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = byte(sr), byte(sg), byte(sb), 255
		return
	}
	if sa == 0 {
		return
	}
	dr := uint32(pix[i])
	dg := uint32(pix[i+1])
	db := uint32(pix[i+2])
	da := uint32(pix[i+3])
	inv := 255 - sa
	na := sa + da*inv/255
	if na == 0 {
		return
	}
	pix[i] = byte((sr*sa + dr*da*inv/255) / na)
	pix[i+1] = byte((sg*sa + dg*da*inv/255) / na)
	pix[i+2] = byte((sb*sa + db*da*inv/255) / na)
	pix[i+3] = byte(na)
}

func compositeShadow(dst *image.RGBA, mask []uint16, strength float64) {
	b := dst.Bounds()
	w := b.Dx()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < w; x++ {
			m := mask[y*w+x]
			if m == 0 {
				continue
			}
			a := uint32(float64(m)*strength + 0.5)
			if a > 180 {
				a = 180
			}
			blendOver(dst.Pix, dst.PixOffset(b.Min.X+x, b.Min.Y+y), 0, 0, 0, a)
		}
	}
}

func boxBlur(src []uint16, w, h, radius int) {
	if radius < 1 || w < 1 || h < 1 || len(src) < w*h {
		return
	}
	stride := w + 1
	sat := make([]int, stride*(h+1))
	for y := 1; y <= h; y++ {
		row := 0
		for x := 1; x <= w; x++ {
			row += int(src[(y-1)*w+(x-1)])
			sat[y*stride+x] = sat[(y-1)*stride+x] + row
		}
	}
	tmp := make([]uint16, w*h)
	for y := 0; y < h; y++ {
		y0 := y - radius
		y1 := y + radius + 1
		if y0 < 0 {
			y0 = 0
		}
		if y1 > h {
			y1 = h
		}
		for x := 0; x < w; x++ {
			x0 := x - radius
			x1 := x + radius + 1
			if x0 < 0 {
				x0 = 0
			}
			if x1 > w {
				x1 = w
			}
			n := (x1 - x0) * (y1 - y0)
			if n <= 0 {
				continue
			}
			sum := sat[y1*stride+x1] - sat[y0*stride+x1] - sat[y1*stride+x0] + sat[y0*stride+x0]
			tmp[y*w+x] = uint16(sum / n)
		}
	}
	copy(src, tmp)
}
