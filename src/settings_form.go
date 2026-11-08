// Shared settings form components. A page is a column of setGroup cards;
// each card is a stack of rows with the label (and an optional hint) on the
// left and the control on the right, separated by hairlines. Every control
// below is built on setRow, so spacing and alignment match across pages.
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

const (
	setRowH     = 44
	setRowPadX  = 16
	setCardR    = 12
	setCtrlW    = 168
	setTextW    = 220
	setSliderW  = 160
	setCheckW   = 120
	setSegH     = 28
	setSegPadX  = 12
	setTitleGap = 8
)

// setRowIndex counts rows in the open group so every row after the first
// gets a divider above it. The UI runs on one goroutine.
var setRowIndex int

// setGroup is a titled card: a small muted title above a rounded surface that
// holds the rows.
func setGroup(c *ui.Context, title string, rows func()) {
	pal := settingsPaletteFor(c)
	ui.Column(c).FillWidth().Gap(setTitleGap).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(pal.textMuted).Padding(0, 0, 0, 4)
		}
		setRowIndex = 0
		raised(ui.Column(c).FillWidth().Radius(setCardR).Background(pal.card), pal).Clip().Children(rows)
	})
}

// setDivider is the hairline between two rows of a card.
func setDivider(c *ui.Context) {
	pal := settingsPaletteFor(c)
	if setRowIndex > 0 {
		ui.Box(c).FillWidth().Height(1).Margin(0, setRowPadX).Background(pal.divider)
	}
	setRowIndex++
}

// setRowHint is one row. hint is a quiet second line under the label.
func setRowHint(c *ui.Context, label, hint string, control func()) {
	pal := settingsPaletteFor(c)
	setDivider(c)
	ui.Row(c).FillWidth().MinHeight(setRowH).Padding(6, setRowPadX).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			if label != "" {
				ui.Text(c, label).FontSize(13).TextColor(pal.text)
			}
			if hint != "" {
				ui.Text(c, hint).FontSize(11.5).TextColor(pal.textMuted)
			}
		})
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

func setRow(c *ui.Context, label string, control func()) {
	setRowHint(c, label, "", control)
}

// setNote is a full-width quiet line at the bottom of a card.
func setNote(c *ui.Context, text string, warn bool) {
	pal := settingsPaletteFor(c)
	color := pal.textMuted
	if warn {
		color = pal.warn
	}
	setDivider(c)
	ui.Text(c, text).FontSize(12).TextColor(color).Padding(8, setRowPadX)
}

// setToggle is a switch row; it reports a change this frame.
func setToggle(c *ui.Context, key, label string, on *bool) bool {
	return setToggleHint(c, key, label, "", on)
}

func setToggleHint(c *ui.Context, key, label, hint string, on *bool) bool {
	before := *on
	setRowHint(c, label, hint, func() {
		settingsToggle(c, key, on, settingsPaletteFor(c)).Label(label)
	})
	return *on != before
}

// setSelect is a dropdown row.
func setSelect[T comparable](c *ui.Context, key, label string, value *T, options []T, name func(T) string) bool {
	before := *value
	setRow(c, label, func() {
		settingsPopup(c, key, value, options, name, setCtrlW, settingsPaletteFor(c))
	})
	return *value != before
}

// settingsSegment is a pill-shaped single-choice control. The chosen segment
// is filled with the accent; every segment is reachable by its label.
func settingsSegment(c *ui.Context, key string, selected *int, labels []string) bool {
	pal := settingsPaletteFor(c)
	seg := ui.SegmentedBase(c.Key(key), selected, len(labels))
	seg.Track.AlignItems(ui.Stretch).Padding(2).Gap(2).Radius(setSegH/2 + 2).Background(pal.chip).Children(func() {
		for i, text := range labels {
			on := *selected == i
			s := seg.Segment(i).Height(setSegH-4).Justify(ui.Center).AlignItems(ui.Center).
				Padding(0, setSegPadX).Radius(setSegH/2 - 2).Label(text)
			if on {
				s.Background(pal.accent)
			}
			s.Children(func() {
				color := pal.text
				if on {
					color = c.Theme().AccentText
				}
				ui.Text(c, text).FontSize(12.5).TextColor(color).SingleLine()
			})
		}
	})
	return seg.Track.Changed()
}

// setSegmented is a row of two or more segments over an index.
func setSegmented(c *ui.Context, key, label string, selected *int, labels ...string) bool {
	before := *selected
	setRow(c, label, func() { settingsSegment(c, key, selected, labels) })
	return *selected != before
}

