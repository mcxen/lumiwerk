// Lumiwerk — a media browser aligned to FlowVision.
package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	mygo.App.SetName(appName)
	mygo.App.SetVersion(appVersion)

	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 512}
	mygo.App.WhenReady(func() {
		a.Window = mygo.NewWindow(mygo.WindowOptions{
			Title:   appName,
			Width:   1280,
			Height:  820,
			Content: ui.View(a.view),
		})

		mygo.App.SetMenu(newAppMenu(a))
		if len(os.Args) > 1 {
			a.Window.Update(func() { a.openFolder(os.Args[1]) })
		}
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
