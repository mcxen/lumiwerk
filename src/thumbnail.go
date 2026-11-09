package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// thumbEdge is the longest side of a grid thumbnail. It tracks the slider
// but stays small enough that a folder of photos does not decode originals.
func thumbEdge(slider float64) int {
	edge := int(slider * 2)
	if edge < 256 {
		edge = 256
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
	case edge <= 256:
		return 256
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

// thumbCachePrefix starts every in-memory thumbnail key of one file.
func thumbCachePrefix(path string) string { return path + "#thumb#" }

// thumbCacheKey names one decoded thumbnail in memory. It carries the
// file's size and modification time, as the disk cache's key does, so a file
// rewritten under the same name misses the old entry and is decoded again.
// Callers pass the values the item already holds; nothing here touches disk.
func thumbCacheKey(path string, edge int, mtime, size int64) string {
	return fmt.Sprintf("%s%d#%d#%d", thumbCachePrefix(path), thumbBand(edge), mtime, size)
}

// itemThumbKey is thumbCacheKey for a listed item.
func itemThumbKey(item *mediaItem, edge int) string {
	return thumbCacheKey(item.Path, edge, item.ModTime, item.Size)
}

// folderCoverKey identifies one stacked folder cover in the shared
// thumbnail cache. digest is a hash of the first media names and mtimes,
// so an edit inside the folder misses this entry and the cover is built
// again. The size band matches thumbCacheKey.
func folderCoverKey(path, digest string, edge int) string {
	return fmt.Sprintf("%s#cover#%d#%s", path, thumbBand(edge), digest)
}

// folderCoverBusyKey is the single in-flight claim for a folder. It uses
// the thumbnail cache's busy set, so a fast scroll cannot queue the same
// folder twice and there is no second cache.
func folderCoverBusyKey(path string) string {
	return path + "#cover-busy"
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
	return mediaThumbJPEG(realPathOr(path), video, edge)
}

// decodeLargeBitmap decodes the original for 1:1 zoom. edge is the stage
// edge the bitmap is for; sources far past it are resampled by the
// platform pipeline instead of decoding every original pixel. Images
// larger than largeMaxSide are always capped so one photo cannot
// exhaust memory.
func decodeLargeBitmap(path string, edge int) (*ui.Bitmap, error) {
	path, err := realPath(path)
	if err != nil {
		return nil, err
	}
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
