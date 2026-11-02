//go:build !android && !ios && !mpvbackend

package main

import "errors"

// newMPVBackend is the stub for builds without the mpvbackend tag. The
// go-mpv package loads libmpv in its package init and panics when the
// library is missing, so it is compiled in only when building a release
// that bundles libmpv; everything else falls back to FFmpeg playback.
func newMPVBackend() (playbackBackend, error) {
	return nil, errors.New("libmpv support not compiled in")
}
