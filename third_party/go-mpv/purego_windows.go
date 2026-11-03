//go:build (!cgo || nocgo) && windows

package mpv

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// libnames are the libmpv DLL names used by various distributions, tried in order.
var libnames = []string{"libmpv-2.dll", "libmpv.dll", "mpv-2.dll", "mpv-1.dll"}

// Locale categories for SetLocale (Windows CRT values).
const (
	LCAll     = 0
	LCNumeric = 4
)

// loadLibrary loads the dll, trying dirs first, then the bare names
// (which LoadLibrary resolves against the standard search order). It
// returns an error instead of panicking when no library can be found.
func loadLibrary(dirs []string) (uintptr, error) {
	var names []string
	for _, dir := range dirs {
		for _, name := range libnames {
			names = append(names, dir+"\\"+name)
		}
	}
	names = append(names, libnames...)
	var err error
	for _, name := range names {
		var handle windows.Handle
		handle, err = windows.LoadLibrary(name)
		if err == nil {
			return uintptr(handle), nil
		}
	}
	return 0, fmt.Errorf("cannot load libmpv (tried %v): %w", names, err)
}

// crtnames are the C runtime DLLs that export malloc/free, tried in order.
var crtnames = []string{"ucrtbase.dll", "msvcrt.dll"}

// memLibrary loads the C runtime to resolve malloc/free, which libmpv does not export.
func memLibrary() (uintptr, error) {
	var err error
	for _, name := range crtnames {
		var handle windows.Handle
		handle, err = windows.LoadLibrary(name)
		if err == nil {
			return uintptr(handle), nil
		}
	}
	return 0, fmt.Errorf("cannot load C runtime (tried %v): %w", crtnames, err)
}
