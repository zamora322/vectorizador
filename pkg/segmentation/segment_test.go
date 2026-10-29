package segmentation

import (
	"image"
	"image/color"
	"testing"

	"github.com/zamora322/vectorizador/pkg/palette"
)

func TestSegmentationAndFiltering(t *testing.T) {
	// Create a 20x20 test image with a 1-pixel noise speckle in the middle
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255}) // White
		}
	}
	// Left half black
	for y := 0; y < 20; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255}) // Black
		}
	}
	// Add 1 single white pixel in black area (speckle)
	img.Set(5, 5, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	pal := palette.Palette{
		{R: 0, G: 0, B: 0},
		{R: 255, G: 255, B: 255},
	}

	// Without filter: speckle should exist
	sm1 := SegmentImage(img, pal, 1, false)
	speckleID := sm1.At(5, 5)
	blackID := sm1.At(4, 5)
	if speckleID == blackID {
		t.Errorf("Expected speckle to be a distinct segment without minPixels filter")
	}

	// With minPixels filter (minPixels = 4): speckle should be absorbed
	sm2 := SegmentImage(img, pal, 4, false)
	speckleID2 := sm2.At(5, 5)
	blackID2 := sm2.At(4, 5)
	if speckleID2 != blackID2 {
		t.Errorf("Expected speckle to be absorbed into black region with minPixels=4, got %d vs %d", speckleID2, blackID2)
	}
}
