// Lumiwerk — a media browser aligned to FlowVision.
package main

import (
	"log"
	"os"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

func main() {
	initLogging()
	defer closeLogging()
	defer logRecover("main")
	logInfof("%s %s starting (go=%s %s/%s, args=%q, log=%s)",
		appName, appVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH, os.Args[1:], LogPath())

	mygo.App.SetName(appName)
	mygo.App.SetVersion(appVersion)
	mygo.App.SetLanguages(appLanguage)
	// The update window checks in the background once a day and offers to
	// install what it finds; it needs a build signed with the update key,
	// so development builds never check (updater.CheckForUpdates says why).
	mygo.Use(updater.Plugin)
	// Media steps leave temporaries only while they run; anything older
	// than an hour was left by a crashed run, so sweep it in the background.
	safeGo("main.tempSweep", sweepTempLeftovers)

	a := &appState{Chosen: -1, LargeIndex: -1, keyPivot: -1, Thumb: defaultThumb, LayoutMode: defaultLayout, FolderFirst: true}
	a.Profiles = loadPrefs()
	thumbLRU.SetLimit(a.Profiles.ThumbCacheMiB << 20)
	a.VideoVolume = normalizedPlaybackVolume(a.Profiles.Volume)
	a.PrevVolume = normalizedPlaybackVolume(a.Profiles.PreviousVolume)
	applyUpdateSource(a.Profiles.UpdateSource)
	mygo.Theme.SetSource(mygo.ThemeSource(a.Profiles.View.Theme))
	logInfof("preferences loaded: prefs=%s profiles=%d active=%d view=%s",
		prefsPath(), len(a.Profiles.Profiles), a.Profiles.Active, a.Profiles.View.Theme)
	// The browsing trail outlives the process: load it before any open so
	// goBack/goForward and the sidebar already span past sessions.
	saved := loadHistory()
	a.History, a.Cursor, a.Visited, a.Recent = saved.History, saved.Cursor, saved.Visited, saved.Recent
	history := saved
	if a.History == nil {
		history.Cursor = -1
	}
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
	// OnBeforeQuit runs before any window closes, on the main thread, so
	// the main window's OnClose can let a real quit through.
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	// A Dock click asks the app to reopen. Show the browser when it is
	// hidden; a visible browser is left where the user put it.
	mygo.App.OnActivate(func(bool) {
		if !mygo.App.IsReady() || (a.Window != nil && a.Window.IsVisible()) {
			return
		}
		a.showMainWindow()
	})
	mygo.App.WhenReady(func() {
		// The icon stays for the whole run, window visible or not, and is
		// removed in OnWillQuit. macOS menu-bar apps keep the item resident.
		a.installTray()
		a.ensureMainWindow()
		a.syncTrayTip()

		a.appMenu = newAppMenu(a)
		mygo.App.SetMenu(a.appMenu)
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
				safeGo("main.openInitialLargeView", func() {
					time.Sleep(1500 * time.Millisecond)
					a.Window.Update(func() {
						if len(a.Visible) > 0 {
							a.openLargeImage(0)
						}
					})
				})
			}
		}
		opened := false
		if active := a.Profiles.Active; active >= 0 && active < len(a.Profiles.Profiles) {
			if p := a.Profiles.Profiles[active]; p.Root != "" && isDir(p.Root) {
				openPath(p.Root)
				opened = true
			}
		}
		// No configured startup folder (or the run below replaces it): fall
		// back to where the last session stopped, so a reopening app shows
		// the folder the user left instead of an empty view.
		if !opened && a.Profiles.restoreSession() {
			if last := history.CursorPath(); last != "" && a.Root == "" {
				openPath(last)
			}
		}
	})

	// openFolder already writes the trail; OnWillQuit flushes once more so a
	// late sidebar dwell is not lost if the process exits right after it.
	mygo.App.OnWillQuit(func(_ *mygo.QuitEvent) {
		// openFolder already persists the trail; flush again on quit so a
		// dwell that finished after the last navigate is not lost.
		a.persistBrowsingHistory()
		a.flushPlayProgress()
	})

	if err := mygo.App.Run(); err != nil {
		logErrorf("app run failed: %v", err)
		log.Fatal(err)
	}
	logInfof("%s exited cleanly", appName)
}
