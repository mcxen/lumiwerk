package main

import (
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// workShowDelay keeps a quick operation from flashing a progress bar.
const workShowDelay = 350 * time.Millisecond

// workState is the long operation running in the background, shown as a
// compact ui.Progress in the status bar: determinate while total is known
// (items done of total), indeterminate otherwise.
type workState struct {
	mu          sync.Mutex
	id          int
	label       string
	done, total int
	since       time.Time
}

// beginWork starts showing a long operation and returns its token for
// stepWork and endWork. total is 0 when progress is unknown.
func (a *appState) beginWork(label string, total int) int {
	w := &a.work
	w.mu.Lock()
	w.id++
	w.label, w.done, w.total, w.since = label, 0, total, time.Now()
	id := w.id
	w.mu.Unlock()
	a.invalidate()
	return id
}

// stepWork counts one item of work id done; safe from any goroutine.
func (a *appState) stepWork(id int) {
	w := &a.work
	w.mu.Lock()
	if w.id == id {
		w.done++
	}
	w.mu.Unlock()
	a.invalidate()
}

// endWork stops showing work id, unless a newer one replaced it.
func (a *appState) endWork(id int) {
	w := &a.work
	w.mu.Lock()
	if w.id == id {
		w.label = ""
	}
	w.mu.Unlock()
	a.invalidate()
}

func (a *appState) invalidate() {
	if a.Window != nil {
		a.Window.Invalidate()
	}
}

// workIndicator draws the running operation in the status bar.
func (a *appState) workIndicator(c *ui.Context) {
	w := &a.work
	w.mu.Lock()
	label, done, total, since := w.label, w.done, w.total, w.since
	w.mu.Unlock()
	if label == "" {
		return
	}
	if wait := workShowDelay - time.Since(since); wait > 0 {
		c.After(wait)
		return
	}
	t := c.Theme()
	value := -1.0
	if total > 1 {
		value = float64(min(done, total)) / float64(total)
		label += " " + strconv.Itoa(min(done, total)) + "/" + strconv.Itoa(total)
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(0, 8, 0, 0).Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(t.TextMuted).SingleLine()
		ui.Progress(c, value).Width(90).Label(label)
	})
}
