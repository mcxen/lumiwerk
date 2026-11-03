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
	waterKey flowLayoutKey
	water    []flowBox
	justKey  flowLayoutKey
	just     [][]int
	// builds counts packings, for tests.
	builds int
}

// noteLayoutChanged invalidates the flow layout cache.
func (a *appState) noteLayoutChanged() { a.layoutGen++ }

func (a *appState) cachedWaterfall(width float32) []flowBox {
	// 容忍 10px 误差
	quantizedWidth := float32(int(width/10) * 10)
	key := flowLayoutKey{a.layoutGen, len(a.Visible), quantizedWidth, a.Thumb}
	if a.flowCache.water == nil || a.flowCache.waterKey != key {
		a.flowCache.water = waterfallFrames(a.visibleAspects(), width, float32(a.Thumb))
		a.flowCache.waterKey = key
		a.flowCache.builds++
	}
	return a.flowCache.water
}

func (a *appState) cachedJustified(width float32) [][]int {
	// 容忍 10px 误差，避免窗口微调触发全量重算
	quantizedWidth := float32(int(width/10) * 10)
	key := flowLayoutKey{a.layoutGen, len(a.Visible), quantizedWidth, a.Thumb}
	if a.flowCache.just == nil || a.flowCache.justKey != key {
		aspects := make([]float64, len(a.Visible))
		for i, ar := range a.visibleAspects() {
			aspects[i] = float64(ar)
		}
		a.flowCache.just = justifiedRows(aspects, float64(width-flowPad*2), a.Thumb, float64(flowGap))
		if a.flowCache.just == nil {
			a.flowCache.just = [][]int{}
		}
		a.flowCache.justKey = key
		a.flowCache.builds++
	}
	return a.flowCache.just
}
