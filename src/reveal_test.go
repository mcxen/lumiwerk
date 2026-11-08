package main

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestRevealUsesShowItemInFolder(t *testing.T) {
	var got string
	orig := showItemInFolder
	t.Cleanup(func() { showItemInFolder = orig })
	showItemInFolder = func(path string) { got = path }

	a := &appState{Chosen: 0, LargeIndex: -1}
	a.Items = []mediaItem{{Path: "/media/shot.jpg", Name: "shot.jpg", Ext: "jpg"}}
	a.Visible = []int{0}
	a.Selected.Add("/media/shot.jpg")
	a.revealInFinder()
	if got != "/media/shot.jpg" {
		t.Fatalf("browser reveal %q", got)
	}

	got = ""
	a.InLargeView = true
	a.LargeIndex = 0
	a.Items[0].Path = "/media/open.jpg"
	a.revealInFinder()
	if got != "/media/open.jpg" {
		t.Fatalf("large-view reveal %q", got)
	}
}

func TestRevealNotReadyDoesNotPanic(t *testing.T) {
	if mygo.App.IsReady() {
		// A tester in this process already started the app. Calling the
		// real shell would open a production window.
		revealIfReady("")
		return
	}
	revealIfReady("/media/a.jpg")
	if err := revealInFileBrowser(""); err != nil {
		t.Fatal(err)
	}
}
