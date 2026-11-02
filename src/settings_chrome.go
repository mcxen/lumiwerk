// Kero-style palette and elevation for the settings window. Colors adapted
// from github.com/egoist/kero (MIT) via GodGo, so the settings window sits
// on the same washes and row treatment as GodGo's.
package main

import "github.com/egoist/mygo/ui"

type settingsPalette struct {
	dark bool

	text      ui.Color
	textMuted ui.Color
	textFaint ui.Color

	rowHover    ui.Color
	rowSelected ui.Color

	card       ui.Color
	cardShadow ui.Color

	control      ui.Color
	controlHover ui.Color

	divider ui.Color
	chip    ui.Color

	danger ui.Color
	warn   ui.Color
	accent ui.Color
}

var settingsLight = settingsPalette{
	text:      ui.Hex("#1d1d1f"),
	textMuted: ui.Hex("#6e6e78"),
	textFaint: ui.Hex("#a3a3ad"),

	rowHover:    ui.RGBA(0, 0, 0, 0.04),
	rowSelected: ui.RGBA(0, 0, 0, 0.07),

	card:       ui.Hex("#ffffff"),
	cardShadow: ui.RGBA(0, 0, 0, 0.07),

	control:      ui.Hex("#ffffff"),
	controlHover: ui.Hex("#f7f7f9"),

	divider: ui.RGBA(0, 0, 0, 0.06),
	chip:    ui.RGBA(0, 0, 0, 0.045),

	danger: ui.Hex("#d93a35"),
	warn:   ui.Hex("#c27b0f"),
	accent: ui.Hex("#3b6cf6"),
}

var settingsDark = settingsPalette{
	dark: true,

	text:      ui.Hex("#ececf0"),
	textMuted: ui.Hex("#a9a9b4"),
	textFaint: ui.Hex("#83838f"),

	rowHover:    ui.RGBA(255, 255, 255, 0.05),
	rowSelected: ui.RGBA(255, 255, 255, 0.10),

	card:       ui.Hex("#232328"),
	cardShadow: ui.RGBA(0, 0, 0, 0.35),

	control:      ui.Hex("#404047"),
	controlHover: ui.Hex("#4a4a52"),

	divider: ui.RGBA(255, 255, 255, 0.07),
	chip:    ui.RGBA(255, 255, 255, 0.07),

	danger: ui.Hex("#ff6961"),
	warn:   ui.Hex("#f0a33a"),
	accent: ui.Hex("#6b93ff"),
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
// inside the settings window match Kero's treatment.
func settingsTheme(c *ui.Context, pal *settingsPalette) *ui.Theme {
	t := *c.Theme()
	t.Accent = pal.accent
	t.Surface = pal.control
	t.SurfaceHover = pal.controlHover
	t.Text = pal.text
	t.TextMuted = pal.textMuted
	return &t
}
