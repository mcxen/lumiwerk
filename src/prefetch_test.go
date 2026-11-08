package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPrefetchAroundLoadsVisibleThenNeighbors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128}
	a.Items = make([]mediaItem, 8)
	a.Visible = make([]int, 8)
	for i := range a.Items {
		a.Items[i] = mediaItem{Path: fmt.Sprintf("/t/%d.jpg", i), Name: fmt.Sprintf("%d.jpg", i), Ext: "jpg"}
		a.Visible[i] = i
	}
	a.prefetchAround("/t", 2, 3, 8, 2)
	var loading []int
	for i := range a.Items {
		if a.Items[i].Loading {
			loading = append(loading, i)
		}
	}
	want := []int{0, 1, 2, 3, 4, 5}
	if len(loading) != len(want) {
		t.Fatalf("loading %v want %v", loading, want)
	}
	for i, v := range want {
		if loading[i] != v {
			t.Fatalf("loading %v want %v", loading, want)
		}
	}
	if a.Items[6].Loading || a.Items[7].Loading {
		t.Fatal("far items were prefetched")
	}
}

func TestPrefetchSkipsWhenWindowHidden(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, windowHidden: true}
	a.Items = []mediaItem{{Path: "/t/a.jpg", Name: "a.jpg", Ext: "jpg"}}
	a.Visible = []int{0}
	a.prefetchAround("/t", 0, 0, 1, 4)
	a.maybeRequestThumb(ui.Element{}, "/t", 0)
	if a.Items[0].Loading {
		t.Fatal("hidden window started a decode")
	}
}

func TestPrefetchSkipsDecodedCards(t *testing.T) {
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128}
	a.Items = []mediaItem{{Path: "/t/a.jpg", Name: "a.jpg", Ext: "jpg", Decoded: true}}
	a.Visible = []int{0}
	a.prefetchAround("/t", 0, 0, 1, 2)
	if a.Items[0].Loading {
		t.Fatal("decoded card started again")
	}
}
