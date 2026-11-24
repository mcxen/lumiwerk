package main

// Flow layout cache.
//
// waterfallView and justifiedView pack every item by aspect ratio. Packing is
// O(n) and ran on every frame, including frames that only scroll or hover.
// The packing depends only on the visible items, their aspects, the view
// width and the thumbnail size, so it is kept until one of those changes:
// layoutGen is bumped whenever Visible is rebuilt or an item's dimensions
// change, and width and thumb are part of the key.

type flowLayoutKey struct {
	gen   uint64
	n     int
	width float32
	thumb float64
}

type flowLayoutCache struct {
	waterKey  flowLayoutKey
	water     []flowBox
	waterH    float32
	waterCols [][]flowBox
	justKey   flowLayoutKey
	just      []flowBox
	justH     float32
	// builds counts packings, for tests.
	builds int
}

// noteLayoutChanged invalidates the flow layout cache.
func (a *appState) noteLayoutChanged() { a.layoutGen++ }

func flowContentHeight(boxes []flowBox) float32 {
	var contentH float32
	for _, b := range boxes {
		if bottom := b.Y + b.H + flowPad; bottom > contentH {
			contentH = bottom
		}
	}
	return contentH
}

func (a *appState) cachedWaterfallData(width float32) ([]flowBox, float32, [][]flowBox) {
	// 容忍 10px 误差
	quantizedWidth := float32(int(width/10) * 10)
	key := flowLayoutKey{a.layoutGen, len(a.Visible), quantizedWidth, a.Thumb}
	if a.flowCache.water == nil || a.flowCache.waterKey != key {
		boxes := waterfallFrames(a.visibleAspects(), width, float32(a.Thumb))
		cols := waterfallColumns(width, float32(a.Thumb))
		colBuckets := make([][]flowBox, cols)
		colX := make([]float32, cols)
		flex, _ := waterfallColumnWidth(width, cols)
		for ci := range colX {
			colX[ci] = flowPad + float32(ci)*(flex+flowGap)
		}
		for _, b := range boxes {
			ci := 0
			if flex+flowGap > 0 {
				ci = int((b.X - flowPad + 0.5) / (flex + flowGap))
			}
			if ci < 0 {
				ci = 0
			} else if ci >= cols {
				ci = cols - 1
			}
			colBuckets[ci] = append(colBuckets[ci], b)
		}
		a.flowCache.water = boxes
		a.flowCache.waterH = flowContentHeight(boxes)
		a.flowCache.waterCols = colBuckets
		a.flowCache.waterKey = key
		a.flowCache.builds++
	}
	return a.flowCache.water, a.flowCache.waterH, a.flowCache.waterCols
}

func (a *appState) cachedWaterfall(width float32) []flowBox {
	boxes, _, _ := a.cachedWaterfallData(width)
	return boxes
}

func (a *appState) cachedJustifiedData(width float32) ([]flowBox, float32) {
	// 容忍 10px 误差，避免窗口微调触发全量重算
	quantizedWidth := float32(int(width/10) * 10)
	key := flowLayoutKey{a.layoutGen, len(a.Visible), quantizedWidth, a.Thumb}
	if a.flowCache.just == nil || a.flowCache.justKey != key {
		boxes := justifiedFrames(a.visibleAspects(), width, float32(a.Thumb))
		if boxes == nil {
			boxes = []flowBox{}
		}
		a.flowCache.just = boxes
		a.flowCache.justH = flowContentHeight(boxes)
		a.flowCache.justKey = key
		a.flowCache.builds++
	}
	return a.flowCache.just, a.flowCache.justH
}

func (a *appState) cachedJustified(width float32) []flowBox {
	boxes, _ := a.cachedJustifiedData(width)
	return boxes
}
