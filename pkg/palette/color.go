package palette

import (
	"fmt"
	"math"
)

// RGB represents an 8-bit per channel RGB color.
type RGB struct {
	R, G, B uint8
}

// Hex returns the hexadecimal color string "#RRGGBB".
func (c RGB) Hex() string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// Lab represents a color in the CIE L*a*b* color space.
type Lab struct {
	L, A, B float64
}

// RGBToLab converts an sRGB color to CIE L*a*b* (D65 standard illuminant).
func RGBToLab(c RGB) Lab {
	// 1. sRGB to linear RGB
	var r, g, b float64
	r = pivotRGB(float64(c.R) / 255.0)
	g = pivotRGB(float64(c.G) / 255.0)
	b = pivotRGB(float64(c.B) / 255.0)

	// 2. Linear RGB to XYZ (D65 matrix)
	x := (r*0.4124564 + g*0.3575761 + b*0.1804375) / 0.95047
	y := (r*0.2126729 + g*0.7151522 + b*0.0721750) / 1.00000
	z := (r*0.0193339 + g*0.1191920 + b*0.9503041) / 1.08883

	// 3. XYZ to CIE L*a*b*
	fx := pivotXYZ(x)
	fy := pivotXYZ(y)
	fz := pivotXYZ(z)

	return Lab{
		L: math.Max(0, 116.0*fy-16.0),
		A: 500.0 * (fx - fy),
		B: 200.0 * (fy - fz),
	}
}

// LabToRGB converts a CIE L*a*b* color back to sRGB.
func LabToRGB(lab Lab) RGB {
	fy := (lab.L + 16.0) / 116.0
	fx := lab.A/500.0 + fy
	fz := fy - lab.B/200.0

	x := unpivotXYZ(fx) * 0.95047
	y := unpivotXYZ(fy) * 1.00000
	z := unpivotXYZ(fz) * 1.08883

	r := x*3.2404542 - y*1.5371385 - z*0.4985314
	g := -x*0.9692660 + y*1.8760108 + z*0.0415560
	b := x*0.0556434 - y*0.2040259 + z*1.0572252

	return RGB{
		R: clampUint8(unpivotRGB(r) * 255.0),
		G: clampUint8(unpivotRGB(g) * 255.0),
		B: clampUint8(unpivotRGB(b) * 255.0),
	}
}

// DeltaE computes the perceptual color distance (CIE76 Euclidean in Lab).
func DeltaE(c1, c2 Lab) float64 {
	dl := c1.L - c2.L
	da := c1.A - c2.A
	db := c1.B - c2.B
	return math.Sqrt(dl*dl + da*da + db*db)
}

// DeltaERGB calculates perceptual distance directly between two RGB colors.
func DeltaERGB(c1, c2 RGB) float64 {
	return DeltaE(RGBToLab(c1), RGBToLab(c2))
}

func pivotRGB(n float64) float64 {
	if n > 0.04045 {
		return math.Pow((n+0.055)/1.055, 2.4)
	}
	return n / 12.92
}

func unpivotRGB(n float64) float64 {
	if n > 0.0031308 {
		return 1.055*math.Pow(n, 1.0/2.4) - 0.055
	}
	return 12.92 * n
}

func pivotXYZ(n float64) float64 {
	if n > 0.008856 {
		return math.Cbrt(n)
	}
	return (7.787 * n) + (16.0 / 116.0)
}

func unpivotXYZ(n float64) float64 {
	if n*n*n > 0.008856 {
		return n * n * n
	}
	return (n - 16.0/116.0) / 7.787
}

func clampUint8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(math.Round(v))
}
