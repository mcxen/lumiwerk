package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
)

// probeExif fills the camera and shooting-time fields of info from the
// file's EXIF metadata. Failures are silent: a file without EXIF simply
// leaves the fields empty.
func probeExif(path string, info *mediaInfo) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".tiff", ".tif", ".heic", ".heif", ".png", ".webp":
	default:
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		return // most images carry no EXIF at all
	}
	if cam, err := x.Get(exif.Make); err == nil {
		if s, err := cam.StringVal(); err == nil {
			info.Camera = strings.TrimSpace(s)
		}
	}
	if model, err := x.Get(exif.Model); err == nil {
		if s, err := model.StringVal(); err == nil {
			info.Model = strings.TrimSpace(s)
		}
	}
	if dt, err := x.DateTime(); err == nil {
		info.Created = dt.Format("2006-01-02 15:04:05")
	}
}
