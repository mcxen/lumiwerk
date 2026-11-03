//go:build (!cgo || nocgo) && unix && !darwin

package mpv

import (
	"fmt"

	"github.com/ebitengine/purego"
)

// libnames are the libmpv shared object names, tried in order.
var libnames = []string{"libmpv.so", "libmpv.so.2"}

// Locale categories for SetLocale (glibc/musl values).
const (
	LCAll     = 6
	LCNumeric = 1
)

// loadLibrary loads the so, trying dirs first, then the bare names. It
// returns an error instead of panicking when no library can be found.
func loadLibrary(dirs []string) (uintptr, error) {
	var names []string
	for _, dir := range dirs {
		for _, name := range libnames {
			names = append(names, dir+"/"+name)
		}
	}
	names = append(names, libnames...)
	var err error
	for _, name := range names {
		var handle uintptr
		handle, err = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			return handle, nil
		}
	}
	return 0, fmt.Errorf("cannot load libmpv (tried %v): %w", names, err)
}

// memLibrary returns the handle for malloc/free; libmpv's libc dependency exposes them.
func memLibrary() (uintptr, error) {
	return libmpv, nil
}
