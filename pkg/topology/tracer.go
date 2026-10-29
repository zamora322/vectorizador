package topology

import (
	"github.com/zamora322/vectorizador/pkg/segmentation"
)

type dualEdge struct {
	x1, y1 int
	x2, y2 int
	left   int
	right  int
}

// TraceTopology extracts the planar graph of shared boundaries between segments.
func TraceTopology(sm *segmentation.SegmentMap) *PlanarGraph {
	w, h := sm.Width, sm.Height

	// Map of directed unit boundary segments: "x,y -> dir"
	// Dual coordinates: x in [0, w], y in [0, h]
	type nodeKey struct {
		x, y int
	}

	type edgeSegment struct {
		from, to    nodeKey
		left, right int
		visited     bool
	}

	// Adjacency from each dual node
	adj := make(map[nodeKey][]*edgeSegment)
	var allSegments []*edgeSegment

	addSegment := func(x1, y1, x2, y2, left, right int) {
		k1 := nodeKey{x1, y1}
		k2 := nodeKey{x2, y2}
		es := &edgeSegment{
			from:  k1,
			to:    k2,
			left:  left,
			right: right,
		}
		adj[k1] = append(adj[k1], es)
		allSegments = append(allSegments, es)
	}

	// 1. Scan horizontal dual edges (between (x, y) and (x+1, y))
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
				// Direction from left to right: top is LeftRegion, bottom is RightRegion
				addSegment(x, y, x+1, y, top, bottom)
			}
		}
	}

	// 2. Scan vertical dual edges (between (x, y) and (x, y+1))
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
				// Direction from top to bottom: right is LeftRegion, left is RightRegion
				// Or from (x, y) to (x, y+1): left is LeftRegion, right is RightRegion if directed downwards?
				// Let's standardise: walking downwards (y to y+1):
				// Left of vector (0, 1) is (+1, 0) in standard math, but in screen coordinates (x right, y down):
				// Forward = (0, 1). Right = (-1, 0) ?
				// In screen space (x right, y down):
				// Walking (x, y) -> (x, y+1):
				//   to the right (+x) is pixel (x, y).
				//   to the left (-x) is pixel (x-1, y).
				// So: left is LeftRegion (x-1, y), right is RightRegion (x, y).
				addSegment(x, y, x, y+1, right, left)
			}
		}
	}

	// 3. Identify junction nodes (nodes with degree != 2)
	degree := make(map[nodeKey]int)
	for _, es := range allSegments {
		degree[es.from]++
		degree[es.to]++
	}

	isJunction := func(nk nodeKey) bool {
		return degree[nk] != 2
	}

	// 4. Trace chains into Edges
	var edges []*Edge
	edgeID := 1

	// Trace starting from junctions first
	for _, es := range allSegments {
		if es.visited {
			continue
		}
		if !isJunction(es.from) {
			continue
		}

		// Start a new edge from this junction
		es.visited = true
		currPoints := []Point{
			{X: float64(es.from.x), Y: float64(es.from.y)},
			{X: float64(es.to.x), Y: float64(es.to.y)},
		}

		leftReg := es.left
		rightReg := es.right
		currNode := es.to

		for !isJunction(currNode) {
			// Find the next unvisited connected segment with matching regions
			var nextSeg *edgeSegment
			for _, candidate := range adj[currNode] {
				if !candidate.visited && candidate.left == leftReg && candidate.right == rightReg {
					nextSeg = candidate
					break
				}
			}

			if nextSeg == nil {
				break
			}

			nextSeg.visited = true
			currPoints = append(currPoints, Point{X: float64(nextSeg.to.x), Y: float64(nextSeg.to.y)})
			currNode = nextSeg.to
		}

		edges = append(edges, &Edge{
			ID:          edgeID,
			Points:      currPoints,
			LeftRegion:  leftReg,
			RightRegion: rightReg,
			IsClosed:    false,
		})
		edgeID++
	}

	// Trace remaining closed isolated loops (islands with only degree 2 vertices)
	for _, es := range allSegments {
		if es.visited {
			continue
		}

		es.visited = true
		currPoints := []Point{
			{X: float64(es.from.x), Y: float64(es.from.y)},
			{X: float64(es.to.x), Y: float64(es.to.y)},
		}

		leftReg := es.left
		rightReg := es.right
		startNode := es.from
		currNode := es.to

		for currNode != startNode {
			var nextSeg *edgeSegment
			for _, candidate := range adj[currNode] {
				if !candidate.visited && candidate.left == leftReg && candidate.right == rightReg {
					nextSeg = candidate
					break
				}
			}

			if nextSeg == nil {
				break
			}

			nextSeg.visited = true
			currPoints = append(currPoints, Point{X: float64(nextSeg.to.x), Y: float64(nextSeg.to.y)})
			currNode = nextSeg.to
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

	// 5. Build region loops
	// Group edges bounding each region
	graph := &PlanarGraph{
		Width:       w,
		Height:      h,
		Edges:       edges,
		RegionLoops: make(map[int][]*RegionLoop),
	}
	graph.buildRegionLoops()

	return graph
}

func (pg *PlanarGraph) buildRegionLoops() {
	// For each region, collect all directed edges that bound it with the region on the inside (Left)
	regionEdges := make(map[int][]*Edge)
	for _, e := range pg.Edges {
		if e.LeftRegion >= 0 {
			regionEdges[e.LeftRegion] = append(regionEdges[e.LeftRegion], e)
		}
		if e.RightRegion >= 0 {
			regionEdges[e.RightRegion] = append(regionEdges[e.RightRegion], e.Reverse())
		}
	}

	for regID, edgeList := range regionEdges {
		loops := assembleLoops(regID, edgeList)
		pg.RegionLoops[regID] = loops
	}
}

func assembleLoops(regID int, edgeList []*Edge) []*RegionLoop {
	if len(edgeList) == 0 {
		return nil
	}

	used := make([]bool, len(edgeList))
	var loops []*RegionLoop

	for i, startEdge := range edgeList {
		if used[i] {
			continue
		}

		used[i] = true
		currentLoop := []*Edge{startEdge}
		if startEdge.IsClosed {
			loops = append(loops, &RegionLoop{
				RegionID: regID,
				Edges:    currentLoop,
				IsHole:   false,
			})
			continue
		}

		currEnd := startEdge.Points[len(startEdge.Points)-1]
		startPt := startEdge.Points[0]

		for currEnd != startPt {
			var nextIdx = -1
			for j, cand := range edgeList {
				if !used[j] && cand.Points[0] == currEnd {
					nextIdx = j
					break
				}
			}

			if nextIdx == -1 {
				break
			}

			used[nextIdx] = true
			currentLoop = append(currentLoop, edgeList[nextIdx])
			currEnd = edgeList[nextIdx].Points[len(edgeList[nextIdx].Points)-1]
		}

		loops = append(loops, &RegionLoop{
			RegionID: regID,
			Edges:    currentLoop,
			IsHole:   false,
		})
	}

	return loops
}
