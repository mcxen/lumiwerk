//go:build !darwin

package main

import "errors"

// newMPVBackend is the stub for platforms without libmpv support.
// Playback falls back to the FFmpeg backend via newFallbackPlayback.
func newMPVBackend() (playbackBackend, error) {
	return nil, errors.New("libmpv playback only supported on darwin")
}
