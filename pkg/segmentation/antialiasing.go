package segmentation

import (
	"math"

	"github.com/zamora322/vectorizador/pkg/palette"
)

// RejectAntiAliasingArtifacts scans for thin boundary transition slivers and merges
// them into their flanking dominant regions in O(W*H) time.
func (sm *SegmentMap) RejectAntiAliasingArtifacts(colorGrid []int) {
	w, h := sm.Width, sm.Height

	// Track neighbor adjacencies and pixel counts for each segment
	adjacencies := make(map[int]map[int]int)
	pixelSizes := make(map[int]int)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			id := sm.At(x, y)
			pixelSizes[id]++

			if _, ok := adjacencies[id]; !ok {
				adjacencies[id] = make(map[int]int)
			}

			// Check right and down
			if x+1 < w {
				nid := sm.At(x+1, y)
				if nid != id {
					adjacencies[id][nid]++
					if _, ok := adjacencies[nid]; !ok {
						adjacencies[nid] = make(map[int]int)
					}
					adjacencies[nid][id]++
				}
			}
			if y+1 < h {
				nid := sm.At(x, y+1)
				if nid != id {
					adjacencies[id][nid]++
					if _, ok := adjacencies[nid]; !ok {
						adjacencies[nid] = make(map[int]int)
					}
					adjacencies[nid][id]++
				}
			}
		}
	}

	// Identify thin transition segments
	type mergeRule struct {
		target1, target2 int
	}
	artifactsToMerge := make(map[int]mergeRule)

	for id, neighbors := range adjacencies {
		size := pixelSizes[id]
		// Anti-aliasing slivers are small and sandwiched between exactly 2 dominant neighbors
		if size > 5000 || len(neighbors) != 2 {
			continue
		}

		var n1, n2 int
		i := 0
		for nid := range neighbors {
			if i == 0 {
				n1 = nid
			} else {
				n2 = nid
			}
			i++
		}

		// Color colinearity check in Lab space
		cSelf := palette.RGBToLab(sm.Palette[colorGrid[id]])
		cN1 := palette.RGBToLab(sm.Palette[colorGrid[n1]])
		cN2 := palette.RGBToLab(sm.Palette[colorGrid[n2]])

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
		for x := 0; x < w; x++ {
			id := sm.At(x, y)
			rule, isArtifact := artifactsToMerge[id]
			if !isArtifact {
				continue
			}

			// Assign to target1 or target2 based on 3x3 local neighborhood
			countN1, countN2 := 0, 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nid := sm.At(x+dx, y+dy)
					if nid == rule.target1 {
						countN1++
					} else if nid == rule.target2 {
						countN2++
					}
				}
			}

			if countN1 >= countN2 {
				sm.Set(x, y, rule.target1)
			} else {
				sm.Set(x, y, rule.target2)
			}
		}
	}
}
