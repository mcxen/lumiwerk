package main

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Main-window geometry is remembered by MyGo under this key
// (window-state.json). Hiding the window keeps the live frame; the next
// launch restores the frame captured when the window last closed.
const mainWindowStateKey = "main"

// ensureMainWindow creates the browser window the first time it is needed,
// including a cold start that reaches the tray before any window exists.
// The same appState is the view's model, so a window created later still
// shows the directory, selection and scroll position already in memory.
func (a *appState) ensureMainWindow() {
	if a.Window != nil {
		return
	}
	w := mygo.NewWindow(windowChrome(mygo.WindowOptions{
		Title:    appName,
		Width:    1280,
		Height:   820,
		StateKey: mainWindowStateKey,
		Content:  ui.View(a.view),
	}))
	w.OnClose(a.mainWindowClosing)
	w.OnShow(func() { a.unshelve() })
	w.OnClosed(func() {
		if a.Window == w {
			a.Window = nil
		}
	})
	a.Window = w
}

// mainWindowClosing hides the browser on the red button and on Cmd+W.
// Settings and diagnostics are other windows and are not wired here, so
// closing one of them leaves the browser and the other windows alone.
// Tabs live inside this window; closing a tab does not come through here.
func (a *appState) mainWindowClosing(e *mygo.CloseEvent) {
	if a.quitting {
		return
	}
	if a.Profiles.hidesOnClose() {
		e.PreventDefault()
		a.shelfMainWindow()
		return
	}
	// "直接退出" ends the process even when a settings or diagnostics
	// window is still open. Quit is posted, so this close finishes first
	// and the quit that follows is not re-entered.
	mygo.App.Quit()
}

// shelfMainWindow hides the browser and releases the work that only the
// visible window needs: the directory watcher, in-flight thumbnail
// decodes, and video playback. The model (folder, selection, scroll,
// tabs) stays where it is.
func (a *appState) shelfMainWindow() {
	if a.Window == nil {
		return
	}
	if !a.isShelved() {
		a.suspendBackground()
		logInfof("main window shelved")
	}
	// Closing to the tray is the usual "I'm done" gesture; flush before
	// the process can be killed while hidden.
	a.persistBrowsingHistory()
	a.Window.Hide()
	a.syncTrayTip()
	a.applyDockPolicy()
}

// showMainWindow brings the browser back, creating it when this run has
// not made one yet. A minimized window is restored, then ordered front
// and focused. Background work resumes only if the window had been shelved.
func (a *appState) showMainWindow() {
	created := a.Window == nil
	a.ensureMainWindow()
	a.unshelve()
	if a.Window.IsMinimized() {
		a.Window.Restore()
	}
	a.Window.Show()
	a.Window.Focus()
	a.syncTrayTip()
	a.applyDockPolicy()
	if created && a.Root == "" {
		a.scheduleLaunchFolder()
	}
}

// scheduleLaunchFolder opens the configured startup folder, or the last
// session's folder, when nothing is open yet. It is the cold-start path
// for a window created from the tray rather than at launch.
func (a *appState) scheduleLaunchFolder() {
	if a.Root != "" || a.Window == nil {
		return
	}
	if active := a.Profiles.Active; active >= 0 && active < len(a.Profiles.Profiles) {
		if p := a.Profiles.Profiles[active]; p.Root != "" && isDir(p.Root) {
			a.update(func() { a.openFolder(p.Root) })
			return
		}
	}
	if !a.Profiles.restoreSession() {
		return
	}
	last := (historyFile{History: a.History, Cursor: a.Cursor}).CursorPath()
	if last != "" && isDir(last) {
		a.update(func() { a.openFolder(last) })
	}
}

func (a *appState) isShelved() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.shelved
}

func (a *appState) setShelved(v bool) {
	a.mu.Lock()
	a.shelved = v
	a.mu.Unlock()
}

// suspendBackground pauses watcher, playback and the thumbnail queue.
// Decodes already sitting on thumbGate notice the cancelled context and
// the bumped generation and leave without calling the decoder.
func (a *appState) suspendBackground() {
	if a.isShelved() {
		return
	}
	a.setShelved(true)
	a.stopWatcher()
	a.shelvedScroll = a.AutoScroll
	a.shelvedPlay = a.AutoPlay
	a.stopAutomation()
	a.resumeVideo = a.videoPlaying()
	if a.resumeVideo {
		a.holdVideo(false)
	}
	a.cancelThumbQueue()
}

// unshelve resumes background work the first time a shelved window is
// shown, including a show that did not go through showMainWindow (the
// Dock's reopen, for example).
func (a *appState) unshelve() {
	if !a.isShelved() {
		return
	}
	a.setShelved(false)
	a.resumeBackground()
	logInfof("main window restored")
}

func (a *appState) resumeBackground() {
	if a.decodeCancel == nil {
		a.decodeCtx, a.decodeCancel = context.WithCancel(context.Background())
	}
	if a.Root != "" {
		a.startWatcher()
	}
	if a.resumeVideo {
		a.resumeVideo = false
		a.playVideo()
	}
	if a.shelvedScroll {
		a.AutoScroll = true
	}
	if a.shelvedPlay {
		a.AutoPlay = true
	}
	a.shelvedScroll = false
	a.shelvedPlay = false
}

// cancelThumbQueue drops queued thumbnail and large-image decodes. Bitmaps
// already on screen stay, so showing the window does not flash empty cards.
func (a *appState) cancelThumbQueue() {
	if a.decodeCancel != nil {
		a.decodeCancel()
		a.decodeCancel = nil
		a.decodeCtx = nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.Items {
		if a.Items[i].Loading {
			a.Items[i].Loading = false
			a.Items[i].LoadGen++
		}
		a.Items[i].LargeLoading = false
	}
}

// thumbClaimStale reports whether a queued thumbnail decode should give
// up: the window was shelved, its generation was superseded, or the
// folder's decode context was cancelled.
func (a *appState) thumbClaimStale(index int, path string, gen int) bool {
	if a.isShelved() {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.decodeCtx != nil && a.decodeCtx.Err() != nil {
		return true
	}
	if index < 0 || index >= len(a.Items) {
		return true
	}
	item := &a.Items[index]
	return item.Path != path || item.LoadGen != gen
}

func (a *appState) clearThumbClaim(index int, path string, gen int) {
	a.update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if index < len(a.Items) && a.Items[index].Path == path && a.Items[index].LoadGen == gen {
			a.Items[index].Loading = false
		}
	})
}
