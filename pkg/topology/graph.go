package topology

import (
	"fmt"
	"math"
)

// Point represents a 2D coordinate on the dual grid or in vector space.
type Point struct {
	X, Y float64
}

func (p Point) String() string {
	return fmt.Sprintf("(%.2f, %.2f)", p.X, p.Y)
}

// Distance returns Euclidean distance between p and other.
func (p Point) Distance(other Point) float64 {
	dx := p.X - other.X
	dy := p.Y - other.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// Edge represents a directed boundary between LeftRegion and RightRegion.
type Edge struct {
	ID          int
	Points      []Point // Sub-pixel or pixel polyline coordinates
	LeftRegion  int     // Segment ID on the left side (walking from start to end)
	RightRegion int     // Segment ID on the right side
	IsClosed    bool    // True if Start == End and no junction was encountered (isolated island)
}

// Reverse returns a new Edge with points reversed and regions swapped.
func (e *Edge) Reverse() *Edge {
	n := len(e.Points)
	revPoints := make([]Point, n)
	for i, pt := range e.Points {
		revPoints[n-1-i] = pt
	}
	return &Edge{
		ID:          -e.ID,
		Points:      revPoints,
		LeftRegion:  e.RightRegion,
		RightRegion: e.LeftRegion,
		IsClosed:    e.IsClosed,
	}
}

// RegionLoop represents a directed closed loop of edges bounding a single region.
type RegionLoop struct {
	RegionID int
	Edges    []*Edge
	IsHole   bool
}

// PlanarGraph contains all topological edges and region boundaries.
type PlanarGraph struct {
	Width       int
	Height      int
	Edges       []*Edge
	RegionLoops map[int][]*RegionLoop
}
