package main

import (
	"math/rand"
	"testing"
)

// referenceWaterfallGroups is the old linear shortest-column scan.
func referenceWaterfallGroups(aspects []float32, cols int) [][]int {
	if cols < 1 {
		cols = 1
	}
	groups := make([][]int, cols)
	heights := make([]float32, cols)
	for i, ar := range aspects {
		if ar <= 0 {
			ar = 1
		}
		best := 0
		for j := 1; j < cols; j++ {
			if heights[j] < heights[best] {
				best = j
			}
		}
		groups[best] = append(groups[best], i)
		heights[best] += 1 / ar
	}
	return groups
}

func TestWaterfallGroupsMatchLinearScan(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, cols := range []int{1, 2, 4, 8, 16} {
		for _, n := range []int{0, 1, 7, 64, 1000} {
			aspects := make([]float32, n)
			for i := range aspects {
				aspects[i] = float32(rng.Intn(20)+1) / 10
			}
			got := waterfallGroups(aspects, cols)
			want := referenceWaterfallGroups(aspects, cols)
			if len(got) != len(want) {
				t.Fatalf("cols=%d n=%d groups=%d want=%d", cols, n, len(got), len(want))
			}
			for c := range got {
				if len(got[c]) != len(want[c]) {
					t.Fatalf("cols=%d n=%d column %d lengths %d/%d", cols, n, c, len(got[c]), len(want[c]))
				}
				for i, idx := range got[c] {
					if idx != want[c][i] {
						t.Fatalf("cols=%d n=%d column %d item %d: %d want %d", cols, n, c, i, idx, want[c][i])
					}
				}
			}
		}
	}
}
