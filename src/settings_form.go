// Shared settings form components. Every page is a list of setGroup
// sections whose rows share one label column and one control column,
// so spacing and alignment match across pages.
package main

import (
	"fmt"
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"
)

const (
	setLabelW = 112
	setRowH   = 32
	setCheckW = 128
)

// setGroup is a titled section: a bold 13px title, then its rows.
func setGroup(c *ui.Context, title string, rows func()) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(c.Theme().Text).Padding(0, 0, 2, 0)
		}
		rows()
	})
}

// setRow is one label/control line; every component below is built on it.
// Labels use primary text so they stay the sweep-reading entry; values can
// sit quieter in the control column.
func setRow(c *ui.Context, label string, control func()) {
	ui.Row(c).FillWidth().MinHeight(setRowH).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(setLabelW).FontSize(13).FontWeight(500).TextColor(c.Theme().Text).SingleLine().Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

// setToggle is a switch row; it reports a change this frame.
func setToggle(c *ui.Context, key, label string, on *bool) bool {
	before := *on
	setRow(c, label, func() {
		settingsToggle(c, key, on, settingsPaletteFor(c)).Label(label)
	})
	return *on != before
}

// setSelect is a dropdown row.
func setSelect[T comparable](c *ui.Context, key, label string, value *T, options []T, name func(T) string) bool {
	before := *value
	setRow(c, label, func() {
		settingsPopup(c, key, value, options, name, 180, settingsPaletteFor(c))
	})
	return *value != before
}

// setChoice is a single-choice row of radio buttons for two or three options.
func setChoice[T comparable](c *ui.Context, key, label string, value *T, options []T, names []string) bool {
	before := *value
	setRow(c, label, func() {
		for i, option := range options {
			ui.Radio(c.Key(fmt.Sprintf("%s-%d", key, i)), value, option, names[i])
		}
	})
	return *value != before
}

// setSegmented is a two-or-more segment control on one row.
func setSegmented(c *ui.Context, key, label string, selected *int, labels ...string) bool {
	before := *selected
	setRow(c, label, func() {
		ui.Segmented(c.Key(key), selected, labels...).Label(label)
	})
	return *selected != before
}

// setCheck is one entry of a setChecks group.
type setCheck struct {
	key, label string
	value      *bool
}

// setChecks is a multi-select block: a label above an even checkbox grid.
func setChecks(c *ui.Context, label string, checks []setCheck) bool {
	changed := false
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(c.Theme().Text).Padding(0, 0, 2, 0)
		}
		ui.Row(c).FillWidth().Wrap().Gap(4).Children(func() {
			for _, check := range checks {
				ui.Row(c).MinHeight(setRowH).Width(setCheckW).AlignItems(ui.Center).Children(func() {
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
	clicked := false
	setRow(c, label, func() {
		clicked = ui.Button(c.Key(key), text).Clicked()
	})
	return clicked
}

// setText is a text field row with an optional trailing button; it reports
// a click on that button.
func setText(c *ui.Context, key, label string, value *string, placeholder, button string, password bool) bool {
	clicked := false
	setRow(c, label, func() {
		in := ui.TextInput(c.Key(key), value).Grow(1).MinWidth(160).MaxWidth(320).Placeholder(placeholder)
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
		ui.Slider(c.Key(key), value, lo, hi).Width(200).Label(label)
		ui.Text(c, fmt.Sprintf("%d%s", int(*value), unit)).FontSize(12).
			TextColor(c.Theme().TextMuted).Width(56)
	})
	return *value != before
}

// setThumbSlider is the default-thumbnail control: slider with tick marks,
// an editable px field, and a live preview square sized to the value.
func setThumbSlider(c *ui.Context, key, label string, value *float64, lo, hi float64) bool {
	before := *value
	pal := settingsPaletteFor(c)
	setRow(c, label, func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Slider(c.Key(key), value, lo, hi).Width(220).Label(label)
				ui.NumberInput(c.Key(key+"-num"), value, lo, hi, 1).Width(64).Label(label + " 数值")
				ui.Text(c, "px").FontSize(12).TextColor(pal.textMuted)
			})
			ui.Row(c).Width(220).Justify(ui.SpaceBetween).Children(func() {
				for _, tick := range []int{128, 256, 384, 512} {
					ui.Text(c, strconv.Itoa(tick)).FontSize(10).TextColor(pal.textMuted)
				}
			})
			ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
				side := float32(math.Max(24, math.Min(*value/4, 80)))
				preview := ui.Box(c.Key(key+"-preview")).Size(side, side).Radius(6).
					Background(pal.chip).Border(1, pal.border).
					Shadow(0, 1, 2, 0, pal.cardShadow)
				_ = preview
				ui.Text(c, fmt.Sprintf("%d × %d", int(*value), int(*value))).
					FontSize(11).TextColor(pal.textMuted).Padding(0, 0, 4, 0)
			})
		})
	})
	if *value < lo {
		*value = lo
	}
	if *value > hi {
		*value = hi
	}
	return *value != before
}
