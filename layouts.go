package main

import (
	"github.com/egoist/mygo/ui"
)

// waterfallView packs items into N columns, each appended to the shortest.
func (a *appState) waterfallView(c *ui.Context) {
	root := a.Root
	const cols = 4
	colItems := make([][]int, cols)
	heights := make([]float32, cols)
	for i := range a.Visible {
		item := &a.Items[a.Visible[i]]
		var ar float32 = 1
		if item.Info.Height > 0 && item.Info.Width > 0 {
			ar = float32(item.Info.Width) / float32(item.Info.Height)
		}
		if ar == 0 {
			ar = 1
		}
		// pick shortest column
		best := 0
		for j := 1; j < cols; j++ {
			if heights[j] < heights[best] {
				best = j
			}
		}
		colItems[best] = append(colItems[best], i)
		heights[best] += 1.0 / ar
		if !item.InfoLoaded {
			go a.loadInfo(a.Visible[i])
		}
	}
	row := ui.Row(c).Fill().Gap(8).Padding(8)
	row.Children(func() {
		for ci := 0; ci < cols; ci++ {
			col := ui.Column(c).Grow(1).Gap(8)
			col.Children(func() {
				for _, i := range colItems[ci] {
					a.thumbCard(c, root, i)
				}
			})
		}
	})
}

// justifiedView packs items into rows, each row stretched to fill the width.
func (a *appState) justifiedView(c *ui.Context) {
	t := c.Theme()
	root := a.Root
	const rowWidth = 1280.0
	const gap = 8.0
	var rows [][]int
	var row []int
	var rowAR float64
	for i := range a.Visible {
		item := &a.Items[a.Visible[i]]
		var ar float64 = 1
		if item.Info.Height > 0 && item.Info.Width > 0 {
			ar = float64(item.Info.Width) / float64(item.Info.Height)
		}
		if ar == 0 {
			ar = 1
		}
		row = append(row, i)
		rowAR += ar
		if !item.InfoLoaded {
			go a.loadInfo(a.Visible[i])
		}
		// pack when row aspect ratio exceeds target
		if rowAR > rowWidth/float64(a.Thumb) {
			rows = append(rows, row)
			row = nil
			rowAR = 0
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	col := ui.Column(c).Fill().Gap(gap).Padding(8)
	col.Children(func() {
		for _, r := range rows {
			r := r
			rr := ui.Row(c).Gap(gap).Grow(1)
			rr.Children(func() {
				for _, i := range r {
					a.thumbCard(c, root, i)
				}
			})
		}
	})
	_ = t
}

func (a *appState) thumbCard(c *ui.Context, root string, i int) {
	t := c.Theme()
	item := &a.Items[a.Visible[i]]
	card := ui.Column(c).Grow(1).Margin(2).Padding(2).Gap(2).Radius(6).Background(t.Surface)
	card.Children(func() {
		thumb := ui.Box(c).Grow(1).Radius(4).Background(t.Background).Justify(ui.Center).AlignItems(ui.Center)
		thumb.Children(func() {
			if item.Bitmap != nil {
				ui.Image(c, item.Bitmap).Fit(ui.Contain).Grow(1)
			} else if item.Failed {
				ui.Icon(c, videoIcon).FontSize(24).TextColor(t.TextMuted)
			} else {
				ui.Spinner(c)
			}
		})
		ui.Text(c, item.Name).FontSize(11).MaxLines(1).SingleLine().TextAlign(ui.Start)
	})
	card.ContextMenu(func(m *ui.Menu) { a.mediaContextMenu(m, i) })
	if !item.Decoded && !item.Failed && !item.Loading {
		a.loadThumb(root, a.Visible[i])
	}
}
