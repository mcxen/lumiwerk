package main

// The browser window's look, after github.com/egoist/kero (MIT): a wash of
// color under the window, the sidebar written straight on it, and the
// content in one rounded card that floats above it. A wash is a name in
// the preferences (View.Wash); the card and its controls stay neutral so
// the pictures keep their own colors.

import (
	"github.com/egoist/mygo/ui"
)

const (
	// cardGap is the wash that shows around the content card.
	cardGap = 8
	// cardRadius rounds the card.
	cardRadius = 14
	// defaultWash is the wash of a profile that has not chosen one.
	defaultWash = "paper"
)

// shellPalette colors the browser window's chrome in one appearance.
type shellPalette struct {
	dark bool

	card       ui.Color // the content card
	surface    ui.Color // bars and controls inside the card
	hover      ui.Color
	pressed    ui.Color
	divider    ui.Color
	text       ui.Color
	textMuted  ui.Color
	textFaint  ui.Color
	rowHover   ui.Color // sidebar rows, on the wash
	rowCurrent ui.Color
	cardEdge   ui.Color
	shadow     ui.Color
}

var shellLight = shellPalette{
	card:       ui.Hex("#ffffff"),
	surface:    ui.Hex("#f5f5f8"),
	hover:      ui.Hex("#eeeef3"),
	pressed:    ui.Hex("#e4e4ea"),
	divider:    ui.RGBA(0, 0, 0, 0.06),
	text:       ui.Hex("#1d1d1f"),
	textMuted:  ui.Hex("#6e6e78"),
	textFaint:  ui.Hex("#a3a3ad"),
	rowHover:   ui.RGBA(0, 0, 0, 0.045),
	rowCurrent: ui.RGBA(255, 255, 255, 0.85),
	cardEdge:   ui.RGBA(0, 0, 0, 0.07),
	shadow:     ui.RGBA(20, 20, 50, 0.10),
}

var shellDark = shellPalette{
	dark:       true,
	card:       ui.Hex("#232328"),
	surface:    ui.Hex("#2b2b31"),
	hover:      ui.Hex("#34343b"),
	pressed:    ui.Hex("#3d3d45"),
	divider:    ui.RGBA(255, 255, 255, 0.07),
	text:       ui.Hex("#ececf0"),
	textMuted:  ui.Hex("#a9a9b4"),
	textFaint:  ui.Hex("#83838f"),
	rowHover:   ui.RGBA(255, 255, 255, 0.06),
	rowCurrent: ui.RGBA(255, 255, 255, 0.11),
	cardEdge:   ui.RGBA(255, 255, 255, 0.08),
	shadow:     ui.RGBA(0, 0, 0, 0.40),
}

func shellPaletteFor(c *ui.Context) *shellPalette {
	if c.Theme().Dark {
		return &shellDark
	}
	return &shellLight
}

// wash is a window's backdrop in both appearances: gradient stops from top
// to bottom and the glow from its top right.
type wash struct {
	light, dark         []string
	lightGlow, darkGlow ui.Color
}

// washNames lists the washes in the order settings offers them.
var washNames = []string{"paper", "mist", "meadow", "dusk", "tide", "apricot", "plain"}

// washLabels are the names settings shows, in the same order.
var washLabels = []string{"纸", "雾", "草地", "暮色", "潮汐", "杏", "纯色"}

