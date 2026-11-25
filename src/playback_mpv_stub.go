//go:build android || ios || nompv

package main

import "errors"

// newMPVBackend is the stub for targets without desktop libmpv support
// (mobile targets or builds opting out via -tags nompv). Desktop builds
// compile libmpv in by default and load it lazily on first playback.
func newMPVBackend() (playbackBackend, error) {
	return nil, errors.New("libmpv support not compiled in")
}
