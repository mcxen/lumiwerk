package main

import "testing"

func TestIsMediaFile(t *testing.T) {
	for _, ext := range []string{"png", "jpg", "jpeg", "gif", "webp", "bmp", "tiff", "tif", "mp4", "mov", "mkv", "avi", "webm", "m4v", "ts", "mts"} {
		if !isMediaFile(ext) {
			t.Fatalf("isMediaFile(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{"txt", "doc", "zip", "pdf", "go", ""} {
		if isMediaFile(ext) {
			t.Fatalf("isMediaFile(%q) = true, want false", ext)
		}
	}
}

func TestApplyFilter(t *testing.T) {
	a := &appState{Items: []mediaItem{{Name: "apple.png"}, {Name: "banana.mp4"}, {Name: "cherry.jpg"}}, Visible: make([]int, 0, 3)}
	a.Query = "a"
	a.applyFilter()
	if len(a.Visible) != 2 {
		t.Fatalf("filtered to %d items, want 2", len(a.Visible))
	}
}
