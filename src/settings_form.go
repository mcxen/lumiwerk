// Shared settings form components. Every page is a list of setGroup
// sections whose rows share one 140px label column and one control column,
// so spacing and alignment match across pages.
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

const (
	setLabelW = 140
	setRowH   = 32
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
func setRow(c *ui.Context, label string, control func()) {
	ui.Row(c).FillWidth().MinHeight(setRowH).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(setLabelW).FontSize(13).TextColor(c.Theme().TextMuted).SingleLine().Shrink(0)
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

// setCheck is one entry of a setChecks group.
type setCheck struct {
	key, label string
	value      *bool
}

// setChecks is a multi-select row: checkboxes wrapped under one label.
func setChecks(c *ui.Context, label string, checks []setCheck) bool {
	changed := false
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Text(c, label).Width(setLabelW).FontSize(13).TextColor(c.Theme().TextMuted).SingleLine().Shrink(0).Padding(7, 0)
		ui.Row(c).Grow(1).MinWidth(0).Wrap().Gap(0).Children(func() {
			for _, check := range checks {
				ui.Row(c).MinHeight(setRowH).Width(150).AlignItems(ui.Center).Children(func() {
					if ui.Checkbox(c.Key(check.key), check.value, check.label).Changed() {
						changed = true
					}
				})
			}
		})
	})
	return changed
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
