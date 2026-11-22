package main

import "github.com/egoist/mygo"

// windowChrome hides the macOS title bar and keeps the traffic lights over
// the top bar (NSWindow full-size content view with a transparent title
// bar, via MyGo's TitleBarHiddenInset), vertically centered with the 44px
// seamless card toolbar (cardGap + 22 = 30).
func windowChrome(o mygo.WindowOptions) mygo.WindowOptions {
	o.TitleBarStyle = mygo.TitleBarHiddenInset
	o.TrafficLightPosition = &mygo.Point{X: 16, Y: 23}
	return o
}
