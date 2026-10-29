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

	// Verify that the inner region has a closed loop
	var foundSquareLoop bool
	for _, loops := range graph.RegionLoops {
		for _, l := range loops {
			if len(l.Edges) > 0 {
				foundSquareLoop = true
			}
		}
	}

	if !foundSquareLoop {
		t.Errorf("Expected to find closed region loop for square")
	}
}
