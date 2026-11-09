// Small schematic figures for the settings pages. They share one visual
// language so every page reads the same: a soft stage, one miniature window,
// the same four photo tints, an accent ring for "this is what you picked",
// and a round chevron between a cause and its effect. Each figure redraws
// from the live value. They are built from boxes and text, never from the
// user's files, and use no emoji so they look alike on every platform.
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

const (
	figW = 200 // the stage; the caption takes the rest of the row
	figH = 88
)

// Photo tints: pairs of colors for the gradient of a thumbnail.
var figPhotos = [][2]ui.Color{
	{ui.RGB(137, 196, 255), ui.RGB(86, 141, 235)},  // sky
	{ui.RGB(255, 190, 140), ui.RGB(240, 120, 110)}, // sunset
	{ui.RGB(150, 225, 190), ui.RGB(70, 170, 140)},  // mint
	{ui.RGB(200, 170, 250), ui.RGB(130, 100, 215)}, // violet
}

// setFigure is a card row: the stage on the left, a caption on the right.
func setFigure(c *ui.Context, pal *settingsPalette, caption string, draw func()) {
	setDivider(c)
	ui.Row(c).FillWidth().Padding(12, setRowPadX).Gap(16).AlignItems(ui.Center).Children(func() {
		stage := ui.Row(c).Width(figW).Height(figH).Radius(10).Padding(10).Gap(8).
			AlignItems(ui.Center).Justify(ui.Center).Clip().
			Gradient(pal.accent.Alpha(0.10), pal.accent.Alpha(0.03), 135).
			Border(1, pal.accent.Alpha(0.14))
		stage.Children(draw)
		ui.Text(c, caption).FontSize(12).TextColor(pal.textMuted).Grow(1).MinWidth(0)
	})
}

// figPhoto is a thumbnail with a soft gradient; ringed marks the pick.
func figPhoto(c *ui.Context, pal *settingsPalette, w, h float32, tint int, ringed bool) {
	p := figPhotos[tint%len(figPhotos)]
	t := ui.Box(c).Width(w).Height(h).Radius(4).Gradient(p[0], p[1], 150)
	if ringed {
		t.Border(2, pal.accent)
	}
}

// figWindow is the one miniature window: a title strip with three dots over
// a body that the caller fills.
func figWindow(c *ui.Context, pal *settingsPalette, w, h float32, dark, ringed bool, body func()) {
	bg, bar, dot := ui.RGB(252, 252, 254), ui.RGB(232, 233, 238), ui.RGB(196, 198, 206)
	edge := ui.RGBA(0, 0, 0, 0.10)
	if dark {
		bg, bar, dot = ui.RGB(36, 37, 42), ui.RGB(54, 55, 62), ui.RGB(96, 98, 108)
		edge = ui.RGBA(255, 255, 255, 0.14)
	}
	win := ui.Column(c).Width(w).Height(h).Radius(6).Background(bg).Clip().
		Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.16))
	if ringed {
		win.Border(2, pal.accent)
	} else {
		win.Border(1, edge)
	}
	win.Children(func() {
		ui.Row(c).FillWidth().Height(9).Padding(0, 5).Gap(2).AlignItems(ui.Center).Background(bar).Children(func() {
			for i := 0; i < 3; i++ {
				ui.Box(c).Width(3).Height(3).Radius(2).Background(dot)
			}
		})
		ui.Row(c).FillWidth().Grow(1).Padding(5).Gap(4).AlignItems(ui.Center).Children(body)
	})
}

// figThen is the round chevron between a cause and its effect.
func figThen(c *ui.Context, pal *settingsPalette) {
	ui.Column(c).Width(18).Height(18).Radius(9).Background(pal.accent.Alpha(0.16)).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Text(c, "›").FontSize(13).FontWeight(700).TextColor(pal.accent)
	})
}

// figPill is a small rounded label; tone is the text and dot color.
func figPill(c *ui.Context, pal *settingsPalette, text string, tone ui.Color, filled bool) {
	row := ui.Row(c).Height(22).Padding(0, 8).Gap(4).Radius(11).AlignItems(ui.Center)
	if filled {
		row.Background(tone.Alpha(0.16))
	} else {
		row.Background(ui.RGBA(255, 255, 255, 0.7)).Border(1, tone.Alpha(0.4))
	}
	row.Children(func() {
		ui.Text(c, text).FontSize(11).FontWeight(600).TextColor(tone).SingleLine()
	})
}

// figGlyph is a round badge holding one symbol: a state such as stop or loop.
func figGlyph(c *ui.Context, pal *settingsPalette, glyph string, tone ui.Color) {
	ui.Column(c).Width(28).Height(28).Radius(14).Background(tone.Alpha(0.16)).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Text(c, glyph).FontSize(13).FontWeight(700).TextColor(tone)
	})
}

