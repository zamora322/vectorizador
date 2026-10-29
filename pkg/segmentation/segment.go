package segmentation

import (
	"image"
	"image/color"

	"github.com/zamora322/vectorizador/pkg/palette"
)

// Segment represents a contiguous region sharing the same color.
type Segment struct {
	ID          int
	ColorIndex  int
	Color       palette.RGB
	PixelCount  int
	Transparent bool
}

// SegmentMap stores the spatial partitioning of pixels into segments.
type SegmentMap struct {
	Width    int
	Height   int
	Grid     []int // Flattened Width * Height array of Segment IDs
	Segments map[int]*Segment
	Palette  palette.Palette
}

// NewSegmentMap allocates a new segment map for dimensions w x h.
func NewSegmentMap(w, h int, pal palette.Palette) *SegmentMap {
	return &SegmentMap{
		Width:    w,
		Height:   h,
		Grid:     make([]int, w*h),
		Segments: make(map[int]*Segment),
		Palette:  pal,
	}
}

// At returns the segment ID at pixel (x, y).
func (sm *SegmentMap) At(x, y int) int {
	if x < 0 || x >= sm.Width || y < 0 || y >= sm.Height {
		return -1
	}
	return sm.Grid[y*sm.Width+x]
}

// Set sets the segment ID at pixel (x, y).
func (sm *SegmentMap) Set(x, y int, id int) {
	sm.Grid[y*sm.Width+x] = id
}

// SegmentImage partitions the image into connected components based on the palette.
func SegmentImage(img image.Image, pal palette.Palette, minPixels int, rejectAA bool) *SegmentMap {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	sm := NewSegmentMap(w, h, pal)

	// Step 1: Quantize each pixel to its nearest palette color index
	colorGrid := make([]int, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			if a < 128*257 {
				colorGrid[y*w+x] = len(pal) - 1
				continue
			}
			rgb := palette.RGB{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}
			colorGrid[y*w+x] = pal.NearestIndex(rgb)
		}
	}

	// Step 2: Connected Component Labeling using Disjoint Set Union-Find (4-connectivity)
	parent := make([]int, w*h)
	for i := range parent {
		parent[i] = i
	}

	var find func(i int) int
	find = func(i int) int {
		root := i
		for parent[root] != root {
			root = parent[root]
		}
		// Path compression
		curr := i
		for curr != root {
			nxt := parent[curr]
			parent[curr] = root
			curr = nxt
		}
		return root
	}

	union := func(i, j int) {
		rootI := find(i)
		rootJ := find(j)
		if rootI != rootJ {
			parent[rootJ] = rootI
		}
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			c := colorGrid[idx]

			// Check left neighbor
			if x > 0 && colorGrid[idx-1] == c {
				union(idx, idx-1)
			}
			// Check top neighbor
			if y > 0 && colorGrid[(y-1)*w+x] == c {
				union(idx, (y-1)*w+x)
			}
		}
	}

	// Assign canonical root IDs to Grid
	for i := range sm.Grid {
		sm.Grid[i] = find(i)
	}

	// Step 3: Anti-aliasing artifact rejection if requested
	if rejectAA {
		sm.RejectAntiAliasingArtifacts(colorGrid)
	}

	// Step 4: Re-calculate segments and apply minPixels threshold
	sm.rebuildSegments(colorGrid)
	if minPixels > 1 {
		sm.filterSmallSegments(minPixels)
		sm.rebuildSegments(colorGrid)
	}

	return sm
}

func (sm *SegmentMap) rebuildSegments(colorGrid []int) {
	sm.Segments = make(map[int]*Segment)
	counts := make(map[int]int)

	for _, id := range sm.Grid {
		counts[id]++
	}

	for id, count := range counts {
		colorIdx := colorGrid[id]
		sm.Segments[id] = &Segment{
			ID:         id,
			ColorIndex: colorIdx,
			Color:      sm.Palette[colorIdx],
			PixelCount: count,
		}
	}
}

// filterSmallSegments merges segments smaller than minPixels into their most compatible neighbor.
// Optimized to run in O(W*H) instead of O(S * W * H).
func (sm *SegmentMap) filterSmallSegments(minPixels int) {
	w, h := sm.Width, sm.Height

	// 1. Collect pixel coordinates ONLY for small segments
	smallPixels := make(map[int][][2]int)
	for id, seg := range sm.Segments {
		if seg.PixelCount < minPixels {
			smallPixels[id] = make([][2]int, 0, seg.PixelCount)
		}
	}

	if len(smallPixels) == 0 {
		return
	}

	// Single O(W*H) pass to locate coordinates of small segments
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			id := sm.At(x, y)
			if pts, ok := smallPixels[id]; ok {
				smallPixels[id] = append(pts, [2]int{x, y})
			}
		}
	}

	// 2. Merge each small segment into its closest neighboring segment
	dirs := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	for id, pts := range smallPixels {
		seg := sm.Segments[id]
		if seg == nil {
			continue
		}

		neighborCounts := make(map[int]int)
		for _, pt := range pts {
			x, y := pt[0], pt[1]
			for _, d := range dirs {
				nx, ny := x+d[0], y+d[1]
				nid := sm.At(nx, ny)
				if nid != -1 && nid != id {
					neighborCounts[nid]++
				}
			}
		}

		bestNeighbor := -1
		bestDist := 999999.0
		for nid := range neighborCounts {
			nSeg, exists := sm.Segments[nid]
			if !exists {
				continue
			}
			dist := palette.DeltaERGB(seg.Color, nSeg.Color)
			if dist < bestDist {
				bestDist = dist
				bestNeighbor = nid
			}
		}

		if bestNeighbor != -1 {
			for _, pt := range pts {
				sm.Set(pt[0], pt[1], bestNeighbor)
			}
		}
	}
}

// ToRGBA generates an image.RGBA visual representation of the segmented regions.
func (sm *SegmentMap) ToRGBA() *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, sm.Width, sm.Height))
	for y := 0; y < sm.Height; y++ {
		for x := 0; x < sm.Width; x++ {
			segID := sm.At(x, y)
			seg, ok := sm.Segments[segID]
			if !ok {
				continue
			}
			if seg.Transparent {
				// Checkerboard pattern for transparent regions
				isEven := ((x/8)+(y/8))%2 == 0
				var v uint8 = 200
				if isEven {
					v = 240
				}
				out.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
			} else {
				out.Set(x, y, color.RGBA{
					R: seg.Color.R,
					G: seg.Color.G,
					B: seg.Color.B,
					A: 255,
				})
			}
		}
	}
	return out
}
