package main

import "testing"

func TestRecentDedupesAndNeedsDwell(t *testing.T) {
	got := dedupeRecent([]string{"/a/b", "/a/c/", "/A/B", "/a/b/."}, nil)
	if len(got) != 2 || got[0] != "/a/b" || got[1] != "/a/c" {
		t.Fatalf("dedupe = %q", got)
	}
	a := &appState{Root: "/x"}
	a.dwellSeq = 2
	a.recordVisit("/x", 1) // navigated away and back: stale timer
	if len(a.Recent) != 0 {
		t.Fatalf("stale dwell recorded: %q", a.Recent)
	}
	a.Recent = []string{"/y", "/x"}
	a.recordVisit("/x", 2)
	if len(a.Recent) != 2 || a.Recent[0] != "/x" || a.Visited["/x"] == 0 {
		t.Fatalf("recent = %q", a.Recent)
	}
	trail := recentFromTrail([]string{"/p", "/q", "/p"}, map[string]int64{"/p": 5, "/q": 9})
	if r := dedupeRecent(trail, nil); len(r) != 2 || r[0] != "/q" {
		t.Fatalf("from trail = %q", r)
	}
}
