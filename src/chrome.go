package main

// Window controls across platforms, after kero's platform.go. macOS keeps
// its traffic lights over the sidebar. Windows and Linux hide the system
// title bar so the wash and the card reach the window's edge, and Lumiwerk
// draws minimize, maximize and close itself at the right of the toolbar.
// Where the desktop gives windows no buttons (a tiling compositor) or the
// user turned them off, none are drawn; LUMIWERK_WINDOW_CONTROLS=1 or 0
// forces them shown or hidden.

import (
	"os"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// ownControls tells that Lumiwerk draws the window controls itself.
var ownControls = runtime.GOOS != "darwin"

// showControls tells that they show (ownControls). Main thread only.
var showControls = ownControls

// followWindowButtons sets showControls and keeps it in step with the
// desktop's button layout. Call it once the app is ready.
func followWindowButtons() {
	if !ownControls {
		return
	}
	update := func() { showControls = controlsWanted(os.Getenv, mygo.Theme.WindowButtons()) }
	update()
	mygo.Theme.OnUpdated(update)
}

// controlsWanted tells whether to draw the controls: where the desktop's
// title bars have buttons, unless the environment says otherwise.
func controlsWanted(getenv func(string) string, buttons mygo.WindowButtons) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("LUMIWERK_WINDOW_CONTROLS"))) {
	case "1", "true", "yes", "on", "show":
		return true
	case "0", "false", "no", "off", "hide":
		return false
	}
	return !buttons.Empty()
}

const (
	controlW = 28
	// controlsWidth is the three buttons, two apart, with their margin.
	controlsWidth = 3*controlW + 2*2 + 6
)

var (
	minimizeIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 8h9"/></svg>`))
	maximizeIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="3.5" width="9" height="9" rx="1.5"/></svg>`))
	restoreIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="5.5" width="7" height="7" rx="1.2"/><path d="M5.5 3.5h5.5a1.5 1.5 0 0 1 1.5 1.5v5.5"/></svg>`))
)

// windowControls draws the minimize, maximize and close buttons at the end
// of a bar. It draws nothing on macOS, under a compositor without buttons
// or in a window that does not exist yet (tests).
func (a *appState) windowControls(c *ui.Context, pal *shellPalette) {
	if !showControls || a.Window == nil {
		return
	}
	w := a.Window
	ui.Row(c.Key("window-controls")).Gap(2).AlignItems(ui.Center).Shrink(0).Margin(0, 0, 0, 6).Children(func() {
		controlButton(c, "minimize", minimizeIcon, "最小化", pal, w.Minimize)
		icon, label := maximizeIcon, "最大化"
		maximized := w.IsMaximized()
		if maximized {
			icon, label = restoreIcon, "还原"
		}
		// The system's maximize button for the pointer brings Windows 11's
		// snap layouts as it rests there, so it carries no tooltip.
		m := ui.ButtonBase(c.Key("maximize")).Size(controlW, 26).Radius(6).Center().TextColor(pal.textMuted).Label(label).MaximizeButton()
		if m.Pressed() {
			m.Background(pal.pressed)
		} else if m.Hovered() {
			m.Background(pal.hover).TextColor(pal.text)
		}
		m.Children(func() { ui.Icon(c, icon).Size(16, 16) })
		if m.Clicked() {
			if maximized {
				w.Unmaximize()
			} else {
				w.Maximize()
			}
		}
		b := ui.ButtonBase(c.Key("close-window")).Size(controlW, 26).Radius(6).Center().TextColor(pal.textMuted).Label("关闭").Tooltip("关闭")
		if b.Hovered() || b.Pressed() {
			b.Background(ui.Hex("#d93a35")).TextColor(ui.RGB(255, 255, 255))
		}
		b.Children(func() { ui.Icon(c, lucideClose).Size(16, 16) })
		if b.Clicked() {
			w.Close()
		}
	})
}

func controlButton(c *ui.Context, key string, icon *ui.SVG, label string, pal *shellPalette, fn func()) {
	b := ui.ButtonBase(c.Key(key)).Size(controlW, 26).Radius(6).Center().TextColor(pal.textMuted).Label(label).Tooltip(label)
	if b.Pressed() {
		b.Background(pal.pressed)
	} else if b.Hovered() {
		b.Background(pal.hover).TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, icon).Size(16, 16) })
	if b.Clicked() {
		fn()
	}
}
