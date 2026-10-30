package engine

import (
	"strings"
)

// Mode defines the canonical image type category.
type Mode string

const (
	// ModeLogoAA is "Artwork with blended edges" (Anti-Aliased).
	// Targets logos, typography and graphics with sub-pixel edge blending.
	ModeLogoAA Mode = "logo_aa"

	// ModeLogoSharp is "Artwork without blended edges" (Sharp / Pixel / Aliased).
	// Targets pixel art, retro games, screenshots, and hard-edged indexed graphics.
	ModeLogoSharp Mode = "logo_sharp"

	// ModePhoto is "Photograph" (Continuous tone, organic curves, no sharp corners).
	// Targets photos, natural textures, portraits and paintings.
	ModePhoto Mode = "photo"
)

// Quality defines the complexity and detail preset level.
type Quality string

const (
	// QualityLow produces simpler paths, smaller file sizes, and aggressive noise suppression.
	QualityLow Quality = "low"

	// QualityMedium is the balanced default.
	QualityMedium Quality = "medium"

	// QualityHigh produces maximum fidelity and detail preservation.
	QualityHigh Quality = "high"
)

// NormalizeMode parses a mode string and returns the canonical Mode.
// It supports common aliases such as "logo", "sharp", "pixel", "photo", "foto", "aa".
func NormalizeMode(m string) Mode {
	s := strings.ToLower(strings.TrimSpace(m))
	switch s {
	case "logo_sharp", "logo", "sharp", "pixel", "pixel_art", "aliased", "hard":
		return ModeLogoSharp
	case "photo", "photograph", "foto", "continuous", "organic":
		return ModePhoto
	case "logo_aa", "logo-aa", "aa", "blended", "artwork", "":
		return ModeLogoAA
	default:
		return ModeLogoAA
	}
}

// NormalizeQuality parses a quality string and returns the canonical Quality.
// It supports Spanish and English aliases ("alta", "media", "baja", "high", "medium", "low").
func NormalizeQuality(q string) Quality {
	s := strings.ToLower(strings.TrimSpace(q))
	switch s {
	case "high", "alta", "hi", "h", "max":
		return QualityHigh
	case "low", "baja", "lo", "l", "min":
		return QualityLow
	case "medium", "media", "med", "m", "std", "":
		return QualityMedium
	default:
		return QualityMedium
	}
}

// Description returns a localized description of the image category.
func (m Mode) Description() string {
	switch m {
	case ModeLogoAA:
		return "Logo con bordes suavizados (Anti-Aliased)"
	case ModeLogoSharp:
		return "Logo con bordes nítidos (Sharp / Pixel Art)"
	case ModePhoto:
		return "Fotografía (Continuo / Curvas orgánicas)"
	default:
		return string(m)
	}
}

// Description returns a localized description of the quality preset.
func (q Quality) Description() string {
	switch q {
	case QualityHigh:
		return "Alta (Máxima fidelidad y micro-detalle)"
	case QualityLow:
		return "Baja (Bajo peso y supresión de ruido)"
	case QualityMedium:
		fallthrough
	default:
		return "Media (Recomendada / Balanceada)"
	}
}

// PresetConfig returns a Config pre-populated with the exact mathematical parameters
// corresponding to the given canonical mode and quality preset.
func PresetConfig(mode Mode, quality Quality) Config {
	m := NormalizeMode(string(mode))
	q := NormalizeQuality(string(quality))

	cfg := Config{
		Mode:                  m,
		Quality:               q,
		AntiSeamStroke:        true,
		TransparentBackground: false,
	}

	switch m {
	case ModeLogoAA:
		cfg.DetectCorners = true
		cfg.AntiAliasingReject = true
		switch q {
		case QualityHigh:
			cfg.Colors = 8
			cfg.MinPixels = 2
			cfg.Smoothness = 0.35
			cfg.CurveTolerance = 0.50
		case QualityLow:
			cfg.Colors = 6
			cfg.MinPixels = 8
			cfg.Smoothness = 0.75
			cfg.CurveTolerance = 1.25
		case QualityMedium:
			fallthrough
		default:
			cfg.Colors = 8
			cfg.MinPixels = 4
			cfg.Smoothness = 0.50
			cfg.CurveTolerance = 0.75
		}

	case ModeLogoSharp:
		cfg.DetectCorners = true
		cfg.AntiAliasingReject = false // Pixel art and sharp graphics must not reject edge pixels
		switch q {
		case QualityHigh:
			cfg.Colors = 8
			cfg.MinPixels = 1 // 1:1 single pixel preservation
			cfg.Smoothness = 0.10
			cfg.CurveTolerance = 0.25
		case QualityLow:
			cfg.Colors = 6
			cfg.MinPixels = 6
			cfg.Smoothness = 0.45
			cfg.CurveTolerance = 0.85
		case QualityMedium:
			fallthrough
		default:
			cfg.Colors = 8
			cfg.MinPixels = 2
			cfg.Smoothness = 0.25
			cfg.CurveTolerance = 0.50
		}

	case ModePhoto:
		cfg.DetectCorners = false      // Continuous tone photos have organic curves without sharp engineering corners
		cfg.AntiAliasingReject = false // Continuous tone gradients are real data, not raster anti-aliasing artifacts
		switch q {
		case QualityHigh:
			cfg.Colors = 16
			cfg.MinPixels = 3
			cfg.Smoothness = 0.30
			cfg.CurveTolerance = 0.40
		case QualityLow:
			cfg.Colors = 8
			cfg.MinPixels = 20
			cfg.Smoothness = 0.75
			cfg.CurveTolerance = 1.30
		case QualityMedium:
			fallthrough
		default:
			cfg.Colors = 12
			cfg.MinPixels = 8
			cfg.Smoothness = 0.50
			cfg.CurveTolerance = 0.75
		}
	}

	return cfg
}

// ApplyPreset updates the Config with preset defaults for any unconfigured or zero fields,
// ensuring the mode and quality rules (e.g. DetectCorners, AntiAliasingReject) are respected.
func (c *Config) ApplyPreset() {
	if c.Mode == "" {
		c.Mode = ModeLogoAA
	} else {
		c.Mode = NormalizeMode(string(c.Mode))
	}

	if c.Quality == "" {
		c.Quality = QualityMedium
	} else {
		c.Quality = NormalizeQuality(string(c.Quality))
	}

	preset := PresetConfig(c.Mode, c.Quality)

	if c.MinPixels <= 0 {
		c.MinPixels = preset.MinPixels
	}
	if c.Smoothness <= 0 {
		c.Smoothness = preset.Smoothness
	}
	if c.CurveTolerance <= 0 {
		c.CurveTolerance = preset.CurveTolerance
	}
	if c.Colors <= 0 {
		c.Colors = preset.Colors
	}

	// Mode-specific invariant rules:
	if c.Mode == ModePhoto {
		// Photos never use corner detection or anti-aliasing rejection
		c.DetectCorners = false
		c.AntiAliasingReject = false
	} else if c.Mode == ModeLogoSharp {
		// Sharp/pixel artwork must never discard edge pixels with anti-aliasing rejection
		c.AntiAliasingReject = false
	}
}
