package segmentation

import (
	"math"

	"github.com/zamora322/vectorizador/pkg/palette"
)

// Point represents a 2D integer coordinate on the pixel grid.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Clone creates a deep copy of the SegmentMap and all its segments.
func (sm *SegmentMap) Clone() *SegmentMap {
	newSM := &SegmentMap{
		Width:    sm.Width,
		Height:   sm.Height,
		Grid:     make([]int, len(sm.Grid)),
		Segments: make(map[int]*Segment, len(sm.Segments)),
		Palette:  make(palette.Palette, len(sm.Palette)),
	}
	copy(newSM.Grid, sm.Grid)
	copy(newSM.Palette, sm.Palette)

	for id, s := range sm.Segments {
		newSM.Segments[id] = &Segment{
			ID:          s.ID,
			ColorIndex:  s.ColorIndex,
			Color:       s.Color,
			PixelCount:  s.PixelCount,
			Transparent: s.Transparent,
		}
	}
	return newSM
}

// RecolorSegmentAt updates the color of the segment located at (x, y).
func (sm *SegmentMap) RecolorSegmentAt(x, y int, newColor palette.RGB) bool {
	id := sm.At(x, y)
	if id == -1 {
		return false
	}
	seg, ok := sm.Segments[id]
	if !ok {
		return false
	}

	// Find or add color in palette
	colorIdx := sm.Palette.NearestIndex(newColor)
	if colorIdx == -1 || sm.Palette[colorIdx] != newColor {
		sm.Palette = append(sm.Palette, newColor)
		colorIdx = len(sm.Palette) - 1
	}

	seg.Color = newColor
	seg.ColorIndex = colorIdx
	seg.Transparent = false
	return true
}

// MergeSegmentsAt unites the segment at (x1, y1) with the segment at (x2, y2).
func (sm *SegmentMap) MergeSegmentsAt(x1, y1, x2, y2 int) bool {
	id1 := sm.At(x1, y1)
	id2 := sm.At(x2, y2)
	if id1 == -1 || id2 == -1 || id1 == id2 {
		return false
	}

	seg1, ok1 := sm.Segments[id1]
	seg2, ok2 := sm.Segments[id2]
	if !ok1 || !ok2 {
		return false
	}

	// Reassign all pixels of id2 to id1
	for i, gid := range sm.Grid {
		if gid == id2 {
			sm.Grid[i] = id1
		}
	}

	seg1.PixelCount += seg2.PixelCount
	delete(sm.Segments, id2)
	return true
}

// EraseSegmentAt marks the segment located at (x, y) as transparent.
func (sm *SegmentMap) EraseSegmentAt(x, y int) bool {
	id := sm.At(x, y)
	if id == -1 {
		return false
	}
	seg, ok := sm.Segments[id]
	if !ok {
		return false
	}

	seg.Transparent = true
	return true
}

// ZapSegmentAt destroys the artifact or boundary segment at (x, y) and redistributes
// its pixels toward adjacent segments based on perceptual CIE L*a*b* color distance.
func (sm *SegmentMap) ZapSegmentAt(x, y int) bool {
	targetID := sm.At(x, y)
	if targetID == -1 {
		return false
	}
	targetSeg, ok := sm.Segments[targetID]
	if !ok {
		return false
	}

	// Identify all pixels of target segment
	isTarget := make([]bool, len(sm.Grid))
	hasTarget := false
	for i, gid := range sm.Grid {
		if gid == targetID {
			isTarget[i] = true
			hasTarget = true
		}
	}
	if !hasTarget {
		return false
	}

	targetLab := palette.RGBToLab(targetSeg.Color)
	neighborDeltaE := make(map[int]float64)
	dxs := []int{0, 0, -1, 1}
	dys := []int{-1, 1, 0, 0}

	// Cache perceptual distance to each contacting foreign neighbor
	for i, isT := range isTarget {
		if !isT {
			continue
		}
		px := i % sm.Width
		py := i / sm.Width
		for k := 0; k < 4; k++ {
			nx := px + dxs[k]
			ny := py + dys[k]
			if nx >= 0 && nx < sm.Width && ny >= 0 && ny < sm.Height {
				nID := sm.At(nx, ny)
				if nID != -1 && nID != targetID {
					if _, seen := neighborDeltaE[nID]; !seen {
						if nSeg, exists := sm.Segments[nID]; exists {
							if nSeg.Transparent {
								neighborDeltaE[nID] = 1000.0 // Higher penalty for transparent unless only choice
							} else {
								nLab := palette.RGBToLab(nSeg.Color)
								neighborDeltaE[nID] = palette.DeltaE(targetLab, nLab)
							}
						}
					}
				}
			}
		}
	}

	// If isolated with no surrounding segments, mark as transparent
	if len(neighborDeltaE) == 0 {
		targetSeg.Transparent = true
		return true
	}

	// Seed BFS with border pixels contacting foreign neighbors
	type seedItem struct {
		gridIdx    int
		neighborID int
		cost       float64
	}
	var queue []seedItem
	visited := make([]bool, len(sm.Grid))

	for i, isT := range isTarget {
		if !isT {
			continue
		}
		px := i % sm.Width
		py := i / sm.Width

		bestNID := -1
		bestDist := 1e9

		for k := 0; k < 4; k++ {
			nx := px + dxs[k]
			ny := py + dys[k]
			if nx >= 0 && nx < sm.Width && ny >= 0 && ny < sm.Height {
				nID := sm.At(nx, ny)
				if nID != -1 && nID != targetID {
					if d, ok := neighborDeltaE[nID]; ok && d < bestDist {
						bestDist = d
						bestNID = nID
					}
				}
			}
		}

		if bestNID != -1 {
			queue = append(queue, seedItem{gridIdx: i, neighborID: bestNID, cost: bestDist})
			visited[i] = true
			sm.Grid[i] = bestNID
			if nSeg, ok := sm.Segments[bestNID]; ok {
				nSeg.PixelCount++
			}
		}
	}

	// Multi-source BFS flood inward to reassign interior pixels
	for head := 0; head < len(queue); head++ {
		curr := queue[head]
		cx := curr.gridIdx % sm.Width
		cy := curr.gridIdx / sm.Width

		for k := 0; k < 4; k++ {
			nx := cx + dxs[k]
			ny := cy + dys[k]
			if nx >= 0 && nx < sm.Width && ny >= 0 && ny < sm.Height {
				nIdx := ny*sm.Width + nx
				if isTarget[nIdx] && !visited[nIdx] {
					visited[nIdx] = true
					sm.Grid[nIdx] = curr.neighborID
					if nSeg, ok := sm.Segments[curr.neighborID]; ok {
						nSeg.PixelCount++
					}
					queue = append(queue, seedItem{gridIdx: nIdx, neighborID: curr.neighborID, cost: curr.cost + 1.0})
				}
			}
		}
	}

	// Delete destroyed target segment
	delete(sm.Segments, targetID)
	return true
}

