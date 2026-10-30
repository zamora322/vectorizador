package palette

import (
	"image"
	"image/color"
	"testing"
)

func TestRGBToLabAndBack(t *testing.T) {
	testColors := []RGB{
		{R: 255, G: 0, B: 0},     // Pure Red
		{R: 0, G: 255, B: 0},     // Pure Green
		{R: 0, G: 0, B: 255},     // Pure Blue
		{R: 255, G: 255, B: 255}, // White
		{R: 0, G: 0, B: 0},       // Black
		{R: 128, G: 128, B: 128}, // Gray
		{R: 245, G: 180, B: 40},  // Yellowish
	}

	for _, c := range testColors {
		lab := RGBToLab(c)
		recovered := LabToRGB(lab)

		diffR := int(c.R) - int(recovered.R)
		diffG := int(c.G) - int(recovered.G)
		diffB := int(c.B) - int(recovered.B)

		if diffR < 0 {
			diffR = -diffR
		}
		if diffG < 0 {
			diffG = -diffG
		}
		if diffB < 0 {
			diffB = -diffB
		}

		if diffR > 1 || diffG > 1 || diffB > 1 {
			t.Errorf("Roundtrip failed for %v: got %v (diffs: %d, %d, %d)", c, recovered, diffR, diffG, diffB)
		}
	}
}

func TestExtractPalette(t *testing.T) {
	// Create a 100x100 synthetic image with 3 distinct color blocks
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x < 33 {
				img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255}) // Red
			} else if x < 66 {
				img.Set(x, y, color.RGBA{R: 0, G: 255, B: 0, A: 255}) // Green
			} else {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255}) // Blue
			}
		}
	}

	pal := ExtractPalette(img, 3, 20)
	if len(pal) != 3 {
		t.Fatalf("Expected 3 colors, got %d", len(pal))
	}

	// Verify all 3 primary colors are matched closely
	for _, target := range []RGB{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}} {
		nearest := pal.NearestColor(target)
		d := DeltaERGB(target, nearest)
		if d > 5.0 {
			t.Errorf("Palette missing target color %v: closest was %v (dist: %f)", target, nearest, d)
		}
	}
}

func TestDetectOptimalColorCount(t *testing.T) {
	// Create an image with exactly 4 distinct colors: Red, Green, Blue, Yellow
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x < 50 && y < 50 {
				img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else if x >= 50 && y < 50 {
				img.Set(x, y, color.RGBA{R: 0, G: 255, B: 0, A: 255})
			} else if x < 50 && y >= 50 {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 0, A: 255})
			}
		}
	}

	count := DetectOptimalColorCount(img)
	if count != 4 {
		t.Errorf("Expected 4 detected colors, got %d", count)
	}
}
