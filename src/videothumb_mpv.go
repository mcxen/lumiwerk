//go:build !android && !ios && mpvbackend

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gen2brain/go-mpv"
)

// mpvVideoThumb has the bundled libmpv write one scaled frame, 10% in,
// through its image output, with no window and no sound.
func mpvVideoThumb(path string, edge int) ([]byte, error) {
	dir, err := os.MkdirTemp("", "lumiwerk-mpv-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	core := mpv.New()
	if core == nil {
		return nil, mpv.LoadError()
	}
	defer core.TerminateDestroy()
	for key, value := range map[string]string{
		"config": "no", "terminal": "no", "audio": "no", "sub": "no",
		"vo": "image", "vo-image-format": "jpg", "vo-image-jpeg-quality": "80",
		"vo-image-outdir": dir, "frames": "1", "start": "10%", "hr-seek": "no",
		"vf": fmt.Sprintf("lavfi=[scale=w=%d:h=%d:force_original_aspect_ratio=decrease]", edge, edge),
	} {
		if err := core.SetOptionString(key, value); err != nil {
			return nil, fmt.Errorf("option %s: %w", key, err)
		}
	}
	if err := core.Initialize(); err != nil {
		return nil, err
	}
	if err := core.Command([]string{"loadfile", path}); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ev := core.WaitEvent(0.5)
		if ev == nil {
			continue
		}
		if ev.EventID == mpv.EventEnd || ev.EventID == mpv.EventShutdown {
			break
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.jpg"))
	if len(files) == 0 {
		return nil, errors.New("no frame written")
	}
	return os.ReadFile(files[0])
}