// PencilStroke paints a continuous raster line between points with the given strokeWidth and color.
func (sm *SegmentMap) PencilStroke(points []Point, strokeWidth int, strokeColor palette.RGB) bool {
	if len(points) == 0 {
		return false
	}
	if strokeWidth < 1 {
		strokeWidth = 1
	}

	// Find or add color to palette
	colorIdx := sm.Palette.NearestIndex(strokeColor)
	if colorIdx == -1 || sm.Palette[colorIdx] != strokeColor {
		sm.Palette = append(sm.Palette, strokeColor)
		colorIdx = len(sm.Palette) - 1
	}

	// Allocate a new unique segment ID
	maxID := 0
	for id := range sm.Segments {
		if id > maxID {
			maxID = id
		}
	}
	strokeID := maxID + 1
	newSeg := &Segment{
		ID:          strokeID,
		ColorIndex:  colorIdx,
		Color:       strokeColor,
		PixelCount:  0,
		Transparent: false,
	}
	sm.Segments[strokeID] = newSeg

	radius := float64(strokeWidth) / 2.0
	rInt := int(math.Ceil(radius))

	paintDisc := func(cx, cy int) {
		for dy := -rInt; dy <= rInt; dy++ {
			for dx := -rInt; dx <= rInt; dx++ {
				distSq := float64(dx*dx + dy*dy)
				if strokeWidth == 1 || distSq <= radius*radius+0.5 {
					x := cx + dx
					y := cy + dy
					if x >= 0 && x < sm.Width && y >= 0 && y < sm.Height {
						oldID := sm.At(x, y)
						if oldID != strokeID {
							if oldSeg, ok := sm.Segments[oldID]; ok {
								oldSeg.PixelCount--
								if oldSeg.PixelCount <= 0 {
									delete(sm.Segments, oldID)
								}
							}
							sm.Set(x, y, strokeID)
							newSeg.PixelCount++
						}
					}
				}
			}
		}
	}

	if len(points) == 1 {
		paintDisc(points[0].X, points[0].Y)
	} else {
		for i := 0; i < len(points)-1; i++ {
			p1 := points[i]
			p2 := points[i+1]
			// Bresenham's line algorithm
			x0, y0 := p1.X, p1.Y
			x1, y1 := p2.X, p2.Y
			dx := abs(x1 - x0)
			dy := -abs(y1 - y0)
			sx := 1
			if x0 > x1 {
				sx = -1
			}
			sy := 1
			if y0 > y1 {
				sy = -1
			}
			err := dx + dy

			for {
				paintDisc(x0, y0)
				if x0 == x1 && y0 == y1 {
					break
				}
				e2 := 2 * err
				if e2 >= dy {
					err += dy
					x0 += sx
				}
				if e2 <= dx {
					err += dx
					y0 += sy
				}
			}
		}
	}

	if newSeg.PixelCount == 0 {
		delete(sm.Segments, strokeID)
		return false
	}

	return true
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

