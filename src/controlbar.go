package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The floating control bar is one design shared by the video transport and
// the image viewer strip: same shell, same icon button, same chip, same
// slider. Only the content slots differ. docs/control-bar-demo.html is the
// visual reference for these tokens.

const (
	barHeight    = 48 // one 32pt button row plus 8pt of padding above and below
	barRadius    = barHeight / 2
	barGap       = 4
	barBtnSize   = 32
	barIconSize  = 18
	barBtnRadius = barBtnSize / 2 // round buttons sit inside the round capsule
	barChipRad   = 13
	barFontSize  = 12
	barBottom    = 16 // gap between the capsule and the window edge

	// barMaxWidth caps the video capsule so a wide window still gets a
	// floating capsule and not a bar; barSideMargin is the least gap to
	// the window sides.
	barMaxWidth   = 680
	barSideMargin = 20
)

// When the bar shows. The default follows the macOS player habit: it comes
// with the pointer and leaves when the pointer rests.
const (
	barAuto   = "auto"   // appears when the pointer moves, fades after barIdleAuto
	barHover  = "hover"  // appears only when the pointer is near the bottom edge
	barAlways = "always" // stays up

	barIdleAuto  = 2 * time.Second
	barIdleHover = 800 * time.Millisecond // grace so leaving the zone does not flicker
	// barHotZone is the strip along the bottom edge that wakes the bar: the
	// capsule, its gap to the edge, and room above it.
	barHotZone = 120
)

// normalizeBarMode maps an unknown or empty stored value to the default.
func normalizeBarMode(mode string) string {
	switch mode {
	case barHover, barAlways:
		return mode
	default:
		return barAuto
	}
}

var (
	barBackground = ui.RGBA(28, 28, 32, 0.78)
	barBorder     = ui.RGBA(255, 255, 255, 0.12)
	barShadow     = ui.RGBA(0, 0, 0, 0.38)
	barFG         = ui.RGB(245, 245, 247)
	barFGDim      = ui.RGB(180, 180, 185)
	barAccent     = ui.RGB(10, 132, 255)
	barPrimaryBG  = ui.RGBA(255, 255, 255, 0.16)
	barSepColor   = ui.RGBA(255, 255, 255, 0.18)
)

// controlBarWidth is the video capsule width for a window of windowW points.
func controlBarWidth(windowW float32) float32 {
	return min(barMaxWidth, max(240, windowW-2*barSideMargin))
}

// controlBar is the floating macOS-style capsule at the bottom centre of the
// stage: fully rounded, translucent, with a hairline edge and a soft shadow.
// width 0 sizes it to its controls; otherwise it is that wide, which is what
// a timeline needs. The row it floats in lets clicks through, so only the
// capsule itself takes the pointer. It returns the capsule so callers can
// read Hovered and fade it with Opacity.
func (a *appState) controlBar(c *ui.Context, key string, width float32, content func()) ui.Element {
	var bar ui.Element
	// Over the gallery the capsule rides above its progress bar and strip,
	// and glides there when the strip comes up.
	lift := float32(0)
	if a.galleryActive() {
		lift = a.galleryLift
	}
	dock := ui.Row(c.Key(key + "-dock")).Absolute().Left(0).Right(0).
		Justify(ui.Center).PassThrough()
	dock.Bottom(barBottom + dock.AnimateTo("lift", lift, ui.Animation{Duration: 160 * time.Millisecond}))
	dock.Children(func() {
		bar = ui.Row(c.Key(key)).Height(barHeight).Padding(0, 10).Gap(barGap).
			AlignItems(ui.Center).Radius(barRadius).Background(barBackground).
			Border(1, barBorder).Shrink(0)
		if width > 0 {
			bar.Width(width)
		}
		bar.Shadow(0, 8, 24, 0, barShadow)
		bar.Children(content)
	})
	return bar
}

// controlBarRow is the horizontal line of controls inside the capsule.
func controlBarRow(c *ui.Context) ui.Element {
	return ui.Row(c).Grow(1).Gap(barGap).AlignItems(ui.Center)
}

// barSeparator is the thin divider between control groups.
func barSeparator(c *ui.Context) {
	ui.Box(c).Width(1).Height(18).Margin(0, 4).Background(barSepColor)
}

// barText is a static label such as a clock or a zoom percentage.
func barText(c *ui.Context, text string, dim bool) {
	color := barFG
	if dim {
		color = barFGDim
	}
	ui.Text(c, text).FontSize(barFontSize).TextColor(color).SingleLine()
}

// barIconButton is the standard 32pt icon button.
func barIconButton(c *ui.Context, key, label, tooltip string, icon *ui.SVG) ui.Element {
	return ui.ButtonBase(c.Key(key)).Label(label).Tooltip(tooltip).Size(barBtnSize, barBtnSize).
		Padding(7).Radius(barBtnRadius).TextColor(barFG).Children(func() {
		ui.Icon(c, icon).Size(barIconSize, barIconSize)
	})
}

