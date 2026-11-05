//go:build !darwin

package main

import "github.com/egoist/mygo"

// windowChrome keeps the system title bar outside macOS.
func windowChrome(o mygo.WindowOptions) mygo.WindowOptions { return o }
