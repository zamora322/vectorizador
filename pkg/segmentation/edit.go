package segmentation

import (
	"github.com/zamora322/vectorizador/pkg/palette"
)

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
