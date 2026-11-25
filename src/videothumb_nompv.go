//go:build android || ios || nompv

package main

import "errors"

func mpvVideoThumb(path string, edge int) ([]byte, error) {
	return nil, errors.New("libmpv support not compiled in")
}
