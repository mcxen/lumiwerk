package main

import (
	"reflect"
	"testing"
)

func copyRuleState(t *testing.T) *appState {
	t.Helper()
	a := &appState{Chosen: -1, LargeIndex: -1, Cursor: -1}
	a.Items = []mediaItem{
		{Path: "/m/a.jpg", Name: "a.jpg"},
		{Path: "/m/b.mp4", Name: "b.mp4", IsVideo: true},
		{Path: "/m/c.png", Name: "c.png"},
	}
	a.Visible = []int{0, 1, 2}
	a.Selected.Add("/m/a.jpg")
	a.Selected.Add("/m/b.mp4")
	return a
}

func TestCopyRequestsKeepEachCallersRules(t *testing.T) {
	a := copyRuleState(t)

	if got, want := a.copyCandidates(a.imageCopyRequest()), []string{"/m/a.jpg", "/m/b.mp4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("image selection = %v, want %v", got, want)
	}
	if got, want := a.copyCandidates(a.videoCopyRequest()), []string{"/m/b.mp4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("video selection = %v, want %v", got, want)
	}
	if got, want := a.copyCandidates(a.configuredCopyRequest()), []string{"/m/a.jpg", "/m/b.mp4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("configured selection = %v, want %v", got, want)
	}

	// Large view on an image: the image folder takes the shown item, the
	// video folder takes nothing.
	a.InLargeView = true
	a.LargeIndex = 2
	if got, want := a.copyCandidates(a.imageCopyRequest()), []string{"/m/c.png"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("image large = %v, want %v", got, want)
	}
	if got := a.copyCandidates(a.videoCopyRequest()); len(got) != 0 {
		t.Fatalf("video large on image = %v, want none", got)
	}

	// Large view on a video: the video folder takes it.
	a.LargeIndex = 1
	if got, want := a.copyCandidates(a.videoCopyRequest()), []string{"/m/b.mp4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("video large = %v, want %v", got, want)
	}

	// Large view with no shown item: the image folder falls back to the
	// selection, the video folder copies nothing.
	a.LargeIndex = -1
	if got, want := a.copyCandidates(a.imageCopyRequest()), []string{"/m/a.jpg", "/m/b.mp4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("image large fallback = %v, want %v", got, want)
	}
	if got := a.copyCandidates(a.videoCopyRequest()); len(got) != 0 {
		t.Fatalf("video large fallback = %v, want none", got)
	}
}

func TestCopyToDestinationToasts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := copyRuleState(t)

	a.copyToImageFolder()
	if a.lastToast != "请先在设置中配置图片文件夹" {
		t.Fatalf("image missing dest toast = %q", a.lastToast)
	}
	a.copyToVideoFolder()
	if a.lastToast != "请先在设置中配置视频文件夹" {
		t.Fatalf("video missing dest toast = %q", a.lastToast)
	}

	a.lastToast = ""
	a.copyToConfigured()
	if a.lastToast != "" {
		t.Fatalf("configured copy toasted %q", a.lastToast)
	}

	// A valid video folder with no selected videos asks for one.
	a.Profiles.VideoDestination = "  " + t.TempDir() + "  "
	a.Selected.Clear()
	a.Selected.Add("/m/a.jpg")
	a.copyToVideoFolder()
	if a.lastToast != "请先选择至少一个视频" {
		t.Fatalf("video no items toast = %q", a.lastToast)
	}

	// The image folder stays silent when nothing is selected.
	a.Profiles.ImageDestination = t.TempDir()
	a.Selected.Clear()
	a.lastToast = ""
	a.copyToImageFolder()
	if a.lastToast != "" {
		t.Fatalf("image no items toasted %q", a.lastToast)
	}
}
