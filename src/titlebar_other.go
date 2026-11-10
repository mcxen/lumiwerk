//go:build !darwin

package main

import "github.com/egoist/mygo"

// windowChrome hides the system title bar so the backdrop and the card
// reach the window's edge; Lumiwerk draws the controls (chrome.go) and the
// bars drag the window. Alt and F10 show the menus, whose shortcuts work
// all the same.
func windowChrome(o mygo.WindowOptions) mygo.WindowOptions {
	o.TitleBarStyle = mygo.TitleBarHidden
	o.HideWindowButtons = true
	o.AutoHideMenuBar = true
	return o
}