// setChoice is a single-choice row over any comparable value.
func setChoice[T comparable](c *ui.Context, key, label string, value *T, options []T, names []string) bool {
	return setChoiceHint(c, key, label, "", value, options, names)
}

func setChoiceHint[T comparable](c *ui.Context, key, label, hint string, value *T, options []T, names []string) bool {
	idx := 0
	for i, option := range options {
		if option == *value {
			idx = i
		}
	}
	before := idx
	setRowHint(c, label, hint, func() { settingsSegment(c, key, &idx, names) })
	if idx != before && idx >= 0 && idx < len(options) {
		*value = options[idx]
		return true
	}
	return false
}

// setCheck is one entry of a setChecks block.
type setCheck struct {
	key, label string
	value      *bool
}

// setChecks is a row holding an even grid of check boxes.
func setChecks(c *ui.Context, label string, checks []setCheck) bool {
	changed := false
	pal := settingsPaletteFor(c)
	setDivider(c)
	ui.Column(c).FillWidth().Padding(8, setRowPadX, 10).Gap(2).Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(13).TextColor(pal.text).Padding(0, 0, 4, 0)
		}
		ui.Row(c).FillWidth().Wrap().Gap(4).Children(func() {
			for _, check := range checks {
				ui.Row(c).MinHeight(32).Width(setCheckW).AlignItems(ui.Center).Children(func() {
					if settingsCheckbox(c, check.key, check.value, check.label).Changed() {
						changed = true
					}
				})
			}
		})
	})
	return changed
}

// settingsCheckbox draws a check box with a visible border and accent fill
// when selected, so dark mode stays readable without touching mygo.
func settingsCheckbox(c *ui.Context, key string, checked *bool, label string) ui.Element {
	pal := settingsPaletteFor(c)
	t := c.Theme()
	row := ui.CheckboxBase(c.Key(key), checked).Gap(8).AlignItems(ui.Center).FocusRing(false).Label(label)
	on := *checked
	row.Children(func() {
		box := ui.Box(c).Size(16, 16).Radius(4).Shrink(0)
		if on {
			box.Background(pal.accent).Border(1, pal.accent)
		} else {
			border := pal.border
			if pal.dark {
				border = pal.text.Alpha(0.45)
			}
			box.Background(pal.control).Border(1.5, border)
		}
		box.DrawOver(func(p *ui.Painter, r ui.Rect) {
			if on {
				var path ui.Path
				path.MoveTo(r.X+r.W*0.22, r.Y+r.H*0.52).
					LineTo(r.X+r.W*0.42, r.Y+r.H*0.71).
					LineTo(r.X+r.W*0.78, r.Y+r.H*0.31)
				p.StrokePath(&path, 1.5, t.AccentText)
			}
		})
		if label != "" {
			ui.Text(c, label).FontSize(13).TextColor(pal.text).SingleLine()
		}
	})
	return row
}

// setButton is a row holding one button; it reports a click.
func setButton(c *ui.Context, key, label, text string) bool {
	return setButtonHint(c, key, label, "", text)
}

func setButtonHint(c *ui.Context, key, label, hint, text string) bool {
	clicked := false
	setRowHint(c, label, hint, func() {
		clicked = ui.Button(c.Key(key), text).Clicked()
	})
	return clicked
}

// setText is a text field row with an optional trailing button; it reports
// a click on that button.
func setText(c *ui.Context, key, label string, value *string, placeholder, button string, password bool) bool {
	return setTextHint(c, key, label, "", value, placeholder, button, password)
}

func setTextHint(c *ui.Context, key, label, hint string, value *string, placeholder, button string, password bool) bool {
	clicked := false
	setRowHint(c, label, hint, func() {
		in := ui.TextInput(c.Key(key), value).Width(setTextW).Placeholder(placeholder).Label(label)
		if password {
			in.Password()
		} else if *value != "" {
			in.Tooltip(*value)
		}
		if button != "" {
			clicked = ui.Button(c.Key(key+"-btn"), button).Clicked()
		}
	})
	return clicked
}

// setSlider is a slider row with its value printed after it.
func setSlider(c *ui.Context, key, label string, value *float64, lo, hi float64, unit string) bool {
	before := *value
	setRow(c, label, func() {
		ui.Slider(c.Key(key), value, lo, hi).Width(setSliderW).Label(label)
		ui.Text(c, fmt.Sprintf("%d%s", int(*value), unit)).FontSize(12).
			TextColor(settingsPaletteFor(c).textMuted).Width(64)
	})
	return *value != before
}
