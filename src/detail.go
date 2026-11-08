package main

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// detailColumns are the columns of the detail (list) layout. Their IDs are
// the app's Sort keys, so a header click and the sort menu set one value.
var detailColumns = []ui.TableColumn{
	{Title: "名称", ID: "name", MinWidth: 120, Sortable: true},
	{Title: "大小", ID: "size", Width: 76, MinWidth: 48, Align: ui.End, Sortable: true},
	{Title: "修改日期", ID: "date", Width: 140, MinWidth: 72, Sortable: true},
}

// tableSortFor maps the app's Sort setting to the table's header arrow:
// none for sorts without a column (type, tag, random, Exif…).
func tableSortFor(sort string) ui.SortOrder {
	key := strings.TrimSuffix(sort, "-desc")
	if key == "" {
		key = "name"
	}
	for _, col := range detailColumns {
		if col.ID == key {
			return ui.SortOrder{Column: key, Descending: strings.HasSuffix(sort, "-desc")}
		}
	}
	return ui.SortOrder{}
}

// sortForTable is the Sort setting a header click chose.
func sortForTable(o ui.SortOrder) string {
	if o.Column == "" {
		return ""
	}
	if o.Descending {
		return o.Column + "-desc"
	}
	return o.Column
}

// denseTheme is the window's theme with a tighter spacing, for table rows
// as dense as the old hand-laid list (mygo sizes rows from Spacing).
func (a *appState) denseTheme(base *ui.Theme) *ui.Theme {
	if a.denseBase != base || a.dense == nil {
		t := *base
		t.Spacing = 3
		a.denseBase, a.dense = base, &t
	}
	return a.dense
}

// detailView is the list layout: a mygo Table of name, size and modified
// date. Selection stays the app's (a.Chosen / a.Selected, drawn as the
// quiet neutral fill); the table's own selection is not wired, as for the
// grid (see mediaListState). Header clicks set a.Sort.
func (a *appState) detailView(c *ui.Context) {
	state := a.mediaListState()
	list := state.List
	if a.takeReveal() {
		list.ScrollIntoView(a.Chosen)
	}
	if !a.columnsLoaded {
		a.columnsLoaded = true
		list.Columns = a.Profiles.DetailColumns
	}
	a.tableSort = tableSortFor(a.Sort)
	list.Sort = &a.tableSort
	before := a.tableSort
	base := c.Theme()
	t := a.denseTheme(base)
	padV, padH := t.Space(1.5), t.Space(2.5)
	ui.Themed(c, t).Background(ui.Transparent).Grow(1).Children(func() {
		table := ui.Table(c, list, detailColumns, len(a.Visible), func(i, col int) {
			item := a.visibleItem(i)
			if item == nil {
				return
			}
			// The cell's content covers its padding, so the whole row takes
			// clicks, drags and the menu, and the selection fill is unbroken.
			cell := ui.Row(c).Margin(-padV, -padH).Padding(padV, padH).Grow(1).AlignSelf(ui.Stretch).
				AlignItems(ui.Center).Gap(6)
			if detailColumns[col].Align == ui.End {
				cell.Justify(ui.End)
			}
			if a.Chosen == i || a.Selected.Has(item.Path) {
				cell.Background(base.SurfacePressed)
			}
			var name ui.Element
			wasChosen := a.Chosen == i && a.Selected.Len() <= 1
			cell.Children(func() {
				switch detailColumns[col].ID {
				case "name":
					if item.IsDir {
						paintFolderGlyph(c, item.Path, 15)
					} else {
						paintTypeGlyph(c, item.Ext, 15)
					}
					if a.renamingInline(i) {
						a.inlineRenameField(c, 12, false)
					} else {
						name = ui.Text(c, item.Name).FontSize(12).SingleLine()
					}
				case "size":
					if !item.IsDir {
						ui.Text(c, formatBytes(item.Size)).FontSize(11).TextColor(base.TextMuted).SingleLine()
					}
				case "date":
					ui.Text(c, time.Unix(item.ModTime, 0).Format("2006-01-02 15:04:05")).FontSize(11).TextColor(base.TextMuted).SingleLine()
				}
			})
			a.cardPointer(cell, i)
			if wasChosen && name != (ui.Element{}) && pointerOver(cell, name) && cell.Clicked() && !cell.DoubleClicked() {
				a.armSlowRename(c, i, wasChosen)
			}
			if item.IsDir {
				a.folderDropTarget(c, cell, item.Path)
			}
			cell.ContextMenu(func(m *ui.Menu) { a.mediaContextMenu(m, i) })
			if a.isCut(item.Path) {
				cell.Opacity(0.4)
			}
		})
		table.Grow(1).TrackScroll(&a.browseScroll)
	})
	if a.tableSort != before {
		if sort := sortForTable(a.tableSort); sort != "" && sort != a.Sort {
			a.Sort = sort
			a.refresh()
		}
	}
	a.keepColumns(c, list.Columns)
	first, last := list.Visible()
	a.prefetchAround(a.Root, first, last, len(a.Visible), thumbPrefetchExtra)
}

// keepColumns saves the widths and order the user gave the columns, once
// a resize settles, so a drag does not write the preferences every frame.
func (a *appState) keepColumns(c *ui.Context, cols ui.TableLayout) {
	saved := a.Profiles.DetailColumns
	if slices.Equal(cols.Order, saved.Order) && maps.Equal(cols.Widths, saved.Widths) {
		a.columnsAt = time.Time{}
		return
	}
	if a.columnsAt.IsZero() {
		a.columnsAt = time.Now()
	}
	if wait := 600*time.Millisecond - time.Since(a.columnsAt); wait > 0 {
		c.After(wait)
		return
	}
	a.columnsAt = time.Time{}
	a.Profiles.DetailColumns = ui.TableLayout{Order: slices.Clone(cols.Order), Widths: maps.Clone(cols.Widths)}
	savePrefs(a.Profiles)
}
