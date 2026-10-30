package segmentation

import (
	"math"

	"github.com/zamora322/vectorizador/pkg/palette"
)

type neighborTracker struct {
	n1, n2 int
	count  int // 0, 1, 2, or 3+ (disqualified)
}

func addNeighborPair(t *neighborTracker, nid int) {
	if t.count >= 3 {
		return
	}
	if t.count == 0 {
		t.n1 = nid
		t.count = 1
	} else if t.count == 1 {
		if t.n1 != nid {
			t.n2 = nid
			t.count = 2
		}
	} else if t.count == 2 {
		if t.n1 != nid && t.n2 != nid {
			t.count = 3
		}
	}
}

// RejectAntiAliasingArtifacts scans for thin boundary transition slivers and merges
// them into their flanking dominant regions in linear time without nested map allocations.
func (sm *SegmentMap) RejectAntiAliasingArtifacts(colorGrid []int) {
	w, h := sm.Width, sm.Height

	// Track neighbor adjacencies and pixel counts for each segment
	neighbors := make(map[int]*neighborTracker)
	pixelSizes := make(map[int]int)

	grid := sm.Grid
	for y := 0; y < h; y++ {
		rowOff := y * w
		for x := 0; x < w; x++ {
			id := grid[rowOff+x]
			pixelSizes[id]++

			// Check right neighbor
			if x+1 < w {
				nid := grid[rowOff+x+1]
				if nid != id {
					t1 := neighbors[id]
					if t1 == nil {
						t1 = &neighborTracker{}
						neighbors[id] = t1
					}
					addNeighborPair(t1, nid)

					t2 := neighbors[nid]
					if t2 == nil {
						t2 = &neighborTracker{}
						neighbors[nid] = t2
					}
					addNeighborPair(t2, id)
				}
			}

			// Check bottom neighbor
			if y+1 < h {
				nid := grid[rowOff+w+x]
				if nid != id {
					t1 := neighbors[id]
					if t1 == nil {
						t1 = &neighborTracker{}
						neighbors[id] = t1
					}
					addNeighborPair(t1, nid)

					t2 := neighbors[nid]
					if t2 == nil {
						t2 = &neighborTracker{}
						neighbors[nid] = t2
					}
					addNeighborPair(t2, id)
				}
			}
		}
	}

	// Identify thin transition segments
	type mergeRule struct {
		target1, target2 int
	}
	artifactsToMerge := make(map[int]mergeRule)

	// Precompute Lab coordinates for palette
	palLabs := make([]palette.Lab, len(sm.Palette))
	for i, c := range sm.Palette {
		palLabs[i] = palette.RGBToLab(c)
	}

	for id, t := range neighbors {
		size := pixelSizes[id]
		// Anti-aliasing slivers are small and sandwiched between exactly 2 dominant neighbors
		if size > 5000 || t.count != 2 {
			continue
		}

		n1, n2 := t.n1, t.n2

		// Transparent segments are not anti-aliasing color transitions
		if colorGrid[id] < 0 || colorGrid[n1] < 0 || colorGrid[n2] < 0 {
			continue
		}

		// Color colinearity check in Lab space
		cSelf := palLabs[colorGrid[id]]
		cN1 := palLabs[colorGrid[n1]]
		cN2 := palLabs[colorGrid[n2]]

		distDirect := palette.DeltaE(cN1, cN2)
		distThrough := palette.DeltaE(cN1, cSelf) + palette.DeltaE(cSelf, cN2)

		if distDirect > 10.0 && math.Abs(distThrough-distDirect) < 8.0 {
			artifactsToMerge[id] = mergeRule{target1: n1, target2: n2}
		}
	}

	if len(artifactsToMerge) == 0 {
		return
	}

	// Single pass over the image to resolve all flagged artifact pixels
	for y := 0; y < h; y++ {
		rowOff := y * w
		for x := 0; x < w; x++ {
			id := grid[rowOff+x]
			rule, isArtifact := artifactsToMerge[id]
			if !isArtifact {
				continue
			}

			// Assign to target1 or target2 based on 3x3 local neighborhood
			countN1, countN2 := 0, 0
			for dy := -1; dy <= 1; dy++ {
				ny := y + dy
				if ny < 0 || ny >= h {
					continue
				}
				nRowOff := ny * w
				for dx := -1; dx <= 1; dx++ {
					nx := x + dx
					if nx < 0 || nx >= w {
						continue
					}
					nid := grid[nRowOff+nx]
					if nid == rule.target1 {
						countN1++
					} else if nid == rule.target2 {
						countN2++
					}
				}
			}

			if countN1 >= countN2 {
				grid[rowOff+x] = rule.target1
			} else {
				grid[rowOff+x] = rule.target2
			}
		}
	}
}
