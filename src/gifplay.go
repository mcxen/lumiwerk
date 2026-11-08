package main

import (
	"os"
	"strings"

	"github.com/egoist/mygo/ui"
)

func isGIFFile(ext string) bool {
	return strings.ToLower(strings.TrimPrefix(ext, ".")) == "gif"
}

func (a *appState) closeGIF() {
	if a.gifPlayer != nil {
		a.gifPlayer.Close()
	}
	a.gifPlayer = nil
	a.gifPath = ""
}

// ensureGIF builds a looping GIF VideoPlayer for the large view.
// Cards keep the still thumbnail path; real videos keep libmpv.
func (a *appState) ensureGIF(item *mediaItem) bool {
	if item == nil || item.IsVideo || !isGIFFile(item.Ext) {
		a.closeGIF()
		return false
	}
	if a.gifPlayer != nil && a.gifPath == item.Path {
		return true
	}
	a.closeGIF()
	data, err := os.ReadFile(item.Path)
	if err != nil {
		return false
	}
	dec, err := ui.NewGIFDecoder(data)
	if err != nil {
		return false
	}
	player, err := ui.NewVideoPlayer(dec)
	if err != nil {
		return false
	}
	player.SetLoop(true)
	a.gifPlayer = player
	a.gifPath = item.Path
	return true
}
