package main

import (
	"runtime"

	// MyGo's own controls, dialogs and menus in 简体中文.
	_ "github.com/egoist/mygo/locales/zh-Hans"
)

// Platform-aware labels: the app is written in Chinese only, and the names
// of the system file manager, the volume root and file tags follow the OS.

// appLanguage is the one language Lumiwerk's own texts are written in;
// MyGo's controls and menus (search field, Quit, updater) follow it too.
const appLanguage = "zh-Hans"

// tagsSupported reports whether the platform stores file tags (Finder tags).
var tagsSupported = runtime.GOOS == "darwin"

// rootName names the filesystem root in breadcrumbs and the sidebar.
var rootName = rootNameFor(runtime.GOOS)

func rootNameFor(goos string) string {
	switch goos {
	case "darwin":
		return "Macintosh HD"
	case "windows":
		return "此电脑"
	}
	return "/"
}

// revealLabel is the menu item that shows a file in the system file manager.
var revealLabel = revealLabelFor(runtime.GOOS)

func revealLabelFor(goos string) string {
	switch goos {
	case "darwin":
		return "在 Finder 中显示"
	case "windows":
		return "在资源管理器中显示"
	}
	return "在文件管理器中显示"
}

// openParentLabel jumps to the file's own folder inside Lumiwerk (useful in
// recursive or filtered views); it is not the system file manager.
const openParentLabel = "在 Lumiwerk 中打开所在文件夹"

// tagsLabel names the tag section, menu and setting.
const tagsLabel = "Finder 标签"
