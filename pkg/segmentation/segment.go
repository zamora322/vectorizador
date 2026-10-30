package segmentation

import (
	"image"
	"math"
	"runtime"
	"sync"

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

	// Step 1: Quantize each pixel to its nearest palette color index using multi-core parallel execution
	colorGrid := QuantizeImageColors(img, pal)

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
		if colorIdx < 0 {
			sm.Segments[id] = &Segment{
				ID:          id,
				ColorIndex:  -1,
				Color:       palette.RGB{R: 0, G: 0, B: 0},
				PixelCount:  count,
				Transparent: true,
			}
		} else {
			sm.Segments[id] = &Segment{
				ID:          id,
				ColorIndex:  colorIdx,
				Color:       sm.Palette[colorIdx],
				PixelCount:  count,
				Transparent: false,
			}
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
			// Do not merge an opaque segment into transparency
			if !seg.Transparent && nSeg.Transparent {
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

// QuantizeImageColors maps each pixel of img to the nearest palette color index.
// Optimized using multi-core parallel processing, direct Pix memory access, and an exact L1 color cache.
func QuantizeImageColors(img image.Image, pal palette.Palette) []int {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	colorGrid := make([]int, w*h)

	// Precompute Lab coordinates for palette once
	palLabs := make([]palette.Lab, len(pal))
	for i, c := range pal {
		palLabs[i] = palette.RGBToLab(c)
	}

	rgbaImg, isRGBA := img.(*image.RGBA)
	nrgbaImg, isNRGBA := img.(*image.NRGBA)

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > h {
		workers = h
	}

	rowsPerWorker := (h + workers - 1) / workers
	var wg sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		startY := worker * rowsPerWorker
		endY := startY + rowsPerWorker
		if endY > h {
			endY = h
		}
		if startY >= h {
			break
		}

		wg.Add(1)
		go func(sy, ey int) {
			defer wg.Done()

			// Thread-local direct-mapped cache (4096 entries)
			// Exact 24-bit RGB check ensures 100% mathematical precision with zero loss
			type cacheEntry struct {
				rgb   palette.RGB
				idx   int
				valid bool
			}
			cache := make([]cacheEntry, 4096)

			for y := sy; y < ey; y++ {
				rowOffset := y * w
				imgY := bounds.Min.Y + y
				for x := 0; x < w; x++ {
					imgX := bounds.Min.X + x
					var r, g, b, a uint32

					if isRGBA {
						off := rgbaImg.PixOffset(imgX, imgY)
						r = uint32(rgbaImg.Pix[off]) << 8
						g = uint32(rgbaImg.Pix[off+1]) << 8
						b = uint32(rgbaImg.Pix[off+2]) << 8
						a = uint32(rgbaImg.Pix[off+3]) << 8
					} else if isNRGBA {
						off := nrgbaImg.PixOffset(imgX, imgY)
						r = uint32(nrgbaImg.Pix[off]) << 8
						g = uint32(nrgbaImg.Pix[off+1]) << 8
						b = uint32(nrgbaImg.Pix[off+2]) << 8
						a = uint32(nrgbaImg.Pix[off+3]) << 8
					} else {
						r, g, b, a = img.At(imgX, imgY).RGBA()
					}

					if a < 128*257 {
						colorGrid[rowOffset+x] = -1
						continue
					}

					var red, green, blue uint8
					if a > 0 && a < 65535 {
						red = uint8((r * 65535 / a) >> 8)
						green = uint8((g * 65535 / a) >> 8)
						blue = uint8((b * 65535 / a) >> 8)
					} else {
						red = uint8(r >> 8)
						green = uint8(g >> 8)
						blue = uint8(b >> 8)
					}

					rgb := palette.RGB{R: red, G: green, B: blue}

					// Fast direct hash index
					hKey := (uint32(red)*31 + uint32(green)*17 + uint32(blue)) & 4095
					if cache[hKey].valid && cache[hKey].rgb == rgb {
						colorGrid[rowOffset+x] = cache[hKey].idx
						continue
					}

					// Exact CIE L*a*b* delta E calculation
					lab := palette.RGBToLab(rgb)
					bestDist := math.MaxFloat64
					bestIdx := 0
					for pi, pLab := range palLabs {
						d := palette.DeltaE(lab, pLab)
						if d < bestDist {
							bestDist = d
							bestIdx = pi
						}
					}

					cache[hKey] = cacheEntry{rgb: rgb, idx: bestIdx, valid: true}
					colorGrid[rowOffset+x] = bestIdx
				}
			}
		}(startY, endY)
	}

	wg.Wait()
	return colorGrid
}

// ToRGBA generates an image.RGBA visual representation of the segmented regions with native alpha.
// Optimized using direct contiguous byte slice writes without virtual function overhead.
func (sm *SegmentMap) ToRGBA() *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, sm.Width, sm.Height))
	pix := out.Pix
	total := sm.Width * sm.Height

	for i := 0; i < total; i++ {
		segID := sm.Grid[i]
		seg, ok := sm.Segments[segID]
		if !ok || seg.Transparent {
			continue // newly allocated bytes in image.NewRGBA are already 0,0,0,0
		}
		off := i * 4
		pix[off] = seg.Color.R
		pix[off+1] = seg.Color.G
		pix[off+2] = seg.Color.B
		pix[off+3] = 255
	}
	return out
}
