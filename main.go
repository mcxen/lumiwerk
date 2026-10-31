// Lumiwerk — a media browser aligned to FlowVision's interface and behavior.
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
const appVersion = "0.2.0"

// SVG icons matching FlowVision's visual language.
var (
	dirIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>`))
	imageIcon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="2"/><path d="m21 16-5-5-7 7"/></svg>`))
	videoIcon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="5" width="12" height="14" rx="2"/><path d="m21 8-6 4 6 4z"/></svg>`))
	backIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 12H5"/><path d="m12 19-7-7 7-7"/></svg>`))
	forwardIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>`))
	upIcon      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 19V5"/><path d="m5 12 7-7 7 7"/></svg>`))
)

// mediaItem is one file in the currently scanned folder.
type mediaItem struct {
	Path    string
	Name    string
	Ext     string
	Size    int64
	ModTime int64
	Bitmap  *ui.Bitmap
	Decoded bool
	Failed  bool
}

// treeNode is a directory in the sidebar.
type treeNode struct {
	Path     string
	Name     string
	Children []treeNode
	Loaded   bool
}

// appState is all mutable UI state. It is only read and changed on the
// main thread, in the view and Window.Update callbacks.
type appState struct {
	Window *mygo.Window
	Root   string
	Query  string

	Items   []mediaItem
	Visible []int
	Grid    ui.GridState
	Chosen  int

	// Sidebar
	TreeRoot     treeNode
	Outline      ui.OutlineState[string]
	TreeChose    int
	SidebarWidth float32

	// Large image view
	InLargeView bool
	LargeIndex  int
	LargeBitmap *ui.Bitmap
	LargeFit    bool
	Rotation    int
	Mirror      bool

	mu sync.Mutex
}

func isMediaFile(ext string) bool {
	switch strings.ToLower(ext) {
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "tiff", "tif", "ico", "svg":
		return true
	default:
		return false
	}
}

func isVideoFile(ext string) bool {
	switch strings.ToLower(ext) {
	case "mp4", "mov", "mkv", "avi", "webm", "m4v", "ts", "mts":
		return true
	default:
		return false
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
	a.InLargeView = false
	a.LargeIndex = -1
	a.LargeBitmap = nil
	a.mu.Unlock()

	a.Window.SetTitle(fmt.Sprintf("%s — %s", appName, filepath.Base(path)))
	go a.scanFolder(path)
}

// scanFolder walks a directory tree in the background, then updates the UI.
func (a *appState) scanFolder(root string) {
	var files []mediaItem
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
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

// loadThumb decodes a thumbnail off the main thread.
func (a *appState) loadThumb(root string, index int) {
	a.mu.Lock()
	item := &a.Items[index]
	if item.Decoded || item.Failed {
		a.mu.Unlock()
		return
	}
	path := item.Path
	a.mu.Unlock()

	go func() {
		data, err := os.ReadFile(path)
		var bitmap *ui.Bitmap
		if err == nil {
			bitmap, err = ui.DecodeBitmap(data)
		}
		a.Window.Update(func() {
			a.mu.Lock()
			if a.Root != root || index >= len(a.Items) || a.Items[index].Path != path {
				a.mu.Unlock()
				return
			}
			current := &a.Items[index]
			if err != nil {
				current.Failed = true
			} else {
				current.Bitmap = bitmap
				current.Decoded = true
			}
			a.mu.Unlock()
		})
	}()
}

// buildTree loads the root directory tree (one level at a time).
func (a *appState) buildTree(root string) []treeNode {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var nodes []treeNode
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		nodes = append(nodes, treeNode{
			Path: filepath.Join(root, name),
			Name: name,
		})
	}
	return nodes
}

// treeChildren loads children lazily when a node is expanded.
func (a *appState) treeChildren(path string) []string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, filepath.Join(path, e.Name()))
		}
	}
	return out
}

func pathName(path string) string {
	if path == "/" {
		return "Macintosh HD"
	}
	return filepath.Base(path)
}

// ============================
// Main View (aligned to FlowVision)
// ============================

func (a *appState) view(c *ui.Context) {
	t := c.Theme()
	root := c.Root()
	root.Background(t.Background)

	// Global shortcuts
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		go a.pickFolder()
	}

	// FlowVision layout: sidebar (left) | main area (right)
	// Sidebar ~220 DIPs wide like FlowVision
	a.SidebarWidth = 220
	ui.Split(c, &a.SidebarWidth, func() { a.sidebarView(c) }, func() { a.mainView(c) }).Fill()
}