// barToggleButton is an icon button that shows an active state in accent.
func barToggleButton(c *ui.Context, key, label, tooltip string, icon *ui.SVG, on bool) ui.Element {
	btn := ui.ButtonBase(c.Key(key)).Label(label).Tooltip(tooltip).Size(barBtnSize, barBtnSize).
		Padding(7).Radius(barBtnRadius).TextColor(barFG)
	if on {
		btn = btn.TextColor(barAccent)
	}
	return btn.Children(func() {
		ui.Icon(c, icon).Size(barIconSize, barIconSize)
	})
}

// barPrimaryButton is the single main action of a bar, drawn on a light
// fill. Put it first in the row.
func barPrimaryButton(c *ui.Context, key, label, tooltip string, icon *ui.SVG) ui.Element {
	return ui.ButtonBase(c.Key(key)).Label(label).Tooltip(tooltip).Size(barBtnSize, barBtnSize).
		Padding(7).Radius(barBtnRadius).Background(barPrimaryBG).TextColor(barFG).Children(func() {
		ui.Icon(c, icon).Size(barIconSize, barIconSize)
	})
}

// barChip is a compact text button for a value such as speed or fit mode.
// label is the accessible name; text is what is drawn.
func barChip(c *ui.Context, key, label, tooltip, text string) ui.Element {
	return ui.ButtonBase(c.Key(key)).Label(label).Tooltip(tooltip).
		Padding(4, 10).Radius(barChipRad).TextColor(barFG).Children(func() {
		ui.Text(c, text).FontSize(barFontSize).TextColor(barFG)
	})
}

// notePlayerPointer is fed the pointer position inside the stage (h is the
// stage height). It records whether the pointer is in the bottom hot zone,
// and in auto mode restarts the idle clock when the pointer really moves. A
// still pointer, including the first sample, does not count.
func (a *appState) notePlayerPointer(x, y, h float32) {
	a.playerInZone = h > 0 && y >= h-barHotZone
	if !a.playerPtrSeen {
		a.playerPtrSeen = true
		a.playerPtrX, a.playerPtrY = x, y
		return
	}
	if abs32(x-a.playerPtrX) <= 0.5 && abs32(y-a.playerPtrY) <= 0.5 {
		return
	}
	a.playerPtrX, a.playerPtrY = x, y
	if normalizeBarMode(a.Profiles.ViewerBar) == barAuto {
		a.playerMoved = time.Now()
	}
}

// revealControlBar shows the bar for a moment, as when a new picture or clip
// opens, so the user sees it is there before it settles.
func (a *appState) revealControlBar() {
	a.playerMoved = time.Now()
	a.playerPtrSeen = false
	a.playerInZone = false
}

// controlBarShown decides whether the bar is up.
//   - always: yes.
//   - the pointer is on the bar, the bar holds focus, a menu is open or a
//     scrub is under way: yes, and the idle clock restarts.
//   - the pointer is in the bottom hot zone: yes, in every mode.
//   - hold: something the user is waiting on (a paused clip); auto mode only.
//   - otherwise it stays up until the idle time since the last wake passes.
//
// playerMoved zero means the bar stays visible and schedules no wake, which
// is how tests render it.
func (a *appState) controlBarShown(c *ui.Context, hover, focus, busy, hold bool) bool {
	now := c.Now()
	active := hover || focus || busy || a.playerMenu || a.playerInZone
	shown, restart, wake := barVisible(normalizeBarMode(a.Profiles.ViewerBar), a.playerMoved, now, active, hold)
	if restart {
		a.playerMoved = now
	}
	if wake > 0 {
		c.After(wake)
	}
	return shown
}

// barVisible is the pure rule behind controlBarShown. active means the user
// is using the bar or has the pointer in the hot zone; hold means a state
// worth keeping the bar for in auto mode. restart asks to move the idle clock
// to now; wake is how long until the answer can change, or 0.
func barVisible(mode string, moved, now time.Time, active, hold bool) (shown, restart bool, wake time.Duration) {
	if mode == barAlways || moved.IsZero() {
		return true, false, 0
	}
	if active || (hold && mode == barAuto) {
		return true, true, 0
	}
	idle := barIdleAuto
	if mode == barHover {
		idle = barIdleHover
	}
	if remain := idle - now.Sub(moved); remain > 0 {
		return true, false, remain
	}
	return false, false, 0
}

// fadeControlBar fades the capsule in or out. It is hidden only after the
// fade ends, so a click on the picture is not swallowed by an invisible bar.
func fadeControlBar(bar ui.Element, key string, shown bool) {
	target := float32(0)
	if shown {
		target = 1
	}
	opacity := bar.AnimateTo(key, target, ui.Animation{Duration: 180 * time.Millisecond})
	bar.Opacity(opacity)
	if !shown && opacity < 0.04 {
		bar.Hidden(true)
	}
}
