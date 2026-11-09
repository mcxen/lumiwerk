// Kero-style settings controls: switch and popup select. Adapted from
// github.com/egoist/kero (MIT) via GodGo.
package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	settingsPopupH    = 28
	settingsMenuPad   = 5
	settingsToggleOnW = 16
	settingsMenuItemH = 24
)

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func settingsToggle(c *ui.Context, key string, on *bool, pal *settingsPalette) ui.Element {
	sw := ui.SwitchBase(c.Key(key), on).Size(38, 22).Radius(11).Padding(2).Shrink(0)
	track := pal.text.Alpha(0.14)
	if *on {
		track = pal.accent
	}
	sw.Background(sw.AnimateColor("track", track, ui.Animation{Duration: 150 * time.Millisecond}))
	x := sw.AnimateTo("knob", b2f(*on)*settingsToggleOnW, ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.15}})
	sw.Children(func() {
		ui.Box(c).Size(18, 18).Radius(9).Background(ui.Hex("#ffffff")).Translate(x, 0).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.22))
	})
	return sw
}

// settingsPopup renders a Kero-style select: a pill trigger and a popup
// menu of Item rows. The chosen option shows a check; clicking a row
// selects it and closes.
func settingsPopup[T comparable](c *ui.Context, key string, value *T, options []T, label func(T) string, width float32, pal *settingsPalette) {
	sel := ui.SelectBase(c.Key(key), value)
	trig := sel.Trigger.MaxWidth(width).Height(settingsPopupH).Padding(0, 3, 0, 10).Gap(6).
		Radius(settingsPopupH / 2).AlignItems(ui.Center).Cursor(ui.CursorDefault)
	open := sel.Open()
	tint := pal.text.Alpha(0.065)
	if pal.dark {
		tint = ui.RGBA(255, 255, 255, 0.09)
	}
	bg, disc := ui.Transparent, tint
	switch {
	case open || trig.Pressed():
		bg, disc = tint.Mix(pal.text, 0.06), ui.Transparent
	case trig.Hovered():
		bg, disc = tint, ui.Transparent
	}
	anim := ui.Animation{Duration: 120 * time.Millisecond}
	trig.Background(trig.AnimateColor("bg", bg, anim))
	trig.Children(func() {
		ui.Text(c, label(*value)).FontSize(13).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		d := ui.Box(c.Key("disc")).Size(22, 22).Radius(11).Center().Shrink(0)
		d.Background(d.AnimateColor("bg", disc, anim)).Children(func() {
			ui.Icon(c, lucideChevronDown).Size(16, 16).TextColor(pal.text.Alpha(0.85))
		})
	})
	sel.Popup(func(panel ui.Element) {
		settingsMenuPanel(panel, pal)
		ui.Scroll(c.Key("items")).MaxHeight(280).MinWidth(width - 2*settingsMenuPad).Gap(0).Children(func() {
			for _, option := range options {
				option := option
				// Each option is one horizontal row: check, then name.
				// Without Row() and an explicit Children the check stacked
				// over the name and every row grew to about 52pt.
				item := sel.Item(option)
				item.Row().Height(settingsMenuItemH).Shrink(0).Padding(0, 10).Gap(8).AlignItems(ui.Center).Radius(6)
				item.Children(func() {
					if option == *value {
						ui.Icon(c, lucideCheck).Size(13, 13).Shrink(0).TextColor(pal.text)
					} else {
						ui.Box(c).Size(13, 13).Shrink(0)
					}
					ui.Text(c, label(option)).FontSize(13).TextColor(pal.text).SingleLine()
				})
			}
		})
	})
}

func settingsMenuPanel(panel ui.Element, pal *settingsPalette) {
	panel.Radius(10).Background(pal.card).Padding(settingsMenuPad).
		Shadow(0, 1, 3, 0, pal.cardShadow).
		Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.14))
	if pal.dark {
		panel.Border(1, pal.text.Alpha(0.12))
	}
}
