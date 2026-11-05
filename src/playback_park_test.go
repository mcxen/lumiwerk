package main

import (
	"image"
	"testing"
)

type stubBackend struct {
	name            string
	parked, playing bool
	path            string
}

func (b *stubBackend) Name() string { return b.name }
func (b *stubBackend) Open(path string, at float64, playing bool) error {
	b.path = path
	b.playing = playing
	b.parked = false
	return nil
}
func (b *stubBackend) Close() error             { b.path = ""; return nil }
func (b *stubBackend) Frame() image.Image       { return nil }
func (b *stubBackend) Position() float64        { return 0 }
func (b *stubBackend) Duration() float64        { return 0 }
func (b *stubBackend) SetPlaying(playing bool)  { b.playing = playing }
func (b *stubBackend) Seek(seconds float64)     {}
func (b *stubBackend) SetVolume(volume float64) {}
func (b *stubBackend) SetSpeed(speed float64)   {}
func (b *stubBackend) FrameStep(direction int) (float64, error) {
	return 0, nil
}
func (b *stubBackend) Park() { b.parked = true; b.playing = false; b.path = "" }

func TestPlaybackSessionParkKeepsBackend(t *testing.T) {
	s := newPlaybackSession()
	stub := &stubBackend{name: "stub"}
	s.backend = stub
	if err := s.Open("/tmp/a.mp4", 0, true); err != nil {
		t.Fatal(err)
	}
	s.Park()
	if stub.parked != true || s.backend != stub {
		t.Fatalf("parked=%v backend kept=%v", stub.parked, s.backend == stub)
	}
	if err := s.Open("/tmp/b.mp4", 0, true); err != nil {
		t.Fatal(err)
	}
	if stub.path != "/tmp/b.mp4" || s.backend != stub {
		t.Fatalf("reuse failed path=%q", stub.path)
	}
}
