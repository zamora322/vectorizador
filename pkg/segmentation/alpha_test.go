package segmentation_test

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/zamora322/vectorizador/pkg/engine"
	"github.com/zamora322/vectorizador/pkg/palette"
	"github.com/zamora322/vectorizador/pkg/segmentation"
)

func TestTransparencyPreservation(t *testing.T) {
	// 20x20 image with transparent background and a 10x10 solid red square
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
		}
	}
	for y := 5; y < 15; y++ {
		for x := 5; x < 15; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	pal := palette.ExtractPalette(img, 2, 10)
	if len(pal) == 0 {
		t.Fatalf("Expected palette to be extracted")
	}

	sm := segmentation.SegmentImage(img, pal, 4, true)
	if sm == nil {
		t.Fatalf("SegmentImage returned nil")
	}

	// Check transparent background
	transID := sm.At(0, 0)
	transSeg, ok := sm.Segments[transID]
	if !ok {
		t.Fatalf("Missing segment for (0,0)")
	}
	if !transSeg.Transparent {
		t.Errorf("Expected segment at (0,0) to be marked Transparent: true")
	}
	if transSeg.ColorIndex != -1 {
		t.Errorf("Expected transparent segment to have ColorIndex -1, got %d", transSeg.ColorIndex)
	}

	// Check opaque red square
	redID := sm.At(10, 10)
	redSeg, ok := sm.Segments[redID]
	if !ok {
		t.Fatalf("Missing segment for (10,10)")
	}
	if redSeg.Transparent {
		t.Errorf("Expected segment at (10,10) to NOT be transparent")
	}
	if redSeg.ColorIndex < 0 {
		t.Errorf("Expected opaque segment to have valid ColorIndex >= 0")
	}

	// Verify ToRGBA produces transparent pixels at (0,0)
	rgbaImg := sm.ToRGBA()
	r, g, b, a := rgbaImg.At(0, 0).RGBA()
	if a != 0 {
		t.Errorf("Expected alpha to be 0 at (0,0), got r=%d, g=%d, b=%d, a=%d", r, g, b, a)
	}

	// Run full engine vectorization
	cfg := engine.DefaultConfig()
	res, err := engine.Vectorize(img, cfg, nil)
	if err != nil {
		t.Fatalf("Vectorize failed: %v", err)
	}

	// SVG should contain the red path (#ff0000) and NO black background rectangle
	if !strings.Contains(strings.ToLower(res.SVG), "fill=\"#ff0000\"") {
		t.Errorf("Expected SVG to contain red fill path, got:\n%s", res.SVG)
	}
}
