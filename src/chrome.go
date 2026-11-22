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
	controlW = 44
	controlH = 36
	// controlsWidth is the three flush top-right window buttons.
	controlsWidth = 3 * controlW
)

var (
	minimizeIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 8h9"/></svg>`))
	maximizeIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="3.5" width="9" height="9" rx="1.2"/></svg>`))
	restoreIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"><rect x="3.2" y="5.2" width="7.2" height="7.2" rx="1.0"/><path d="M5.4 3.5h5.8a1.3 1.3 0 0 1 1.3 1.3v5.8"/></svg>`))
	closeWindowIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"><path d="M3.8 3.8l8.4 8.4M12.2 3.8l-8.4 8.4"/></svg>`))
)

// showWindowControls tells whether the self-drawn top-right window caption
// buttons are active for this frame.
func (a *appState) showWindowControls() bool {
	return showControls && (a.Window == nil || !a.Window.IsFullScreen())
}

// windowControlsOverlay draws the minimize, maximize and close buttons anchored
// flush at the window's top-right corner outside toolbarView, so they remain
// pinned across single-tab, multi-tab, collapsed-sidebar and gallery views.
func (a *appState) windowControlsOverlay(c *ui.Context, pal *shellPalette) {
	if !a.showWindowControls() {
		return
	}
	w := a.Window
	darkStage := pal.dark || a.galleryActive()
	fg := ui.Hex("#1d1d1f")
	hoverBg := ui.RGBA(0, 0, 0, 0.06)
	pressedBg := ui.RGBA(0, 0, 0, 0.11)
	if darkStage {
		fg = ui.Hex("#ececf0")
		hoverBg = ui.RGBA(255, 255, 255, 0.10)
		pressedBg = ui.RGBA(255, 255, 255, 0.16)
	}
	ui.Box(c.Key("window-controls-overlay")).Absolute().Top(0).Right(0).Children(func() {
		ui.Row(c.Key("window-controls")).Gap(0).AlignItems(ui.Center).Shrink(0).Children(func() {
			captionButton(c, "minimize", minimizeIcon, "最小化", fg, hoverBg, pressedBg, func() {
				if w != nil {
					w.Minimize()
				}
			})
			icon, label := maximizeIcon, "最大化"
			maximized := w != nil && w.IsMaximized()
			if maximized {
				icon, label = restoreIcon, "还原"
			}
			// The system's maximize button for the pointer brings Windows 11's
			// snap layouts as it rests there, so it carries no tooltip.
			m := ui.ButtonBase(c.Key("maximize")).Size(controlW, controlH).Radius(0).Center().TextColor(fg).Label(label).MaximizeButton()
			if m.Pressed() {
				m.Background(pressedBg)
			} else if m.Hovered() {
				m.Background(hoverBg)
			}
			m.Children(func() { ui.Icon(c, icon).Size(12, 12) })
			if m.Clicked() && w != nil {
				if maximized {
					w.Unmaximize()
				} else {
					w.Maximize()
				}
			}
			b := ui.ButtonBase(c.Key("close-window")).Size(controlW, controlH).Radius(0).Center().TextColor(fg).Label("关闭").Tooltip("关闭")
			if b.Pressed() {
				b.Background(ui.Hex("#c4101f")).TextColor(ui.RGB(255, 255, 255))
			} else if b.Hovered() {
				b.Background(ui.Hex("#e81123")).TextColor(ui.RGB(255, 255, 255))
			}
			b.Children(func() { ui.Icon(c, closeWindowIcon).Size(12, 12) })
			if b.Clicked() && w != nil {
				w.Close()
			}
		})
	})
}

func captionButton(c *ui.Context, key string, icon *ui.SVG, label string, fg, hoverBg, pressedBg ui.Color, fn func()) {
	b := ui.ButtonBase(c.Key(key)).Size(controlW, controlH).Radius(0).Center().TextColor(fg).Label(label).Tooltip(label)
	if b.Pressed() {
		b.Background(pressedBg)
	} else if b.Hovered() {
		b.Background(hoverBg)
	}
	b.Children(func() { ui.Icon(c, icon).Size(12, 12) })
	if b.Clicked() {
		fn()
	}
}