var washTable = map[string]wash{
	"paper": {
		light: []string{"#f6f6f7", "#f7f7f8", "#f8f8f8"}, lightGlow: ui.RGBA(255, 255, 255, 0),
		dark: []string{"#141416", "#151517", "#161618"}, darkGlow: ui.RGBA(90, 120, 200, 0.10),
	},
	"mist": {
		light: []string{"#e3e9f4", "#ebeff6", "#f1f2f3", "#f2f1ec"}, lightGlow: ui.RGBA(210, 225, 248, 0.75),
		dark: []string{"#0c1628", "#0d1827", "#111b23"}, darkGlow: ui.RGBA(46, 91, 166, 0.30),
	},
	"meadow": {
		light: []string{"#e2eee5", "#ebf2e7", "#f3f2e2", "#f5f0d6"}, lightGlow: ui.RGBA(214, 240, 222, 0.75),
		dark: []string{"#081c0d", "#101d0c", "#1b1c0b"}, darkGlow: ui.RGBA(6, 113, 50, 0.30),
	},
	"dusk": {
		light: []string{"#e4dbf3", "#efdeee", "#f6e3dc", "#f7e6cf"}, lightGlow: ui.RGBA(246, 206, 222, 0.80),
		dark: []string{"#190f2e", "#260f24", "#2d140b"}, darkGlow: ui.RGBA(125, 65, 138, 0.30),
	},
	"tide": {
		light: []string{"#dbebef", "#e3eff1", "#ecf3f0", "#f2f2e8"}, lightGlow: ui.RGBA(194, 232, 238, 0.75),
		dark: []string{"#001b22", "#001e1f", "#0b1e1b"}, darkGlow: ui.RGBA(0, 112, 128, 0.30),
	},
	"apricot": {
		light: []string{"#f5e1d6", "#f8e7dc", "#f8eedf", "#f6f0da"}, lightGlow: ui.RGBA(252, 212, 194, 0.78),
		dark: []string{"#2a1207", "#291507", "#25190a"}, darkGlow: ui.RGBA(151, 62, 22, 0.30),
	},
	"plain": {
		light: []string{"#eceef4", "#eceef4"}, lightGlow: ui.RGBA(0, 0, 0, 0),
		dark: []string{"#141418", "#141418"}, darkGlow: ui.RGBA(0, 0, 0, 0),
	},
}

// washOf returns the named wash, the default one for an unknown name.
func washOf(name string) wash {
	if w, ok := washTable[name]; ok {
		return w
	}
	return washTable[defaultWash]
}

// washGradients are the base and the glow of a wash in one appearance.
func washGradients(name string, dark bool) (base, glow ui.LinearGradient) {
	w := washOf(name)
	hexes, g := w.light, w.lightGlow
	if dark {
		hexes, g = w.dark, w.darkGlow
	}
	stops := make([]ui.GradientStop, len(hexes))
	for i, h := range hexes {
		stops[i] = ui.GradientStop{Color: ui.Hex(h), At: float32(i) / float32(len(hexes)-1)}
	}
	base = ui.LinearGradient{Angle: 180, Oklab: true, Stops: stops}
	glow = ui.LinearGradient{Angle: 225, Oklab: true, Stops: []ui.GradientStop{{Color: g, At: 0}, {Color: g.Alpha(0), At: 0.5}}}
	return base, glow
}

// shellTheme retints the stock widgets inside the card to the palette.
func shellTheme(c *ui.Context, pal *shellPalette) *ui.Theme {
	t := *c.Theme()
	t.Background = pal.card
	t.Surface = pal.surface
	t.SurfaceHover = pal.hover
	t.SurfacePressed = pal.pressed
	t.Border = pal.divider
	return &t
}

// paintBackdrop washes the window: the base on the root, the glow on row,
// which holds the sidebar and the card. Over the window's material the base
// is a little transparent so the desktop shows through.
func (a *appState) paintBackdrop(c *ui.Context, row ui.Element) {
	base, glow := washGradients(a.Profiles.View.Wash, c.Theme().Dark)
	if c.Vibrancy() {
		stops := make([]ui.GradientStop, len(base.Stops))
		for i, st := range base.Stops {
			stops[i] = ui.GradientStop{Color: st.Color.Alpha(0.95), At: st.At}
		}
		base.Stops = stops
	}
	c.Root().LinearGradient(base)
	row.LinearGradient(glow)
}

// card gives the content its floating surface: rounded, clipped, with the
// light's soft shadow or the dark's hairline.
func cardSurface(e ui.Element, pal *shellPalette) ui.Element {
	e = e.Radius(cardRadius).Background(pal.card).Clip()
	if pal.dark {
		return e.Border(1, pal.cardEdge).Shadow(0, 4, 12, -2, pal.shadow)
	}
	return e.Shadow(0, 1, 2, 0, pal.shadow.Alpha(0.5)).Shadow(0, 4, 14, -2, pal.shadow)
}
