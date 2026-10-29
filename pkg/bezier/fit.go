package bezier

import (
	"math"

	"github.com/zamora322/vectorizador/pkg/topology"
)

// FitCurve fits a sequence of cubic Bézier curves to a digitized polyline within maxError.
func FitCurve(points []topology.Point, maxError float64) []CubicBezier {
	n := len(points)
	if n < 2 {
		return nil
	}
	if n == 2 {
		// Single straight line
		p0, p3 := points[0], points[1]
		return []CubicBezier{{
			P0: p0,
			P1: topology.Point{X: p0.X + (p3.X-p0.X)/3.0, Y: p0.Y + (p3.Y-p0.Y)/3.0},
			P2: topology.Point{X: p0.X + 2.0*(p3.X-p0.X)/3.0, Y: p0.Y + 2.0*(p3.Y-p0.Y)/3.0},
			P3: p3,
		}}
	}

	tHat1 := computeLeftTangent(points, 0)
	tHat2 := computeRightTangent(points, n-1)

	var curves []CubicBezier
	fitCubic(points, 0, n-1, tHat1, tHat2, maxError, &curves)
	return curves
}

func fitCubic(pts []topology.Point, first, last int, tHat1, tHat2 topology.Point, maxError float64, out *[]CubicBezier) {
	nPts := last - first + 1
	if nPts == 2 {
		p0, p3 := pts[first], pts[last]
		dist := p0.Distance(p3) / 3.0
		*out = append(*out, CubicBezier{
			P0: p0,
			P1: topology.Point{X: p0.X + tHat1.X*dist, Y: p0.Y + tHat1.Y*dist},
			P2: topology.Point{X: p3.X + tHat2.X*dist, Y: p3.Y + tHat2.Y*dist},
			P3: p3,
		})
		return
	}

	u := chordLengthParameterize(pts, first, last)
	bezCurve := generateBezier(pts, first, last, u, tHat1, tHat2)

	maxDist, splitPoint := computeMaxError(pts, first, last, bezCurve, u)
	if maxDist < maxError {
		*out = append(*out, bezCurve)
		return
	}

	// If error too large, split at maximum error point and recurse
	if maxDist >= maxError {
		tHatCenter := computeCenterTangent(pts, splitPoint)
		tHatCenterNeg := topology.Point{X: -tHatCenter.X, Y: -tHatCenter.Y}

		fitCubic(pts, first, splitPoint, tHat1, tHatCenter, maxError, out)
		fitCubic(pts, splitPoint, last, tHatCenterNeg, tHat2, maxError, out)
	}
}

func generateBezier(pts []topology.Point, first, last int, u []float64, tHat1, tHat2 topology.Point) CubicBezier {
	p0 := pts[first]
	p3 := pts[last]
	nPts := last - first + 1

	// Compute A matrix components
	var c00, c01, c10, c11 float64
	var x0, x1 float64

	for i := 0; i < nPts; i++ {
		uVal := u[i]
		mt := 1.0 - uVal

		// B0(u) = (1-u)^3, B1(u) = 3(1-u)^2*u, B2(u) = 3(1-u)*u^2, B3(u) = u^3
		b0 := mt * mt * mt
		b1 := 3.0 * mt * mt * uVal
		b2 := 3.0 * mt * uVal * uVal
		b3 := uVal * uVal * uVal

		a1 := topology.Point{X: tHat1.X * b1, Y: tHat1.Y * b1}
		a2 := topology.Point{X: tHat2.X * b2, Y: tHat2.Y * b2}

		c00 += a1.X*a1.X + a1.Y*a1.Y
		c01 += a1.X*a2.X + a1.Y*a2.Y
		c10 = c01
		c11 += a2.X*a2.X + a2.Y*a2.Y

		pt := pts[first+i]
		tmpX := pt.X - (b0*p0.X + b1*p0.X + b2*p3.X + b3*p3.X)
		tmpY := pt.Y - (b0*p0.Y + b1*p0.Y + b2*p3.Y + b3*p3.Y)

		x0 += a1.X*tmpX + a1.Y*tmpY
		x1 += a2.X*tmpX + a2.Y*tmpY
	}

	det := c00*c11 - c10*c01
	var alphaL, alphaR float64
	if math.Abs(det) > 1e-9 {
		alphaL = (x0*c11 - x1*c01) / det
		alphaR = (c00*x1 - c10*x0) / det
	}

	segLen := p0.Distance(p3)
	epsilon := 1e-6 * segLen
	if alphaL < epsilon || alphaR < epsilon {
		alphaL = segLen / 3.0
		alphaR = segLen / 3.0
	}

	return CubicBezier{
		P0: p0,
		P1: topology.Point{X: p0.X + tHat1.X*alphaL, Y: p0.Y + tHat1.Y*alphaL},
		P2: topology.Point{X: p3.X + tHat2.X*alphaR, Y: p3.Y + tHat2.Y*alphaR},
		P3: p3,
	}
}

func chordLengthParameterize(pts []topology.Point, first, last int) []float64 {
	nPts := last - first + 1
	u := make([]float64, nPts)
	u[0] = 0.0

	for i := 1; i < nPts; i++ {
		u[i] = u[i-1] + pts[first+i].Distance(pts[first+i-1])
	}

	total := u[nPts-1]
	if total > 1e-9 {
		for i := 1; i < nPts; i++ {
			u[i] /= total
		}
	}
	return u
}

func computeMaxError(pts []topology.Point, first, last int, bez CubicBezier, u []float64) (float64, int) {
	nPts := last - first + 1
	splitPoint := (last - first) / 2
	maxDist := 0.0

	for i := 1; i < nPts-1; i++ {
		pOnCurve := bez.PointAt(u[i])
		pActual := pts[first+i]
		dist := pOnCurve.Distance(pActual)
		if dist > maxDist {
			maxDist = dist
			splitPoint = first + i
		}
	}

	return maxDist, splitPoint
}

func computeLeftTangent(pts []topology.Point, end int) topology.Point {
	d := topology.Point{X: pts[end+1].X - pts[end].X, Y: pts[end+1].Y - pts[end].Y}
	return normalize(d)
}

func computeRightTangent(pts []topology.Point, end int) topology.Point {
	d := topology.Point{X: pts[end-1].X - pts[end].X, Y: pts[end-1].Y - pts[end].Y}
	return normalize(d)
}

func computeCenterTangent(pts []topology.Point, center int) topology.Point {
	v1 := topology.Point{X: pts[center-1].X - pts[center].X, Y: pts[center-1].Y - pts[center].Y}
	v2 := topology.Point{X: pts[center].X - pts[center+1].X, Y: pts[center].Y - pts[center+1].Y}
	return normalize(topology.Point{X: (v1.X + v2.X) / 2.0, Y: (v1.Y + v2.Y) / 2.0})
}

func normalize(p topology.Point) topology.Point {
	len := math.Sqrt(p.X*p.X + p.Y*p.Y)
	if len > 1e-9 {
		return topology.Point{X: p.X / len, Y: p.Y / len}
	}
	return topology.Point{X: 1, Y: 0}
}
