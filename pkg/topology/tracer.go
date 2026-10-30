package topology

import (
	"github.com/zamora322/vectorizador/pkg/segmentation"
)

type nodeKey struct {
	x, y int
}

type rawEdge struct {
	id              int
	u, v            nodeKey
	leftUV, rightUV int
	visited         bool
}

type incidentEdge struct {
	edge     *rawEdge
	target   nodeKey
	leftReg  int
	rightReg int
}

// TraceTopology extracts the planar graph of shared boundaries between segments.
func TraceTopology(sm *segmentation.SegmentMap) *PlanarGraph {
	w, h := sm.Width, sm.Height
	adj := make(map[nodeKey][]incidentEdge)
	edgeIDCounter := 0

	addUndirected := func(x1, y1, x2, y2, leftUV, rightUV int) {
		u := nodeKey{x1, y1}
		v := nodeKey{x2, y2}
		re := &rawEdge{
			id:      edgeIDCounter,
			u:       u,
			v:       v,
			leftUV:  leftUV,
			rightUV: rightUV,
		}
		edgeIDCounter++
		adj[u] = append(adj[u], incidentEdge{
			edge:     re,
			target:   v,
			leftReg:  leftUV,
			rightReg: rightUV,
		})
		adj[v] = append(adj[v], incidentEdge{
			edge:     re,
			target:   u,
			leftReg:  rightUV,
			rightReg: leftUV,
		})
	}

	// 1. Horizontal dual edges (between (x, y) and (x+1, y))
	for y := 0; y <= h; y++ {
		for x := 0; x < w; x++ {
			var top, bottom int
			if y == 0 {
				top = -1
			} else {
				top = sm.At(x, y-1)
			}
			if y == h {
				bottom = -1
			} else {
				bottom = sm.At(x, y)
			}
			if top != bottom {
				// Moving (x, y) -> (x+1, y) [+X]: left is top, right is bottom
				addUndirected(x, y, x+1, y, top, bottom)
			}
		}
	}

	// 2. Vertical dual edges (between (x, y) and (x, y+1))
	for y := 0; y < h; y++ {
		for x := 0; x <= w; x++ {
			var left, right int
			if x == 0 {
				left = -1
			} else {
				left = sm.At(x-1, y)
			}
			if x == w {
				right = -1
			} else {
				right = sm.At(x, y)
			}
			if left != right {
				// Moving (x, y) -> (x, y+1) [+Y]: left is right, right is left
				addUndirected(x, y, x, y+1, right, left)
			}
		}
	}

	degree := make(map[nodeKey]int, len(adj))
	for k, list := range adj {
		degree[k] = len(list)
	}

	var edges []*Edge
	edgeID := 1

	// Step 1: Trace starting from all junctions (degree != 2)
	for k, list := range adj {
		if degree[k] == 2 {
			continue
		}
		for _, inc := range list {
			if inc.edge.visited {
				continue
			}
			inc.edge.visited = true
			currPoints := []Point{
				{X: float64(k.x), Y: float64(k.y)},
				{X: float64(inc.target.x), Y: float64(inc.target.y)},
			}
			leftReg := inc.leftReg
			rightReg := inc.rightReg
			currNode := inc.target

			for degree[currNode] == 2 {
				var nextInc *incidentEdge
				for i := range adj[currNode] {
					if !adj[currNode][i].edge.visited {
						nextInc = &adj[currNode][i]
						break
					}
				}
				if nextInc == nil {
					break
				}
				nextInc.edge.visited = true
				currPoints = append(currPoints, Point{
					X: float64(nextInc.target.x),
					Y: float64(nextInc.target.y),
				})
				currNode = nextInc.target
			}

			edges = append(edges, &Edge{
				ID:          edgeID,
				Points:      currPoints,
				LeftRegion:  leftReg,
				RightRegion: rightReg,
				IsClosed:    (k == currNode),
			})
			edgeID++
		}
	}

	// Step 2: Trace remaining closed loops (islands with all degree == 2)
	for k, list := range adj {
		for _, inc := range list {
			if inc.edge.visited {
				continue
			}
			inc.edge.visited = true
			startNode := k
			currPoints := []Point{
				{X: float64(k.x), Y: float64(k.y)},
				{X: float64(inc.target.x), Y: float64(inc.target.y)},
			}
			leftReg := inc.leftReg
			rightReg := inc.rightReg
			currNode := inc.target

			for currNode != startNode {
				var nextInc *incidentEdge
				for i := range adj[currNode] {
					if !adj[currNode][i].edge.visited {
						nextInc = &adj[currNode][i]
						break
					}
				}
				if nextInc == nil {
					break
				}
				nextInc.edge.visited = true
				currPoints = append(currPoints, Point{
					X: float64(nextInc.target.x),
					Y: float64(nextInc.target.y),
				})
				currNode = nextInc.target
			}

			edges = append(edges, &Edge{
				ID:          edgeID,
				Points:      currPoints,
				LeftRegion:  leftReg,
				RightRegion: rightReg,
				IsClosed:    true,
			})
			edgeID++
		}
	}

	// 5. Build region loops
	graph := &PlanarGraph{
		Width:       w,
		Height:      h,
		Edges:       edges,
		RegionLoops: make(map[int][]*RegionLoop),
	}
	graph.buildRegionLoops()

	return graph
}
