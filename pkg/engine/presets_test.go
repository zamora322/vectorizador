package engine

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestCanonicalModesAndQualityPresets(t *testing.T) {
	modes := []Mode{ModeLogoAA, ModeLogoSharp, ModePhoto}
	qualities := []Quality{QualityLow, QualityMedium, QualityHigh}

	for _, m := range modes {
		for _, q := range qualities {
			cfg := PresetConfig(m, q)

			if cfg.Mode != m {
				t.Errorf("Expected mode %s, got %s", m, cfg.Mode)
			}
			if cfg.Quality != q {
				t.Errorf("Expected quality %s, got %s", q, cfg.Quality)
			}
			if cfg.MinPixels <= 0 {
				t.Errorf("Expected MinPixels > 0 for %s/%s, got %d", m, q, cfg.MinPixels)
			}
			if cfg.Smoothness <= 0 || cfg.Smoothness > 1.0 {
				t.Errorf("Expected Smoothness in (0, 1] for %s/%s, got %f", m, q, cfg.Smoothness)
			}
			if cfg.CurveTolerance <= 0 {
				t.Errorf("Expected CurveTolerance > 0 for %s/%s, got %f", m, q, cfg.CurveTolerance)
			}

			// Mode specific invariant checks
			switch m {
			case ModeLogoAA:
				if !cfg.AntiAliasingReject {
					t.Errorf("ModeLogoAA must have AntiAliasingReject = true")
				}
				if !cfg.DetectCorners {
					t.Errorf("ModeLogoAA must have DetectCorners = true")
				}
			case ModeLogoSharp:
				if cfg.AntiAliasingReject {
					t.Errorf("ModeLogoSharp must have AntiAliasingReject = false")
				}
				if !cfg.DetectCorners {
					t.Errorf("ModeLogoSharp must have DetectCorners = true")
				}
			case ModePhoto:
				if cfg.AntiAliasingReject {
					t.Errorf("ModePhoto must have AntiAliasingReject = false")
				}
				if cfg.DetectCorners {
					t.Errorf("ModePhoto must have DetectCorners = false")
				}
			}
		}
	}
}

func TestModeAndQualityNormalization(t *testing.T) {
	tests := []struct {
		inputMode    string
		inputQual    string
		expectedMode Mode
		expectedQual Quality
	}{
		{"logo", "high", ModeLogoSharp, QualityHigh},
		{"pixel", "alta", ModeLogoSharp, QualityHigh},
		{"SHARP", "baja", ModeLogoSharp, QualityLow},
		{"photo", "low", ModePhoto, QualityLow},
		{"foto", "media", ModePhoto, QualityMedium},
		{"continuous", "std", ModePhoto, QualityMedium},
		{"logo_aa", "medium", ModeLogoAA, QualityMedium},
		{"aa", "max", ModeLogoAA, QualityHigh},
		{"blended", "min", ModeLogoAA, QualityLow},
		{"", "", ModeLogoAA, QualityMedium},
	}

	for _, tt := range tests {
		m := NormalizeMode(tt.inputMode)
		q := NormalizeQuality(tt.inputQual)
		if m != tt.expectedMode {
			t.Errorf("NormalizeMode(%q) = %s, expected %s", tt.inputMode, m, tt.expectedMode)
		}
		if q != tt.expectedQual {
			t.Errorf("NormalizeQuality(%q) = %s, expected %s", tt.inputQual, q, tt.expectedQual)
		}
	}
}

func TestVectorizeAllThreeCanonicalModes(t *testing.T) {
	// 40x40 synthetic image with shapes and gradient
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if x < 20 && y < 20 {
				img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else if x >= 20 && y < 20 {
				img.Set(x, y, color.RGBA{R: 0, G: 255, B: 0, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
			}
		}
	}

	modes := []Mode{ModeLogoAA, ModeLogoSharp, ModePhoto}
	for _, m := range modes {
		cfg := Config{
			Mode:    m,
			Quality: QualityHigh,
		}
		res, err := Vectorize(img, cfg, nil)
		if err != nil {
			t.Fatalf("Vectorize failed for mode %s: %v", m, err)
		}
		if !strings.Contains(res.SVG, "<svg") {
			t.Errorf("Mode %s did not generate valid SVG", m)
		}
		if res.Doc == nil || len(res.Doc.Layers) == 0 {
			t.Errorf("Mode %s did not produce vector layers", m)
		}
	}
}
