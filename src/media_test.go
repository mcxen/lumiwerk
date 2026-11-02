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

func TestFinderSortOrdering(t *testing.T) {
	a := &appState{Sort: "name", FolderFirst: true}
	dir := mediaItem{Name: "alpha", Path: "/alpha", IsDir: true}
	file := mediaItem{Name: "beta.png", Path: "/beta.png", Size: 2}
	if a.compareMedia(dir, file) >= 0 {
		t.Fatal("folders should sort before files")
	}

	a.Sort = "name-desc"
	if a.compareMedia(mediaItem{Name: "a.png"}, mediaItem{Name: "b.png"}) <= 0 {
		t.Fatal("descending name order is wrong")
	}

	a.Sort = "size-desc"
	if a.compareMedia(mediaItem{Size: 1}, mediaItem{Size: 2}) <= 0 {
		t.Fatal("descending size order is wrong")
	}

	a.Sort = "name"
	a.FullPathSort = true
	x := mediaItem{Name: "same.png", Path: "/a/same.png"}
	y := mediaItem{Name: "same.png", Path: "/b/same.png"}
	if a.compareMedia(x, y) >= 0 {
		t.Fatal("full path order is wrong")
	}
}

func TestAncestorRootsExpandFinderSidebar(t *testing.T) {
	roots := ancestorRoots("/Users/mcx/Pictures/Qxpicture")
	if len(roots) != 5 || roots[4] != "/Users/mcx/Pictures/Qxpicture" {
		t.Fatalf("roots = %#v", roots)
	}
}
