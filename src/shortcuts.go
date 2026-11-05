// Rebindable keyboard shortcuts: one registry drives the in-view key
// handling, the app menu accelerators and the 快捷键 settings page.
package main

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type keyBinding struct {
	Mods ui.Modifiers `json:"mods"`
	Key  ui.Key       `json:"key"`
}

type shortcutAction struct {
	id, label string
	def       keyBinding
}

var shortcutActions = []shortcutAction{
	{"sidebar", "显示/隐藏侧栏", keyBinding{ui.Cmd, ui.KeyB}},
	{"nocaption", "无字模式", keyBinding{ui.Cmd | ui.Shift, ui.KeyH}},
	{"hidden", "显示隐藏文件", keyBinding{ui.Cmd | ui.Shift, ui.KeyPeriod}},
	{"alltypes", "显示全部类型", keyBinding{ui.Cmd | ui.Shift, ui.KeyComma}},
	{"recursive", "递归浏览", keyBinding{ui.Cmd | ui.Shift, ui.KeyR}},
	{"folders", "包含文件夹", keyBinding{ui.Cmd | ui.Shift, ui.KeyF}},
	{"pin", "窗口置顶", keyBinding{0, ui.KeyT}},
	{"maximize", "最大化窗口", keyBinding{0, ui.Key1}},
	{"fitwindow", "合适窗口", keyBinding{0, ui.Key2}},
	{"center", "窗口居中", keyBinding{0, ui.Key5}},
	{"rename", "重命名", keyBinding{0, ui.KeyF2}},
	{"info", "文件简介", keyBinding{ui.Cmd, ui.KeyI}},
	{"copycfg", "复制到目标", keyBinding{ui.Cmd | ui.Shift, ui.KeyC}},
	{"diagnostics", "运行诊断", keyBinding{ui.Cmd | ui.Shift, ui.KeyD}},
	{"settings", "设置", keyBinding{ui.Cmd, ui.KeyComma}},
}

func shortcutDefault(id string) keyBinding {
	for _, s := range shortcutActions {
		if s.id == id {
			return s.def
		}
	}
	return keyBinding{}
}

// binding is the user's key for id, or its default.
func (a *appState) binding(id string) keyBinding {
	if b, ok := a.Profiles.Shortcuts[id]; ok {
		return b
	}
	return shortcutDefault(id)
}

// keyHit reports whether id's shortcut was pressed this frame.
func (a *appState) keyHit(c *ui.Context, id string) bool {
	b := a.binding(id)
	return b.Key != 0 && c.Shortcut(b.Mods, b.Key)
}

// accel is id's shortcut as an app menu accelerator.
func (a *appState) accel(id string) string {
	b := a.binding(id)
	name := bindingKeyName(b.Key)
	if name == "" {
		return ""
	}
	var parts []string
	if b.Mods&ui.Cmd != 0 {
		parts = append(parts, "CmdOrCtrl")
	}
	if b.Mods&ui.Ctrl != 0 && ui.Cmd != ui.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if b.Mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if b.Mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, name), "+")
}

// bindingLabel is a binding as shown on the shortcuts page.
func bindingLabel(b keyBinding) string {
	name := bindingKeyName(b.Key)
	if name == "" {
		return "无"
	}
	mac := runtime.GOOS == "darwin"
	var s string
	for _, m := range []struct {
		mod       ui.Modifiers
		mac, text string
	}{{ui.Ctrl, "⌃", "Ctrl+"}, {ui.Alt, "⌥", "Alt+"}, {ui.Shift, "⇧", "Shift+"}, {ui.Super, "⌘", "Win+"}} {
		if b.Mods&m.mod != 0 {
			if mac {
				s += m.mac
			} else {
				s += m.text
			}
		}
	}
	return s + name
}

func bindingKeyName(k ui.Key) string {
	switch {
	case k >= ui.KeyA && k <= ui.KeyZ:
		return string(rune('A' + k - ui.KeyA))
	case k >= ui.Key0 && k <= ui.Key9:
		return string(rune('0' + k - ui.Key0))
	case k >= ui.KeyF1 && k <= ui.KeyF12:
		return "F" + strconv.Itoa(int(k-ui.KeyF1)+1)
	}
	switch k {
	case ui.KeyPeriod:
		return "."
	case ui.KeyComma:
		return ","
	case ui.KeyMinus:
		return "-"
	case ui.KeyEqual:
		return "="
	case ui.KeyUp:
		return "Up"
	case ui.KeyDown:
		return "Down"
	case ui.KeyLeft:
		return "Left"
	case ui.KeyRight:
		return "Right"
	case ui.KeyDelete:
		return "Delete"
	case ui.KeyEnter:
		return "Enter"
	case ui.KeySpace:
		return "Space"
	}
	return ""
}

// shortcutConflictWith names the action already using b, other than id.
func (a *appState) shortcutConflictWith(id string, b keyBinding) string {
	for _, s := range shortcutActions {
		if s.id != id && a.binding(s.id) == b {
			return s.label
		}
	}
	return ""
}

// setBinding stores b for id (the default clears the override), saves
// and rebuilds the app menu so its accelerators follow.
func (a *appState) setBinding(id string, b keyBinding) {
	if a.Profiles.Shortcuts == nil {
		a.Profiles.Shortcuts = map[string]keyBinding{}
	}
	if b == shortcutDefault(id) {
		delete(a.Profiles.Shortcuts, id)
	} else {
		a.Profiles.Shortcuts[id] = b
	}
	savePrefs(a.Profiles)
	a.refreshAppMenu()
}

// refreshAppMenu reinstalls the app menu after a binding changes.
func (a *appState) refreshAppMenu() {
	if a.Window == nil {
		return
	}
	a.appMenu, a.menuKeysOff = newAppMenu(a), nil
	mygo.App.SetMenu(a.appMenu)
}
