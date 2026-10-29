package smoothing

import (
	"math"
	"testing"

	"github.com/zamora322/vectorizador/pkg/topology"
)

func TestCornerDetection(t *testing.T) {
	// A polyline forming a 90-degree right angle: (0,0) -> (5,0) -> (5,5)
	pts := []topology.Point{
		{X: 0, Y: 0},
		{X: 2, Y: 0},
		{X: 5, Y: 0}, // Corner at index 2
		{X: 5, Y: 2},
		{X: 5, Y: 5},
	}

	cd := &CornerDetector{
		AngleThreshold: 45.0 * (math.Pi / 180.0),
		Lookahead:      1,
	}

	corners := cd.DetectCorners(pts, false)
	if !corners[2] {
		t.Errorf("Expected index 2 to be detected as a sharp corner")
	}
}

func TestSmoothGraph(t *testing.T) {
	edge := &topology.Edge{
		ID: 1,
		Points: []topology.Point{
			{X: 0, Y: 0},
			{X: 1, Y: 1}, // Jagged step
			{X: 2, Y: 0},
			{X: 3, Y: 1},
			{X: 4, Y: 0},
		},
		LeftRegion:  1,
		RightRegion: 0,
		IsClosed:    false,
	}

	graph := &topology.PlanarGraph{
		Width: 5, Height: 5,
		Edges: []*topology.Edge{edge},
	}

	SmoothGraph(graph, 0.5, false)

	// Interior jagged points should be softened towards Y=0.5
	if edge.Points[1].Y == 1.0 {
		t.Errorf("Expected interior jagged point to be smoothed")
	}
	// Endpoints must remain pinned
	if edge.Points[0].X != 0 || edge.Points[0].Y != 0 {
		t.Errorf("Start endpoint was moved!")
	}
	if edge.Points[4].X != 4 || edge.Points[4].Y != 0 {
		t.Errorf("End endpoint was moved!")
	}
}
