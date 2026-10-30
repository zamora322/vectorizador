package topology

import (
	"image"
	"image/color"
	"testing"

	"github.com/zamora322/vectorizador/pkg/palette"
	"github.com/zamora322/vectorizador/pkg/segmentation"
)

func TestTraceTopology(t *testing.T) {
	// Create a 10x10 image with a 4x4 square in the middle
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	for y := 3; y < 7; y++ {
		for x := 3; x < 7; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255})
		}
	}

	pal := palette.Palette{
		{R: 0, G: 0, B: 0},
		{R: 255, G: 255, B: 255},
	}

	sm := segmentation.SegmentImage(img, pal, 1, false)
	graph := TraceTopology(sm)

	if len(graph.Edges) == 0 {
		t.Fatalf("Expected boundary edges, got 0")
	}

	// Verify that all region loops are strictly closed cycles
	for regID, loops := range graph.RegionLoops {
		for i, l := range loops {
			if len(l.Edges) == 0 {
				t.Errorf("Region %d loop %d has 0 edges", regID, i)
				continue
			}
			p0 := l.Edges[0].Points[0]
			pEnd := l.Edges[len(l.Edges)-1].Points[len(l.Edges[len(l.Edges)-1].Points)-1]
			if p0.Distance(pEnd) > 1e-4 {
				t.Errorf("Region %d loop %d is NOT closed: starts at %v, ends at %v", regID, i, p0, pEnd)
			}
		}
	}
}
