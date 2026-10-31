// Lumiwerk — a media browser aligned to FlowVision.
package main

import (
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	mygo.App.SetName(appName)
	mygo.App.SetVersion(appVersion)

	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128}
	a.Profiles = loadPrefs()
	openPath := func(path string) {
		if a.Window != nil {
			a.Window.Update(func() { a.openFolder(path) })
		}
	}
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
		log.Fatal(err)
	}
}
