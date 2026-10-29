package engine

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestEndToEndVectorize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 30, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 30; x++ {
			if (x-15)*(x-15)+(y-15)*(y-15) < 100 {
				img.Set(x, y, color.RGBA{R: 0, G: 128, B: 255, A: 255}) // Blue circle
			} else {
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255}) // White background
			}
		}
	}

	cfg := DefaultConfig()
	cfg.Colors = 2

	var progressReports []string
	res, err := Vectorize(img, cfg, func(stage string, percent int) {
		progressReports = append(progressReports, stage)
	})

	if err != nil {
		t.Fatalf("Vectorize failed: %v", err)
	}

	if len(res.Palette) != 2 {
		t.Errorf("Expected 2 colors in palette, got %d", len(res.Palette))
	}

	if !strings.Contains(res.SVG, "<svg") {
		t.Errorf("Invalid SVG generated")
	}

	if len(progressReports) < 5 {
		t.Errorf("Expected at least 5 progress reports, got %d", len(progressReports))
	}
}
