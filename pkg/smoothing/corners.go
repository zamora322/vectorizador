package smoothing

import (
	"math"

	"github.com/zamora322/vectorizador/pkg/topology"
)

// CornerDetector identifies sharp corner vertices along polylines.
type CornerDetector struct {
	AngleThreshold float64 // Minimum angle deviation to be classified as a corner (in radians)
	Lookahead      int     // Distance to sample before and after current point
}

// NewCornerDetector returns a corner detector with sensible defaults.
func NewCornerDetector() *CornerDetector {
	return &CornerDetector{
		AngleThreshold: 60.0 * (math.Pi / 180.0), // 60 degrees turn
		Lookahead:      2,
	}
}

// DetectCorners returns a boolean mask marking which points in pts are sharp corners.
func (cd *CornerDetector) DetectCorners(pts []topology.Point, isClosed bool) []bool {
	n := len(pts)
	corners := make([]bool, n)
	if n < 3 {
		return corners
	}

	// First and last points of an open edge are always corners (junctions)
	if !isClosed {
		corners[0] = true
		corners[n-1] = true
	}

	k := cd.Lookahead
	for i := 0; i < n; i++ {
		if !isClosed && (i < k || i >= n-k) {
			continue
		}

		prevIdx := (i - k + n) % n
		nextIdx := (i + k) % n

		pPrev := pts[prevIdx]
		pCurr := pts[i]
		pNext := pts[nextIdx]

		vInX, vInY := pCurr.X-pPrev.X, pCurr.Y-pPrev.Y
		vOutX, vOutY := pNext.X-pCurr.X, pNext.Y-pCurr.Y

		lenIn := math.Sqrt(vInX*vInX + vInY*vInY)
		lenOut := math.Sqrt(vOutX*vOutX + vOutY*vOutY)

		if lenIn < 1e-4 || lenOut < 1e-4 {
			continue
		}

		dot := (vInX*vOutX + vInY*vOutY) / (lenIn * lenOut)
		dot = math.Max(-1.0, math.Min(1.0, dot))

		// Angle of deflection: arccos(dot)
		turnAngle := math.Acos(dot)
		if turnAngle >= cd.AngleThreshold {
			corners[i] = true
		}
	}

	return corners
}
