package main

// setTitle tolerates the nil window of headless tests.
func (a *appState) setTitle(title string) {
	if a.Window != nil {
		a.Window.SetTitle(title)
	}
}
