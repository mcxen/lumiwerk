package main

import (
	"context"
	"testing"
)

func TestShelvedSkipsThumbnailDecode(t *testing.T) {
	a := &appState{
		shelved: true,
		Items:   []mediaItem{{Path: "/tmp/a.jpg", Name: "a.jpg", Ext: "jpg"}},
	}
	a.loadThumb("/tmp", 0)
	if a.Items[0].Loading || a.Items[0].LoadGen != 0 {
		t.Fatalf("shelved window started a decode: loading=%v gen=%d", a.Items[0].Loading, a.Items[0].LoadGen)
	}
}

func TestShelvedDoesNotWatch(t *testing.T) {
	a := &appState{shelved: true, Root: t.TempDir()}
	a.startWatcher()
	if a.watchStop != nil {
		t.Fatal("watcher started while the window was shelved")
	}
}

func TestSuspendDropsThumbnailClaimsAndPausesVideo(t *testing.T) {
	backend := &fakePlaybackBackend{playing: true}
	kernel := newPlaybackSession()
	kernel.backend = backend
	kernel.path = "/tmp/a.mp4"
	kernel.playing = true
	kernel.ready = true
	a := &appState{
		AutoScroll: true,
		AutoPlay:   true,
		Video:      &videoView{kernel: kernel, path: kernel.path},
		Items: []mediaItem{
			{Path: "/tmp/a.jpg", Loading: true, LoadGen: 3, LargeLoading: true},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.decodeCtx, a.decodeCancel = ctx, cancel

	a.suspendBackground()

	if !a.isShelved() {
		t.Fatal("window not marked shelved")
	}
	if a.Items[0].Loading || a.Items[0].LargeLoading || a.Items[0].LoadGen == 3 {
		t.Fatalf("claim kept: %+v", a.Items[0])
	}
	if ctx.Err() == nil {
		t.Fatal("decode context still live")
	}
	if a.videoPlaying() || !a.resumeVideo {
		t.Fatalf("playing=%v resume=%v", a.videoPlaying(), a.resumeVideo)
	}
	if a.AutoScroll || a.AutoPlay || !a.shelvedScroll || !a.shelvedPlay {
		t.Fatal("automation was not parked")
	}
	if len(backend.playStates) != 1 || backend.playStates[0] {
		t.Fatalf("backend play states = %v, want [false]", backend.playStates)
	}

	// A second suspend must not forget that playback should resume.
	a.suspendBackground()
	if !a.resumeVideo {
		t.Fatal("second suspend cleared the resume flag")
	}
}

func TestResumeRestoresDecodesPlaybackAndAutomation(t *testing.T) {
	// Root stays empty so resume does not call mygo.WatchFiles, which
	// panics without a running app. The shelved guard is covered above.
	backend := &fakePlaybackBackend{}
	kernel := newPlaybackSession()
	kernel.backend = backend
	kernel.path = "/tmp/a.mp4"
	kernel.ready = true
	a := &appState{
		resumeVideo:   true,
		shelvedScroll: true,
		shelvedPlay:   true,
		Video:         &videoView{kernel: kernel, path: kernel.path},
	}
	a.setShelved(true)

	a.unshelve()

	if a.isShelved() {
		t.Fatal("still shelved")
	}
	if a.decodeCtx == nil || a.decodeCtx.Err() != nil {
		t.Fatal("decode context was not restored")
	}
	if !a.videoPlaying() || a.resumeVideo {
		t.Fatalf("playing=%v resume=%v", a.videoPlaying(), a.resumeVideo)
	}
	if !a.AutoScroll || !a.AutoPlay || a.shelvedScroll || a.shelvedPlay {
		t.Fatal("automation was not restored")
	}
	if len(backend.playStates) != 1 || !backend.playStates[0] {
		t.Fatalf("backend play states = %v, want [true]", backend.playStates)
	}
}

func TestThumbClaimStaleAfterShelf(t *testing.T) {
	a := &appState{Items: []mediaItem{{Path: "/tmp/a.jpg", LoadGen: 4, Loading: true}}}
	ctx, cancel := context.WithCancel(context.Background())
	a.decodeCtx, a.decodeCancel = ctx, cancel
	if a.thumbClaimStale(0, "/tmp/a.jpg", 4) {
		t.Fatal("fresh claim reported stale")
	}
	a.suspendBackground()
	if !a.thumbClaimStale(0, "/tmp/a.jpg", 4) {
		t.Fatal("shelved claim was still current")
	}
}
