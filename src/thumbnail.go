package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// thumbEdge is the longest side of a grid thumbnail. It tracks the slider
// but stays small enough that a folder of photos does not decode originals.
func thumbEdge(slider float64) int {
	edge := int(slider * 2)
	if edge < 384 {
		edge = 384
	}
	if edge > 1024 {
		edge = 1024
	}
	return edge
}

// thumbBand keeps nearby slider positions on one decode size so pinch-zoom
// does not rebuild every thumbnail for a one-pixel change.
func thumbBand(edge int) int {
	switch {
	case edge < 448:
		return 384
	case edge < 640:
		return 512
	case edge < 896:
		return 768
	default:
		return 1024
	}
}

func thumbCacheKey(path string, edge int) string {
	return fmt.Sprintf("%s#thumb#%d", path, thumbBand(edge))
}

// largeMaxSide caps any dimension of the 1:1 bitmap, so one photo cannot
// exhaust memory.
const largeMaxSide = 12000

// largeOversample keeps headroom for zooming before the sharp resample
// takes over: the opened bitmap holds this many pixels per screen pixel.
const largeOversample = 2

// largeStageEdge is the decode edge for the opened bitmap: the larger
// stage side times largeOversample, clamped to what a Retina window
// needs. Sources past it are resampled by the platform pipeline, which
// opens measurably faster than decoding every original pixel. stage is
// the larger stage side from a.largeSize(); 0 falls back to a default.
func largeStageEdge(stage float32) int {
	long := float64(stage)
	if long <= 0 {
		long = 1280
	}
	edge := int(long * largeOversample)
	if edge < 1600 {
		edge = 1600
	}
	if edge > largeMaxSide {
		edge = largeMaxSide
	}
	return edge
}

// thumbBytes builds a JPEG thumbnail. Images go through the vfs pipeline
// (sips on macOS, in-process Go elsewhere); videos take one scaled frame.
// Callers fall back to the original bytes if this fails.
func thumbBytes(path string, video bool, edge int) ([]byte, error) {
	return mediaThumbJPEG(path, video, edge)
}

// decodeLargeBitmap decodes the original for 1:1 zoom. edge is the stage
// edge the bitmap is for; sources far past it are resampled by the
// platform pipeline instead of decoding every original pixel. Images
// larger than largeMaxSide are always capped so one photo cannot
// exhaust memory.
func decodeLargeBitmap(path string, edge int) (*ui.Bitmap, error) {
	data, err := decodeLargeImage(path, edge)
	if err != nil {
		return nil, err
	}
	if bmp, err := ui.DecodeBitmap(data); err == nil {
		return bmp, nil
	}
	data, err = imageResampleJPEG(path, largeMaxSide, "82")
	if err != nil {
		return nil, err
	}
	return ui.DecodeBitmap(data)
}
