package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// blankFrameBackend has not produced a picture yet.
type blankFrameBackend struct{ fakePlaybackBackend }

func (b *blankFrameBackend) Frame() image.Image { return nil }

// stallingBackend reports a stall so a ready, playing session buffers.
type stallingBackend struct {
	playbackBackend
	stalled bool
}

func (b *stallingBackend) Stalled() bool { return b.stalled }

func badgeVideoState(backend playbackBackend, position float64) *appState {
	a := fakeVideoState(&fakePlaybackBackend{playing: true})
	a.Video.kernel.backend = backend
	a.Video.kernel.position = position
	return a
}

// TestVideoLayerWaitBadges covers the shared spinner badges: the loading
// and buffering captions before the first frame (over the thumbnail or on
// the bare stage) and the status badge over a live frame.
func TestVideoLayerWaitBadges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	thumb := ui.NewBitmap(image.NewNRGBA(image.Rect(0, 0, 16, 9)))
	for _, tc := range []struct {
		name     string
		backend  playbackBackend
		position float64
		thumb    bool
		want     string
	}{
		{"loading over thumbnail", &blankFrameBackend{fakePlaybackBackend{playing: true}}, 0, true, "正在加载"},
		{"loading on stage", &blankFrameBackend{fakePlaybackBackend{playing: true}}, 0, false, "正在加载"},
		{"buffering over thumbnail", &stallingBackend{&blankFrameBackend{fakePlaybackBackend{playing: true, position: 5}}, true}, 5, true, "正在缓冲"},
		{"buffering on stage", &stallingBackend{&blankFrameBackend{fakePlaybackBackend{playing: true, position: 5}}, true}, 5, false, "正在缓冲"},
		{"buffering over frame", &stallingBackend{&fakePlaybackBackend{playing: true, position: 5}, true}, 5, false, "正在缓冲"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := badgeVideoState(tc.backend, tc.position)
			if tc.thumb {
				a.Items[1].Bitmap = thumb
			}
			te := ui.NewTester(func(c *ui.Context) { a.videoLayer(c, false) }, 640, 360)
			te.Frame()
			if !te.HasText(tc.want) {
				t.Fatalf("badge %q missing: %v", tc.want, te.Texts())
			}
		})
	}

	a := badgeVideoState(&stallingBackend{&fakePlaybackBackend{playing: true, position: 5}, false}, 5)
	te := ui.NewTester(func(c *ui.Context) { a.videoLayer(c, false) }, 640, 360)
	te.Frame()
	if te.HasText("正在加载") || te.HasText("正在缓冲") {
		t.Fatalf("playing video shows a wait badge: %v", te.Texts())
	}
}

// edgePathBackend implements the optional edge and rename interfaces the
// FFmpeg backend offers, so the session reaches them without its type.
type edgePathBackend struct {
	fakePlaybackBackend
	edge     int
	edgeSet  int
	notified bool
}

func (b *edgePathBackend) RenderEdge() int        { return b.edge }
func (b *edgePathBackend) SetRenderEdge(edge int) { b.edge = edge; b.edgeSet++ }
func (b *edgePathBackend) Retarget(path string)   { b.path = path }
func (b *edgePathBackend) SetNotify(fn func())    { b.notified = fn != nil }

func TestPlaybackSessionOptionalBackendInterfaces(t *testing.T) {
	plain := newPlaybackSession()
	plain.backend = &fakePlaybackBackend{}
	if _, ok := plain.RenderEdge(); ok {
		t.Fatal("a backend without edgeRenderer reported an edge")
	}
	plain.retarget("/tmp/renamed.mp4")
	if plain.path != "/tmp/renamed.mp4" {
		t.Fatalf("session path = %q", plain.path)
	}

	backend := &edgePathBackend{fakePlaybackBackend: fakePlaybackBackend{path: "/tmp/movie.mp4"}}
	a := &appState{Zoom: 1}
	kernel := newPlaybackSession()
	kernel.backend = backend
	a.wirePlaybackBackend(kernel)
	if backend.edgeSet != 1 || backend.edge != playbackEdge(a.Zoom) || !backend.notified {
		t.Fatalf("wire: edge=%d set=%d notified=%v", backend.edge, backend.edgeSet, backend.notified)
	}
	if edge, ok := kernel.RenderEdge(); !ok || edge != playbackEdge(a.Zoom) {
		t.Fatalf("RenderEdge = %d, %v", edge, ok)
	}
	kernel.retarget("/tmp/renamed.mp4")
	if backend.path != "/tmp/renamed.mp4" || kernel.path != "/tmp/renamed.mp4" {
		t.Fatalf("retarget backend=%q session=%q", backend.path, kernel.path)
	}

	// The real FFmpeg backend satisfies every optional interface above.
	var _ edgeRenderer = (*ffmpegBackend)(nil)
	var _ pathRetargeter = (*ffmpegBackend)(nil)
	var _ backendNotifier = (*ffmpegBackend)(nil)
}