// sidebarView renders the directory tree sidebar.
func (a *appState) sidebarView(c *ui.Context) {
	t := c.Theme()
	pane := ui.Column(c).Fill().Background(t.Surface)
	pane.Children(func() {
		// Sidebar header (like "Favorites")
		ui.Row(c).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Favorites").FontSize(11).Bold().TextColor(t.TextMuted)
		})
		ui.Divider(c)

		// Build the tree from root if not yet loaded
		if a.Root != "" {
			// The Outline component renders a tree
			a.Outline.List.Selected = &a.TreeChose
			a.Outline.List.Key = func(i int) any { return i }

			// Use Outline with string keys (paths)
			roots := []string{a.Root}
			ui.Outline(c, &a.Outline, roots, a.treeChildren, func(path string) {
				a.treeRow(c, path)
			}).Grow(1)
		} else {
			ui.Text(c, "Open a folder to begin").TextColor(t.TextMuted).Padding(12)
		}
	})
}

func (a *appState) treeRow(c *ui.Context, path string) {
	t := c.Theme()
	row := ui.Row(c).Padding(4, 8).Gap(6).AlignItems(ui.Center).Radius(4)
	if a.Root == path {
		row.Background(t.Accent).TextColor(t.AccentText)
	} else {
		row.Background(t.Surface)
	}
	row.Children(func() {
		ui.Icon(c, dirIcon).FontSize(14).TextColor(t.Text)
		ui.Text(c, pathName(path)).FontSize(12).SingleLine()
	})
	row.OnClick(func() {
		a.openFolder(path)
	})
}

// mainView renders the toolbar, grid, and status bar.
func (a *appState) mainView(c *ui.Context) {
	t := c.Theme()
	col := ui.Column(c).Fill().Background(t.Background)
	col.Children(func() {
		a.toolbarView(c)
		ui.Divider(c)
		if a.InLargeView {
			a.largeImageView(c)
		} else if len(a.Visible) == 0 {
			a.emptyView(c)
		} else {
			a.gridView(c)
		}
		ui.Divider(c)
		a.statusBar(c)
	})
}

// toolbarView renders the FlowVision-style toolbar with back/forward/up, path, search.
func (a *appState) toolbarView(c *ui.Context) {
	t := c.Theme()
	ui.Toolbar(c, func() {
		if ui.Button(c.Key("back"), "").Label("Back").Width(28).Tooltip("Back").Clicked() {
			// TODO: navigation history
		}
		if ui.Button(c.Key("fwd"), "").Label("Forward").Width(28).Tooltip("Forward").Clicked() {
			// TODO: navigation history
		}
		if ui.Button(c.Key("up"), "").Label("Up").Width(28).Tooltip("Parent Folder").Clicked() {
			parent := filepath.Dir(a.Root)
			if parent != a.Root {
				a.openFolder(parent)
			}
		}

		// Breadcrumb-like path label
		relPath := a.Root
		if relPath == "" {
			relPath = "No folder"
		}
		ui.Text(c, relPath).FontSize(11).TextColor(t.TextMuted).MaxLines(1).Grow(1)

		// Search field
		if search := ui.SearchField(c, &a.Query).Width(180).Changed(); search {
			a.applyFilter()
		}
	}).Padding(4, 8).Background(t.Surface)
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
		zone.Border(2, t.Accent).Radius(8)
	}
	zone.Children(func() {
		ui.Icon(c, imageIcon).FontSize(48).TextColor(t.TextMuted)
		ui.Text(c, "Drop a folder to begin").FontSize(16).Bold()
		ui.Text(c, "or File → Open Folder… (Cmd+O)").TextColor(t.TextMuted)
	})
}

func (a *appState) gridView(c *ui.Context) {
	t := c.Theme()
	root := a.Root

	a.Grid.Key = func(i int) any { return a.Visible[i] }
	a.Grid.Selected = &a.Chosen
	a.Grid.Label = func(i int) string { return a.Items[a.Visible[i]].Name }

	// FlowVision grid: virtualized, thumbnail + filename below, blue outline on selection
	grid := ui.GridView(c, &a.Grid, len(a.Visible), 180, 210, func(i int) {
		item := &a.Items[a.Visible[i]]
		card := ui.Column(c).Grow(1).Margin(4).Padding(4).Gap(4).
			Radius(6).Background(t.Surface)
		if a.Chosen == i {
			card.Border(2, t.Accent)
		}
		card.Children(func() {
			thumb := ui.Box(c).Grow(1).Radius(4).Background(t.Background).
				Justify(ui.Center).AlignItems(ui.Center)
			thumb.Children(func() {
				if item.Bitmap != nil {
					ui.Image(c, item.Bitmap).Fit(ui.Contain).Grow(1)
				} else if item.Failed {
					ui.Icon(c, videoIcon).FontSize(24).TextColor(t.TextMuted)
				} else {
					ui.Spinner(c)
				}
			})
			ui.Text(c, item.Name).FontSize(11).MaxLines(1).SingleLine().
				TextAlign(ui.Start)
		})

		if !item.Decoded && !item.Failed {
			a.loadThumb(root, a.Visible[i])
		}
	})

	grid.Grow(1)
	// Submit (double-click or Enter) opens large view
	if grid.Submitted() && a.Chosen >= 0 && a.Chosen < len(a.Visible) {
		a.openLargeImage(a.Chosen)
	}
}

