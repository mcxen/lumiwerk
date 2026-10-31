package main

import (
	"testing"
)

func TestIsMediaFile(t *testing.T) {
	for _, ext := range []string{"png", "PNG", "jpg", "jpeg", "webp", "gif", "bmp"} {
		if !isMediaFile(ext) {
			t.Errorf("isMediaFile(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{"txt", "mp4", "mov", "cr2", ""} {
		if isMediaFile(ext) {
			t.Errorf("isMediaFile(%q) = true, want false", ext)
		}
	}
}

func TestApplyFilter(t *testing.T) {
	a := &appState{
		Items: []mediaItem{
			{Name: "bridge.jpg"},
			{Name: "poster.png"},
			{Name: "notes.txt"},
		},
	}
	a.applyFilter()
	if len(a.Visible) != 3 {
		t.Fatalf("Visible len = %d, want 3", len(a.Visible))
	}
	a.Query = "poster"
	a.applyFilter()
	if len(a.Visible) != 1 || a.Visible[0] != 1 {
		t.Fatalf("Visible = %v, want [1]", a.Visible)
	}
}
