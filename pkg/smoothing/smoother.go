package smoothing

import (
	"github.com/zamora322/vectorizador/pkg/topology"
)

// SmoothGraph applies corner preservation and Laplacian path smoothing to all edges in the graph.
func SmoothGraph(graph *topology.PlanarGraph, smoothness float64, detectCorners bool) {
	if smoothness <= 0 {
		return
	}
	if smoothness > 1.0 {
		smoothness = 1.0
	}

	cd := NewCornerDetector()

	for _, edge := range graph.Edges {
		pts := edge.Points
		n := len(pts)
		if n < 3 {
			continue
		}

		var corners []bool
		if detectCorners {
			corners = cd.DetectCorners(pts, edge.IsClosed)
		} else {
			corners = make([]bool, n)
			if !edge.IsClosed {
				corners[0] = true
				corners[n-1] = true
			}
		}

		// Perform 3 iterations of Laplacian smoothing
		iterations := 3
		smoothed := make([]topology.Point, n)
		copy(smoothed, pts)

		for it := 0; it < iterations; it++ {
			temp := make([]topology.Point, n)
			copy(temp, smoothed)

			for i := 0; i < n; i++ {
				if corners[i] {
					continue // Pinned corner point
				}

				var prev, next topology.Point
				if edge.IsClosed {
					prev = temp[(i-1+n)%n]
					next = temp[(i+1)%n]
				} else {
					if i == 0 || i == n-1 {
						continue
					}
					prev = temp[i-1]
					next = temp[i+1]
				}

				// Laplacian update: (1 - lambda)*P_i + (lambda/2)*(P_{i-1} + P_{i+1})
				smoothed[i].X = (1.0-smoothness)*temp[i].X + (smoothness/2.0)*(prev.X+next.X)
				smoothed[i].Y = (1.0-smoothness)*temp[i].Y + (smoothness/2.0)*(prev.Y+next.Y)
			}
		}

		edge.Points = smoothed
	}
}
