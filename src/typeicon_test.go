package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTypeIconBitmapCaches(t *testing.T) {
	a := typeIconBitmap("jpg", 32)
	b := typeIconBitmap("jpg", 32)
	if a != b {
		t.Fatal("same extension hit the shell twice")
	}
	if typeIconBitmap("", 32) != nil || typeIconBitmap("jpg", 0) != nil {
		t.Fatal("empty ext or size 0 returned an icon")
	}
}

func TestPaintMediaThumbShowsGlyphBeforeDecode(t *testing.T) {
	item := &mediaItem{Path: "/probe/a.jpg", Name: "a.jpg", Ext: "jpg"}
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(96, 96).Children(func() {
			paintMediaThumb(c, item, 96, ui.Cover)
		})
	}, 96, 96)
	te.Frame()
	if te.Image() == nil {
		t.Fatal("expected a painted card")
	}
	item.Failed = true
	te.Frame()
	if te.Image() == nil {
		t.Fatal("failed card did not paint")
	}
}

func TestHeroTransitionFlies(t *testing.T) {
	tr := heroTransition("/hero/a.png")
	if tr.Match == nil || !tr.Fly {
		t.Fatalf("hero transition = %+v", tr)
	}
}