// figureTheme shows the three looks, ringing the chosen one.
func figureTheme(c *ui.Context, pal *settingsPalette, theme string) {
	caption := map[string]string{
		"system": "跟随 macOS 的浅色或深色外观。",
		"light":  "始终使用浅色界面。",
		"dark":   "始终使用深色界面，看图片更专注。",
	}[theme]
	setFigure(c, pal, caption, func() {
		figWindow(c, pal, 48, 56, false, theme == "light", func() {
			figPhoto(c, pal, 14, 14, 0, false)
			figPhoto(c, pal, 14, 14, 1, false)
		})
		figWindow(c, pal, 48, 56, true, theme == "dark", func() {
			figPhoto(c, pal, 14, 14, 0, false)
			figPhoto(c, pal, 14, 14, 1, false)
		})
		// "system" is both looks at once.
		half := ui.Row(c).Width(48).Height(56).Radius(6).Clip().
			Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.16))
		if theme == "system" {
			half.Border(2, pal.accent)
		} else {
			half.Border(1, ui.RGBA(0, 0, 0, 0.10))
		}
		half.Children(func() {
			ui.Box(c).Grow(1).FillHeight().Background(ui.RGB(252, 252, 254))
			ui.Box(c).Grow(1).FillHeight().Background(ui.RGB(36, 37, 42))
		})
	})
}

// figureClose shows what the red close button does.
func figureClose(c *ui.Context, pal *settingsPalette, action string) {
	quit := action == closeQuit
	caption := "关闭窗口后继续在菜单栏托盘运行，再点图标即可回来。"
	if quit {
		caption = "关闭最后一个窗口就退出，播放和后台任务一并停止。"
	}
	setFigure(c, pal, caption, func() {
		figWindow(c, pal, 64, 56, false, false, func() {
			figPhoto(c, pal, 20, 20, 2, false)
		})
		figThen(c, pal)
		if quit {
			figGlyph(c, pal, "✕", pal.danger)
			return
		}
		figPill(c, pal, "托盘", pal.accent, true)
	})
}

// figureEnter shows what Enter does to the selected file.
func figureEnter(c *ui.Context, pal *settingsPalette, opens bool) {
	caption := "选中文件按 Enter：进入重命名，输入新名字。"
	if opens {
		caption = "选中文件按 Enter：直接打开，再按一次关闭。"
	}
	setFigure(c, pal, caption, func() {
		figWindow(c, pal, 64, 56, false, false, func() {
			figPhoto(c, pal, 20, 20, 0, false)
			figPhoto(c, pal, 20, 20, 1, true)
		})
		figThen(c, pal)
		if opens {
			figPhoto(c, pal, 52, 52, 1, false)
			return
		}
		figPill(c, pal, "IMG_01|", pal.accent, false)
	})
}

// figurePlayEnd shows what happens when a video reaches its end.
func figurePlayEnd(c *ui.Context, pal *settingsPalette, mode string) {
	var caption, glyph string
	switch mode {
	case playbackEndNext:
		caption, glyph = "播完自动播放列表里的下一个视频。", "›"
	case playbackEndLoop:
		caption, glyph = "播完回到开头，一直循环。", "↻"
	default:
		caption, glyph = "播完停在最后一帧，等你操作。", "■"
	}
	setFigure(c, pal, caption, func() {
		ui.Column(c).Gap(6).Children(func() {
			figPhoto(c, pal, 72, 44, 3, false)
			ui.Box(c).Width(72).Height(4).Radius(2).Background(pal.accent)
		})
		figThen(c, pal)
		if mode == playbackEndNext {
			figPhoto(c, pal, 44, 44, 0, false)
			return
		}
		figGlyph(c, pal, glyph, pal.accent)
	})
}

// figureTypes shows which kinds of file the browser lists.
func figureTypes(c *ui.Context, pal *settingsPalette, view viewPreferences) {
	kinds := []struct {
		name string
		tint int
		on   bool
	}{
		{"图", 0, view.AllTypes || view.Images},
		{"RAW", 3, view.AllTypes || view.RAW},
		{"视", 1, view.AllTypes || view.Videos},
		{"包", 2, view.AllTypes || view.Archives},
		{"装", 0, view.AllTypes || view.Installers},
	}
	shown := 0
	for _, k := range kinds {
		if k.on {
			shown++
		}
	}
	caption := fmt.Sprintf("文件夹里只列出亮着的 %d 类，其余隐藏。", shown)
	if view.AllTypes {
		caption = "不过滤，文件夹里的所有文件都会列出。"
	}
	setFigure(c, pal, caption, func() {
		for _, k := range kinds {
			tile := ui.Column(c).Width(28).Height(38).Radius(5).AlignItems(ui.Center).Justify(ui.Center)
			color := ui.RGB(255, 255, 255)
			if k.on {
				p := figPhotos[k.tint]
				tile.Gradient(p[0], p[1], 150)
			} else {
				tile.Background(pal.text.Alpha(0.06))
				color = pal.textFaint
			}
			tile.Children(func() {
				ui.Text(c, k.name).FontSize(10).FontWeight(700).TextColor(color).SingleLine()
			})
		}
	})
}

// figureZipBook shows the automatic password attempts for an encrypted ZIP.
// saved is how many passwords are stored; the last one is drawn as the match.
func figureZipBook(c *ui.Context, pal *settingsPalette, saved int) {
	caption := "还没有保存密码：打开加密压缩包时会直接询问。"
	if saved > 0 {
		caption = fmt.Sprintf("解压加密压缩包时，依次试这 %d 个密码；都不对才弹出询问。", saved)
	}
	setFigure(c, pal, caption, func() {
		figPill(c, pal, "ZIP", pal.accent, false)
		if saved == 0 {
			figThen(c, pal)
			figPill(c, pal, "询问密码", pal.warn, true)
			return
		}
		shown := saved
		if shown > 3 {
			shown = 3
		}
		for i := 0; i < shown; i++ {
			if i == shown-1 {
				figPill(c, pal, "✓", pal.success, true)
			} else {
				figPill(c, pal, "✗", pal.danger, true)
			}
		}
	})
}
