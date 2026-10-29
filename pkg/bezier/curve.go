package bezier

import (
	"fmt"
	"math"

	"github.com/zamora322/vectorizador/pkg/topology"
)

// CubicBezier represents a cubic Bézier curve with control points P0, P1, P2, P3.
type CubicBezier struct {
	P0, P1, P2, P3 topology.Point
}

// PointAt evaluates the curve at parameter t in [0, 1].
func (b CubicBezier) PointAt(t float64) topology.Point {
	mt := 1.0 - t
	mt2 := mt * mt
	mt3 := mt2 * mt
	t2 := t * t
	t3 := t2 * t

	return topology.Point{
		X: mt3*b.P0.X + 3*mt2*t*b.P1.X + 3*mt*t2*b.P2.X + t3*b.P3.X,
		Y: mt3*b.P0.Y + 3*mt2*t*b.P1.Y + 3*mt*t2*b.P2.Y + t3*b.P3.Y,
	}
}

// SVGCommand formats the curve as an SVG cubic Bézier command "C x1,y1 x2,y2 x3,y3".
func (b CubicBezier) SVGCommand() string {
	return fmt.Sprintf("C %.2f,%.2f %.2f,%.2f %.2f,%.2f",
		b.P1.X, b.P1.Y,
		b.P2.X, b.P2.Y,
		b.P3.X, b.P3.Y)
}

// IsCollinear checks if the control points form an approximately straight line.
func (b CubicBezier) IsCollinear(tolerance float64) bool {
	// Distance of P1 and P2 from line P0-P3
	d1 := pointLineDistance(b.P1, b.P0, b.P3)
	d2 := pointLineDistance(b.P2, b.P0, b.P3)
	return d1 < tolerance && d2 < tolerance
}

func pointLineDistance(p, l0, l1 topology.Point) float64 {
	dx := l1.X - l0.X
	dy := l1.Y - l0.Y
	lenSq := dx*dx + dy*dy
	if lenSq < 1e-6 {
		return p.Distance(l0)
	}
	num := math.Abs(dy*p.X - dx*p.Y + l1.X*l0.Y - l1.Y*l0.X)
	return num / math.Sqrt(lenSq)
}
