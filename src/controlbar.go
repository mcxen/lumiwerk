package main

import (
	"github.com/egoist/mygo/ui"
)

// The floating control bar is one design shared by the video transport and
// the image viewer strip: same shell, same icon button, same chip, same
// slider. Only the content slots differ. docs/control-bar-demo.html is the
// visual reference for these tokens.

const (
	barRadius    = 12
	barGap       = 6
	barBtnSize   = 32
	barIconSize  = 18
	barBtnRadius = 6
	barChipRad   = 5
	barFontSize  = 12

	// mediaBarClearance keeps overlays such as rating stars above the bar.
	mediaBarClearance = 74
)

var (
	barBackground = ui.RGBA(16, 16, 18, 0.82)
	barShadow     = ui.RGBA(0, 0, 0, 0.45)
	barFG         = ui.RGB(245, 245, 247)
	barFGDim      = ui.RGB(180, 180, 185)
	barAccent     = ui.RGB(10, 132, 255)
	barPrimaryBG  = ui.RGBA(255, 255, 255, 0.14)
	barSepColor   = ui.RGBA(255, 255, 255, 0.16)
)

// controlBarShell is the bottom-attached floating container. Callers add the
// rows with Children and may fade it with Opacity.
func controlBarShell(c *ui.Context, key string) ui.Element {
	bar := ui.Column(c.Key(key)).Absolute().Left(8).Right(8).Bottom(8).
		Padding(8, 12, 10).Gap(barGap).Radius(barRadius).
		Background(barBackground)
	bar.Shadow(0, -6, 18, 0, barShadow)
	return bar
}

// controlBarRow is one horizontal line inside the shell.
func controlBarRow(c *ui.Context) ui.Element {
	return ui.Row(c).FillWidth().Gap(barGap).AlignItems(ui.Center)
}

// barSpacer pushes the following controls to the right edge.
func barSpacer(c *ui.Context) {
	ui.Box(c).Grow(1)
}

// barSeparator is the thin divider between control groups.
func barSeparator(c *ui.Context) {
	ui.Box(c).Width(1).Height(16).Margin(0, 4).Background(barSepColor)
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
		Padding(4, 8).Radius(barChipRad).TextColor(barFG).Children(func() {
		ui.Text(c, text).FontSize(barFontSize).TextColor(barFG)
	})
}