func (a *appState) statusBar(c *ui.Context) {
	t := c.Theme()
	bar := ui.Row(c).Padding(4, 12).Background(t.Surface)
	bar.Children(func() {
		label := "No folder"
		if a.Root != "" {
			label = fmt.Sprintf("%s (%d images)", a.Root, len(a.Visible))
		}
		ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
	})
}

// largeImageView shows the full image, replacing the grid.
func (a *appState) largeImageView(c *ui.Context) {
	t := c.Theme()

	// Keyboard shortcuts
	if c.Shortcut(0, ui.KeyEscape) {
		a.closeLargeImage()
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyLeft) {
		a.prevImage()
	}
	if c.Shortcut(ui.Cmd, ui.KeyRight) {
		a.nextImage()
	}
	if c.Shortcut(0, ui.KeySpace) {
		a.closeLargeImage()
		return
	}

	// Image display
	if a.LargeIndex >= 0 && a.LargeIndex < len(a.Visible) {
		item := &a.Items[a.Visible[a.LargeIndex]]
		ui.Column(c).Fill().Background(t.Background).Children(func() {
			if item.Bitmap != nil {
				img := ui.Image(c, item.Bitmap)
				if a.LargeFit {
					img.Fit(ui.Contain).Grow(1)
				} else {
					img.Fit(ui.NaturalSize).Grow(1)
				}
			} else {
				ui.Spinner(c)
			}
			ui.Textf(c, "%s (%d/%d)", item.Name, a.LargeIndex+1, len(a.Visible)).
				FontSize(12).TextColor(t.TextMuted).AlignSelf(ui.End).Padding(8)
		})
	} else {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "No image").TextColor(t.TextMuted)
		})
	}
}

func (a *appState) openLargeImage(index int) {
	if index < 0 || index >= len(a.Visible) {
		return
	}
	a.InLargeView = true
	a.LargeIndex = index
	a.LargeFit = true
	item := a.Items[a.Visible[index]]
	a.LargeBitmap = item.Bitmap
	a.Window.SetTitle(fmt.Sprintf("%s (%d/%d)", item.Name, index+1, len(a.Visible)))
	// Load the full bitmap if not yet loaded
	if item.Bitmap == nil && !item.Failed {
		go a.loadFullImage(a.Root, a.Visible[index])
	}
}

func (a *appState) loadFullImage(root string, index int) {
	a.mu.Lock()
	if index >= len(a.Items) {
		a.mu.Unlock()
		return
	}
	path := a.Items[index].Path
	a.mu.Unlock()

	go func() {
		data, err := os.ReadFile(path)
		var bitmap *ui.Bitmap
		if err == nil {
			bitmap, err = ui.DecodeBitmap(data)
		}
		a.Window.Update(func() {
			a.mu.Lock()
			if a.Root != root || index >= len(a.Items) || a.Items[index].Path != path {
				a.mu.Unlock()
				return
			}
			if err != nil {
				a.Items[index].Failed = true
			} else {
				a.Items[index].Bitmap = bitmap
				a.Items[index].Decoded = true
			}
			a.mu.Unlock()
			if a.InLargeView && a.LargeIndex >= 0 && a.LargeIndex < len(a.Visible) && a.Visible[a.LargeIndex] == index {
				a.LargeBitmap = a.Items[index].Bitmap
			}
		})
	}()
}

func (a *appState) closeLargeImage() {
	a.InLargeView = false
	a.LargeBitmap = nil
	if a.Root != "" {
		a.Window.SetTitle(fmt.Sprintf("%s — %s", appName, filepath.Base(a.Root)))
	}
}

func (a *appState) nextImage() {
	if a.LargeIndex+1 < len(a.Visible) {
		a.openLargeImage(a.LargeIndex + 1)
	}
}

func (a *appState) prevImage() {
	if a.LargeIndex > 0 {
		a.openLargeImage(a.LargeIndex - 1)
	}
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
	mygo.App.SetVersion(appVersion)

	a := &appState{Chosen: -1, LargeIndex: -1}
	mygo.App.WhenReady(func() {
		a.Window = mygo.NewWindow(mygo.WindowOptions{
			Title:   appName,
			Width:   1280,
			Height:  820,
			Content: ui.View(a.view),
		})

		// File menu like FlowVision
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
