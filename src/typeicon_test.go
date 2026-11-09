package main

import (
	"testing"

	"github.com/egoist/mygo"
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
			paintMediaThumb(c, item, 96, ui.Cover, 96)
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

func TestFileIconBitmapSkipsWhenAppNotReady(t *testing.T) {
	if fileIconBitmap("", 32) != nil || fileIconBitmap("/no/such", 0) != nil {
		t.Fatal("empty path or size 0 returned an icon")
	}
	// App is not ready in a bare unit test: must not panic via needsApp.
	if got := fileIconBitmap("/no/such/folder", 32); got != nil && !mygo.App.IsReady() {
		t.Fatal("icon returned while app is not ready")
	}
}

func TestFolderCardAndDetailRowPaintWithoutCover(t *testing.T) {
	item := &mediaItem{Path: "/probe/album", Name: "album", IsDir: true}
	te := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).Size(96, 96).Children(func() {
				paintMediaThumb(c, item, 96, ui.Cover, 96)
			})
			ui.Row(c).Children(func() {
				paintFolderGlyph(c, item.Path, 15)
				ui.Text(c, item.Name)
			})
		})
	}, 200, 140)
	te.Frame()
	if te.Image() == nil {
		t.Fatal("expected a painted folder card")
	}
}

func TestHeroTransitionFlies(t *testing.T) {
	tr := heroTransition("/hero/a.png")
	if tr.Match == nil || !tr.Fly {
		t.Fatalf("hero transition = %+v", tr)
	}
}
