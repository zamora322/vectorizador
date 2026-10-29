package bezier

import (
	"math"
	"testing"

	"github.com/zamora322/vectorizador/pkg/topology"
)

func TestFitStraightLine(t *testing.T) {
	// Simple straight line: (0,0) to (10, 0) with multiple sample points
	pts := []topology.Point{
		{X: 0, Y: 0},
		{X: 2, Y: 0},
		{X: 5, Y: 0},
		{X: 8, Y: 0},
		{X: 10, Y: 0},
	}

	curves := FitCurve(pts, 1.0)
	if len(curves) == 0 {
		t.Fatalf("Expected at least one fitted curve")
	}

	// Midpoint on fitted curve should have Y approx 0
	mid := curves[0].PointAt(0.5)
	if math.Abs(mid.Y) > 0.1 {
		t.Errorf("Expected Y approx 0, got %f", mid.Y)
	}
}

func TestFitCircularArc(t *testing.T) {
	// Sample a quarter circle of radius 10: x = 10*cos(theta), y = 10*sin(theta) for theta in [0, pi/2]
	pts := make([]topology.Point, 15)
	for i := 0; i < 15; i++ {
		theta := (float64(i) / 14.0) * (math.Pi / 2.0)
		pts[i] = topology.Point{
			X: 10.0 * math.Cos(theta),
			Y: 10.0 * math.Sin(theta),
		}
	}

	// Fit with tolerance 0.5
	curves := FitCurve(pts, 0.5)
	if len(curves) == 0 {
		t.Fatalf("Expected fitted curves for quarter circle")
	}

	// Check sample error along the curves
	for _, c := range curves {
		for u := 0.0; u <= 1.0; u += 0.2 {
			pt := c.PointAt(u)
			r := math.Sqrt(pt.X*pt.X + pt.Y*pt.Y)
			if math.Abs(r-10.0) > 0.5 {
				t.Errorf("Radius error on arc: expected ~10.0, got %f", r)
			}
		}
	}
}
