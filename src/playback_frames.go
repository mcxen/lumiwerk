package main

import (
	"image"
	"math"
	"sync"
	"unsafe"
)

// mpvFramePool hands rendered frames from the libmpv render thread to the
// main thread without copying them.
//
// ui.Bitmap.Update keeps an *image.RGBA whose rows are 4×width bytes as it
// is and uploads it when it draws, so a buffer the view was given must not
// be written again until the view has moved on. The pool therefore never
// renders into: the newest published buffer, the buffer being rendered, or
// the last two buffers take returned (the one on screen and the one before
// it, which a GPU upload may still be reading). Four or five buffers cover
// that, so steady playback allocates nothing.
//
// Every publish wraps the pixels in a new image.RGBA header, so two frames
// with different pictures are never the same pointer even when they share
// a buffer.
type mpvFramePool struct {
	mu      sync.Mutex
	free    [][]byte
	latest  *image.RGBA // published, not yet taken
	shown   [2][]byte   // buffers of the last two frames take returned
	current *image.RGBA // the frame take returned last
	gen     uint64
}

func sameBuffer(a, b []byte) bool {
	return len(a) > 0 && len(b) > 0 && &a[0] == &b[0]
}

// acquire returns a frame to render into, sized width×height.
func (p *mpvFramePool) acquire(width, height int) *image.RGBA {
	need := width * 4 * height
	p.mu.Lock()
	var pix []byte
	for i := len(p.free) - 1; i >= 0; i-- {
		if cap(p.free[i]) >= need {
			pix = p.free[i][:need]
			p.free = append(p.free[:i], p.free[i+1:]...)
			break
		}
	}
	p.mu.Unlock()
	if pix == nil {
		pix = make([]byte, need)
	}
	return &image.RGBA{Pix: pix, Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
}

// release returns an unpublished frame's buffer.
func (p *mpvFramePool) release(frame *image.RGBA) {
	if frame == nil {
		return
	}
	p.mu.Lock()
	p.recycleLocked(frame.Pix)
	p.mu.Unlock()
}

func (p *mpvFramePool) recycleLocked(pix []byte) {
	if pix == nil || sameBuffer(pix, p.shown[0]) || sameBuffer(pix, p.shown[1]) {
		return
	}
	if p.latest != nil && sameBuffer(pix, p.latest.Pix) {
		return
	}
	// Bound the pool: buffers of an old, larger size are dropped.
	if len(p.free) >= 3 {
		p.free = p.free[1:]
	}
	p.free = append(p.free, pix[:cap(pix)])
}

// publish makes frame the newest; a published frame nobody took is reused.
func (p *mpvFramePool) publish(frame *image.RGBA) {
	p.mu.Lock()
	old := p.latest
	p.latest = frame
	p.gen++
	if old != nil {
		p.recycleLocked(old.Pix)
	}
	p.mu.Unlock()
}

// take returns the newest frame, or the one it returned last when nothing
// new was published.
func (p *mpvFramePool) take() *image.RGBA {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.latest == nil {
		return p.current
	}
	frame := p.latest
	p.latest = nil
	retired := p.shown[1]
	p.shown[1] = p.shown[0]
	p.shown[0] = frame.Pix
	p.current = frame
	if retired != nil {
		p.recycleLocked(retired)
	}
	return frame
}

// generation counts published frames.
func (p *mpvFramePool) generation() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gen
}

// clear forgets the frames of the previous file. Buffers the view may still
// show stay out of the free list until two newer frames were taken.
func (p *mpvFramePool) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.latest != nil {
		latest := p.latest
		p.latest = nil
		p.recycleLocked(latest.Pix)
	}
	p.current = nil
}

// mpvMaxPixels caps the software render target. 1920×1080 keeps one frame at
// ~8 MiB, which the render thread fills and the GPU uploads at 60 fps without
// stalling; larger sources are scaled down by mpv's zimg path.
const mpvMaxPixels = 1920 * 1080

// mpvRenderSize scales the display size under mpvMaxPixels and rounds the
// width to 16 pixels (a 64-byte row) so mpv's SIMD path is aligned and the
// RGBA rows stay exactly 4×width bytes. The height follows the rounded width
// to keep the aspect ratio.
func mpvRenderSize(width, height int) (int, int) {
	if width < 2 || height < 2 {
		return 0, 0
	}
	w, h := float64(width), float64(height)
	if px := w * h; px > mpvMaxPixels {
		scale := math.Sqrt(mpvMaxPixels / px)
		w, h = w*scale, h*scale
	}
	rw := int(w) &^ 15
	if rw < 16 {
		rw = 16
	}
	rh := int(math.Round(h * float64(rw) / w))
	if rh < 2 {
		rh = 2
	}
	return rw, rh
}

// opaque sets the padding byte of every rgb0 pixel to 255 four bytes at a
// time; mpv leaves it undefined and the picture is drawn premultiplied.
func opaque(pix []byte) {
	if len(pix) < 4 {
		return
	}
	words := unsafe.Slice((*uint32)(unsafe.Pointer(&pix[0])), len(pix)/4)
	var alpha uint32
	if nativeLittleEndian {
		alpha = 0xff000000
	} else {
		alpha = 0x000000ff
	}
	for i := range words {
		words[i] |= alpha
	}
}

var nativeLittleEndian = func() bool {
	x := uint16(1)
	return *(*byte)(unsafe.Pointer(&x)) == 1
}()
