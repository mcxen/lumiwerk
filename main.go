// Lumiwerk — a media browser aligned to FlowVision.
package main

import (
	"log"
	"os"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	initLogging()
	defer closeLogging()
	defer logRecover("main")
	logInfof("%s %s starting (go=%s %s/%s, args=%q, log=%s)",
		appName, appVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH, os.Args[1:], LogPath())

	mygo.App.SetName(appName)
	mygo.App.SetVersion(appVersion)

	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 192, FolderFirst: true}
	a.Profiles = loadPrefs()
	mygo.Theme.SetSource(mygo.ThemeSource(a.Profiles.View.Theme))
	logInfof("preferences loaded: prefs=%s profiles=%d active=%d view=%s",
		prefsPath(), len(a.Profiles.Profiles), a.Profiles.Active, a.Profiles.View.Theme)
	// An open event can arrive before WhenReady has built the window; remember
	// it so the folder is not silently dropped.
	pendingOpen := ""
	openPath := func(path string) {
		if a.Window == nil {
			pendingOpen = path
			return
		}
		a.Window.Update(func() { a.openFolder(path) })
	}
	mygo.App.OnOpenFile(func(path string) {
		if isDir(path) {
			openPath(path)
		}
	})
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		for _, arg := range args {
			if isDir(arg) {
				openPath(arg)
			}
		}
	})
	mygo.App.WhenReady(func() {
		a.Window = mygo.NewWindow(mygo.WindowOptions{
			Title:   appName,
			Width:   1280,
			Height:  820,
			Content: ui.View(a.view),
		})

		mygo.App.SetMenu(newAppMenu(a))
		if pendingOpen != "" {
			a.openFolder(pendingOpen)
			pendingOpen = ""
		}
		for _, arg := range os.Args[1:] {
			if isDir(arg) {
				openPath(arg)
			}
		}
		for _, arg := range os.Args[1:] {
			if arg == "--large" {
				go func() {
					time.Sleep(1500 * time.Millisecond)
					a.Window.Update(func() {
						if len(a.Visible) > 0 {
							a.openLargeImage(0)
						}
					})
				}()
			}
		}
		if a.Profiles.Active >= 0 && a.Profiles.Active < len(a.Profiles.Profiles) {
			if p := a.Profiles.Profiles[a.Profiles.Active]; p.Root != "" && isDir(p.Root) {
				openPath(p.Root)
			}
		}
	})

	if err := mygo.App.Run(); err != nil {
		logErrorf("app run failed: %v", err)
		log.Fatal(err)
	}
	logInfof("%s exited cleanly", appName)
}
