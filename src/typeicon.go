package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// typeIcons caches Shell.TypeIcon bitmaps by extension and pixel size.
// A nil bitmap means the shell had no icon, so later frames skip the call.
var typeIcons sync.Map

type typeIconEntry struct{ bm *ui.Bitmap }

func typeIconKey(ext string, size int) string {
	return fmt.Sprintf("%s#%d", strings.ToLower(strings.TrimPrefix(ext, ".")), size)
}

// typeIconBitmap is the file-manager glyph for ext, or nil when the shell
// has none or the app is not ready. No file on disk is required.
func typeIconBitmap(ext string, size int) *ui.Bitmap {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if ext == "" || size <= 0 {
		return nil
	}
	key := typeIconKey(ext, size)
	if v, ok := typeIcons.Load(key); ok {
		return v.(*typeIconEntry).bm
	}
	if !mygo.App.IsReady() {
		return nil
	}
	data, err := mygo.Shell.TypeIcon(ext, size)
	var bm *ui.Bitmap
	if err == nil && len(data) > 0 {
		bm, err = ui.DecodeBitmap(data)
		if err != nil {
			bm = nil
		}
	}
	typeIcons.Store(key, &typeIconEntry{bm: bm})
	return bm
}

func paintTypeGlyph(c *ui.Context, ext string, dip float32) {
	px := int(dip * 2)
	if px < 24 {
		px = 24
	}
	if bm := typeIconBitmap(ext, px); bm != nil {
		ui.Image(c, bm).Size(dip, dip).Fit(ui.Contain)
		return
	}
	icon := imageIcon
	if isVideoFile(ext) {
		icon = videoIcon
	}
	ui.Icon(c, icon).FontSize(dip).TextColor(c.Theme().TextMuted)
}
