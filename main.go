package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const appName = "Lumiwerk"

var (
	dirIcon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 7a2 2 0 0 1 2-2h3l2 2h9a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>`))
	imageIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="2"/><path d="m21 16-5-5-7 7"/></svg>`))
	videoIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="5" width="12" height="14" rx="2"/><path d="m21 8-6 4 6 4z"/></svg>`))
)

type mediaItem struct {
	Path     string
	Name     string
	Ext      string
	Size     int64
	ModTime  int64
	Bitmap   *ui.Bitmap
	Decoded  bool
	Decoding bool
	Failed   bool
}

type appState struct {
	Window *mygo.Window
	Root   string
	Query  string

	Items   []mediaItem
	Visible []int
	Grid    ui.GridState
	Chosen  int

	mu sync.Mutex
}

func isMediaFile(ext string) bool {
	switch strings.ToLower(ext) {
	case "png", "jpg", "jpeg", "gif", "webp", "bmp":
		return true
	default:
		return false
	}
}

func (a *appState) openFolder(path string) {
	if path == "" || !isDir(path) {
		return
	}

	a.mu.Lock()
	a.Root = path
	a.Query = ""
	a.Items = nil
	a.Visible = nil
	a.Chosen = -1
	a.mu.Unlock()

	a.Window.SetTitle(fmt.Sprintf("%s — %s", appName, filepath.Base(path)))
	go a.scanFolder(path)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (a *appState) scanFolder(root string) {
	var files []mediaItem

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.TrimPrefix(filepath.Ext(path), ".")
		if !isMediaFile(ext) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files = append(files, mediaItem{
			Path:    path,
			Name:    filepath.Base(path),
			Ext:     strings.ToLower(ext),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})
		return nil
	})

	if err != nil {
		err = nil
	}

	slices.SortStableFunc(files, func(a, b mediaItem) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	a.Window.Update(func() {
		a.mu.Lock()
		if a.Root != root {
			a.mu.Unlock()
			return
		}
		a.Items = files
		a.mu.Unlock()
		a.applyFilter()
	})
}

func (a *appState) applyFilter() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.Visible = a.Visible[:0]
	q := strings.ToLower(strings.TrimSpace(a.Query))
	for i := range a.Items {
		if q == "" || strings.Contains(strings.ToLower(a.Items[i].Name), q) {
			a.Visible = append(a.Visible, i)
		}
	}
	if a.Chosen >= len(a.Visible) {
		a.Chosen = -1
	}
}

func (a *appState) loadThumb(root string, index int) {
	a.mu.Lock()
	item := &a.Items[index]
	if item.Decoding || item.Decoded || item.Failed || !isMediaFile(item.Ext) {
		a.mu.Unlock()
		return
	}
	item.Decoding = true
	a.mu.Unlock()

	go func() {
		data, err := os.ReadFile(item.Path)
		bitmap, decodeErr := ui.DecodeBitmap(data)
		a.Window.Update(func() {
			a.mu.Lock()
			if a.Root != root || index >= len(a.Items) || a.Items[index].Path != item.Path {
				a.mu.Unlock()
				return
			}
			current := &a.Items[index]
			current.Decoding = false
			if err != nil || decodeErr != nil {
				current.Failed = true
			} else {
				current.Bitmap = bitmap
				current.Decoded = true
			}
			a.mu.Unlock()
		})
	}()
}

func (a *appState) view(c *ui.Context) {
	t := c.Theme()
	root := c.Root()
	root.Background(t.Background)

	if c.Shortcut(ui.Cmd, ui.KeyO) {
		go a.pickFolder()
	}

	// Hero/toolbar
	ui.Column(c).Fill().Children(func() {
		header := ui.Row(c).Padding(12, 16).Gap(12).AlignItems(ui.Center).Background(t.Surface)
		header.Children(func() {
			ui.Icon(c, dirIcon).FontSize(18).TextColor(t.Text)
			title := ui.Text(c, appName).FontSize(20).Bold()
			_ = title

			ui.Box(c).Width(1).Height(24).Background(t.Border)

			pathLabel := "No folder selected"
			if a.Root != "" {
				pathLabel = filepath.Base(a.Root)
			}
			ui.Text(c, pathLabel).TextColor(t.TextMuted).MaxLines(1)

			ui.Spacer(c)

			if ui.Button(c, "Open Folder").Clicked() {
				go a.pickFolder()
			}

			if search := ui.SearchField(c, &a.Query).Width(260).Changed(); search {
				a.applyFilter()
			}
		})

		ui.Divider(c)

		if len(a.Visible) == 0 {
			a.emptyView(c)
			return
		}

		a.mediaGrid(c)
	})
}

func (a *appState) emptyView(c *ui.Context) {
	t := c.Theme()

	zone := ui.Column(c).Fill().Center().AlignItems(ui.Center).Gap(8)
	if files := zone.DroppedFiles(); len(files) > 0 {
		for _, f := range files {
			if isDir(f) {
				a.openFolder(f)
				break
			}
		}
	}
	if zone.FileDragOver() {
		zone.Border(2, t.Accent).Radius(12)
	}
	zone.Children(func() {
		ui.Icon(c, imageIcon).FontSize(48).TextColor(t.TextMuted)
		ui.Text(c, "Drop a folder to begin").FontSize(18).Bold()
		ui.Text(c, "or choose File → Open Folder… (Cmd+O)").TextColor(t.TextMuted)
	})
}

func (a *appState) mediaGrid(c *ui.Context) {
	t := c.Theme()
	root := a.Root

	a.Grid.Key = func(i int) any { return a.Visible[i] }
	a.Grid.Selected = &a.Chosen
	a.Grid.Label = func(i int) string { return a.Items[a.Visible[i]].Name }

	// GridView is virtualized: only visible cells are built.
	grid := ui.GridView(c, &a.Grid, len(a.Visible), 180, 210, func(i int) {
		item := &a.Items[a.Visible[i]]
		card := ui.Column(c).Grow(1).Margin(6).Padding(8).Gap(4).
			Radius(8).Background(t.Surface).
			Border(1, t.Border)
		if a.Chosen == i {
			card.Border(2, t.Accent)
		}
		card.Children(func() {
			thumb := ui.Box(c).Grow(1).Radius(6).Background(t.Background).
				Justify(ui.Center).AlignItems(ui.Center)
			thumb.Children(func() {
				if item.Bitmap != nil {
					ui.Image(c, item.Bitmap).Fit(ui.Contain).Grow(1)
				} else if item.Failed {
					ui.Icon(c, videoIcon).FontSize(24).TextColor(t.TextMuted)
					ui.Text(c, "Preview unavailable").FontSize(11).TextColor(t.TextMuted)
				} else {
					ui.Spinner(c)
				}
			})

			name := ui.Text(c, item.Name).FontSize(12).MaxLines(1)
			_ = name

			ui.Textf(c, "%s · %.1f MB", strings.ToUpper(item.Ext), float64(item.Size)/(1024*1024)).
				FontSize(10).TextColor(t.TextMuted)
		})

		if !item.Decoded && !item.Failed && !item.Decoding {
			a.loadThumb(root, a.Visible[i])
		}
	})
	grid.Grow(1)
}

func (a *appState) pickFolder() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:            a.Window,
		Title:             "Open Media Folder",
		Directory:         true,
		CreateDirectories: false,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.Window.Update(func() {
		a.openFolder(paths[0])
	})
}

func main() {
	mygo.App.SetName(appName)
	mygo.App.SetVersion("0.1.0")

	a := &appState{Chosen: -1}
	mygo.App.WhenReady(func() {
		a.Window = mygo.NewWindow(mygo.WindowOptions{
			Title:   appName,
			Width:   1280,
			Height:  820,
			Content: ui.View(a.view),
		})

		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Label: "File", Submenu: []*mygo.MenuItem{
				{Label: "Open Folder…", Accelerator: "CmdOrCtrl+O", Click: func(item *mygo.MenuItem, win *mygo.Window) {
					go a.pickFolder()
				}},
			}},
		}))
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
