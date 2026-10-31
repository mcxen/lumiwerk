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

func TestPinyinContains(t *testing.T) {
	if !pinyinContains("小猫视频", "xmsp") && !pinyinContains("小猫视频", "xiaomaoshipin") {
		t.Fatalf("pinyin search failed")
	}
	if pinyinContains("小猫视频", "xyz") {
		t.Fatalf("unexpected pinyin match")
	}
}

func TestFormatRateAndBits(t *testing.T) {
	if got := formatRate("30000/1001"); got != "29.97" {
		t.Fatalf("rate = %s", got)
	}
	if formatRate("0/0") != "" || formatRate("30") != "" {
		t.Fatal("bad rates should be empty")
	}
	if got := formatBits(2500000); got != "2.50 Mbps" {
		t.Fatalf("bits = %s", got)
	}
}
