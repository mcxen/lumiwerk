// The 快捷键 settings page: every rebindable action with its key. Click a
// key to record a new one; Esc cancels, conflicts are refused.
package main

import (
	"github.com/egoist/mygo/ui"
)

// keyCapture is the action waiting for a new key on the shortcuts page.
var keyCapture struct {
	id       string
	conflict string
}

func (a *appState) settingsKeysPage(c *ui.Context, pal *settingsPalette) {
	page := ui.Column(c.Key("keys-page")).FillWidth().Gap(0).Focusable().FocusRing(false)
	if keyCapture.id != "" {
		page.Focus()
	}
	page.HandleInput(func(ev ui.InputEvent) bool {
		if keyCapture.id == "" || ev.Kind != ui.InputKeyDown {
			return false
		}
		if ev.Key == ui.KeyUnknown {
			return false
		}
		a.captureKey(keyCapture.id, ev.Mods, ev.Key)
		return true
	})
	page.Children(func() {
		setGroup(c, "", func() {
			for _, s := range shortcutActions {
				s := s
				setRow(c, s.label, func() {
					if keyCapture.id == s.id && keyCapture.conflict != "" {
						ui.Text(c, "与「"+keyCapture.conflict+"」冲突").FontSize(12).TextColor(pal.warn)
					}
					if a.binding(s.id) != s.def && ui.Button(c.Key("reset-"+s.id), "恢复").Clicked() {
						a.setBinding(s.id, s.def)
					}
					text := bindingLabel(a.binding(s.id))
					if keyCapture.id == s.id {
						text = "按下新快捷键…"
					}
					if ui.Button(c.Key("key-"+s.id), text).MinWidth(120).Clicked() {
						keyCapture.id, keyCapture.conflict = s.id, ""
					}
				})
			}
		})
		ui.Row(c).Children(func() {
			if ui.Button(c.Key("keys-reset-all"), "全部恢复默认").Disabled(len(a.Profiles.Shortcuts) == 0).Clicked() {
				a.Profiles.Shortcuts = nil
				keyCapture.id = ""
				savePrefs(a.Profiles)
				a.refreshAppMenu()
			}
		})
	})
}

// captureKey applies a recorded key: Esc cancels, bare modifiers wait,
// keys without a name or already taken are refused.
func (a *appState) captureKey(id string, mods ui.Modifiers, key ui.Key) {
	if key == ui.KeyEscape {
		keyCapture.id = ""
		return
	}
	if bindingKeyName(key) == "" {
		return
	}
	b := keyBinding{Mods: mods, Key: key}
	if other := a.shortcutConflictWith(id, b); other != "" {
		keyCapture.conflict = other
		return
	}
	keyCapture.id, keyCapture.conflict = "", ""
	a.setBinding(id, b)
}
