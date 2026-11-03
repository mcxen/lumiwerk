// Kero-style palette and elevation for the settings window. Colors adapted
// from github.com/egoist/kero (MIT) via GodGo, so the settings window sits
// on the same washes and row treatment as GodGo's.
package main

import "github.com/egoist/mygo/ui"

// settingsPalette is the semantic palette of the design spec: the light
// and dark swatches of section 2.1, plus the row and card washes the
// settings chrome is drawn with.
type settingsPalette struct {
	dark bool

	background     ui.Color
	surface        ui.Color
	surfaceHover   ui.Color
	surfacePressed ui.Color
	border         ui.Color
	borderLight    ui.Color

	text      ui.Color
	textMuted ui.Color
	textFaint ui.Color

	accent        ui.Color
	accentHover   ui.Color
	accentPressed ui.Color

	danger  ui.Color
	warn    ui.Color
	success ui.Color
	info    ui.Color

	rowHover    ui.Color
	rowSelected ui.Color

	card       ui.Color
	cardShadow ui.Color

	control      ui.Color
	controlHover ui.Color

	divider ui.Color
	chip    ui.Color
}

var settingsLight = settingsPalette{
	background:     ui.Hex("#ffffff"),
	surface:        ui.Hex("#f5f5f5"),
	surfaceHover:   ui.Hex("#ececec"),
	surfacePressed: ui.Hex("#e0e0e0"),
	border:         ui.Hex("#d1d1d1"),
	borderLight:    ui.Hex("#e8e8e8"),

	text:      ui.Hex("#1c1c1c"),
	textMuted: ui.Hex("#6e6e6e"),
	textFaint: ui.Hex("#a8a8a8"),

	accent:        ui.Hex("#007aff"),
	accentHover:   ui.Hex("#0051d5"),
	accentPressed: ui.Hex("#003e9e"),

	danger:  ui.Hex("#ff3b30"),
	warn:    ui.Hex("#ff9500"),
	success: ui.Hex("#34c759"),
	info:    ui.Hex("#5ac8fa"),

	rowHover:    ui.Hex("#ececec"),
	rowSelected: ui.Hex("#e0e0e0"),

	card:       ui.Hex("#ffffff"),
	cardShadow: ui.RGBA(0, 0, 0, 0.07),

	control:      ui.Hex("#ffffff"),
	controlHover: ui.Hex("#f5f5f5"),

	divider: ui.Hex("#e8e8e8"),
	chip:    ui.Hex("#f5f5f5"),
}

var settingsDark = settingsPalette{
	dark: true,

	background:     ui.Hex("#1c1c1e"),
	surface:        ui.Hex("#2c2c2e"),
	surfaceHover:   ui.Hex("#3a3a3c"),
	surfacePressed: ui.Hex("#48484a"),
	border:         ui.Hex("#38383a"),
	borderLight:    ui.Hex("#2c2c2e"),

	text:      ui.Hex("#ffffff"),
	textMuted: ui.Hex("#98989d"),
	textFaint: ui.Hex("#5e5e60"),

	accent:        ui.Hex("#0a84ff"),
	accentHover:   ui.Hex("#409cff"),
	accentPressed: ui.Hex("#0077ed"),

	danger:  ui.Hex("#ff453a"),
	warn:    ui.Hex("#ff9f0a"),
	success: ui.Hex("#30d158"),
	info:    ui.Hex("#64d2ff"),

	rowHover:    ui.Hex("#3a3a3c"),
	rowSelected: ui.Hex("#48484a"),

	card:       ui.Hex("#2c2c2e"),
	cardShadow: ui.RGBA(0, 0, 0, 0.35),

	control:      ui.Hex("#2c2c2e"),
	controlHover: ui.Hex("#3a3a3c"),

	divider: ui.Hex("#38383a"),
	chip:    ui.RGBA(255, 255, 255, 0.07),
}

func settingsPaletteFor(c *ui.Context) *settingsPalette {
	if c.Theme().Dark {
		return &settingsDark
	}
	return &settingsLight
}

// raised draws Kero's surfaces: in the light a soft shadow, in the dark a
// lit hairline border.
func raised(e ui.Element, pal *settingsPalette) ui.Element {
	if pal.dark {
		return e.Border(1, pal.text.Alpha(0.12)).
			Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.2)).
			Shadow(0, 4, 6, -3, ui.RGBA(0, 0, 0, 0.24))
	}
	return e.Shadow(0, 1, 2, 0, pal.cardShadow).Shadow(0, 1, 6, 0, pal.cardShadow.Alpha(0.5))
}

// settingsTheme retints the context theme to the palette so stock widgets
// inside the settings window match the spec: 13px body text on a 4px
// spacing grid, semantic colors for stock controls and their states.
func settingsTheme(c *ui.Context, pal *settingsPalette) *ui.Theme {
	t := *c.Theme()
	t.Background = pal.background
	t.Surface = pal.control
	t.SurfaceHover = pal.controlHover
	t.SurfacePressed = pal.surfacePressed
	t.Border = pal.border
	t.Text = pal.text
	t.TextMuted = pal.textMuted
	t.Accent = pal.accent
	t.AccentHover = pal.accentHover
	t.AccentPressed = pal.accentPressed
	t.Danger = pal.danger
	t.Warning = pal.warn
	t.Success = pal.success
	t.Selection = pal.surfacePressed
	t.Focus = pal.accent
	t.FontSize = 13
	t.Spacing = 4
	return &t
}
