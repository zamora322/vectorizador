package exporter

import (
	"github.com/zamora322/vectorizador/pkg/bezier"
	"github.com/zamora322/vectorizador/pkg/palette"
	"github.com/zamora322/vectorizador/pkg/segmentation"
	"github.com/zamora322/vectorizador/pkg/topology"
)

// Options defines common configuration parameters for vector and CAD exporters.
type Options struct {
	AntiSeamStroke        bool    // Apply 0.09375pt stroke to eliminate seams
	TransparentBackground bool    // Omit background/transparent color
	CurveTolerance        float64 // Error tolerance in pixels for Bézier fitting (default 0.75)
	DXFPolylineTolerance  float64 // Maximum chord error when subdividing Bézier curves for DXF (default 0.25)
	CompressPDF           bool    // Apply Flate/zlib compression to PDF streams
}

// DefaultOptions returns standard vector export settings.
func DefaultOptions() Options {
	return Options{
		AntiSeamStroke:        true,
		TransparentBackground: false,
		CurveTolerance:        0.75,
		DXFPolylineTolerance:  0.25,
		CompressPDF:           true,
	}
}

// PathSegment represents either a straight line or a cubic Bézier curve segment.
type PathSegment struct {
	IsLine bool
	P0     topology.Point
	P1     topology.Point // Control point 1 (Bézier only)
	P2     topology.Point // Control point 2 (Bézier only)
	P3     topology.Point // End point
}

// PathLoop represents a closed boundary path composed of connected segments.
type PathLoop struct {
	StartPoint topology.Point
	Segments   []PathSegment
}

// ColorLayer groups all path loops sharing a specific palette color.
type ColorLayer struct {
	ColorIndex int
	Color      palette.RGB
	Loops      []PathLoop
}

// VectorDocument represents the unified vector geometry ready for multi-format export.
type VectorDocument struct {
	Width  int
	Height int
	Layers []ColorLayer
}

// BuildDocument converts a planar graph and segment map into a unified VectorDocument.
func BuildDocument(graph *topology.PlanarGraph, sm *segmentation.SegmentMap, opts Options) *VectorDocument {
	w, h := graph.Width, graph.Height
	if opts.CurveTolerance <= 0 {
		opts.CurveTolerance = 0.75
	}

	// 1. Fit Bézier curves for all unique edges in the graph
	edgeCurves := make(map[int][]bezier.CubicBezier)
	for _, e := range graph.Edges {
		curves := bezier.FitCurve(e.Points, opts.CurveTolerance)
		edgeCurves[e.ID] = curves
	}

	// 2. Group loops by palette color index
	colorLoops := make(map[int][]PathLoop)

	for regID, loops := range graph.RegionLoops {
		seg, ok := sm.Segments[regID]
		if !ok || seg.Transparent {
			continue
		}

		for _, loop := range loops {
			if len(loop.Edges) == 0 {
				continue
			}

			var pathSegments []PathSegment
			startPt := loop.Edges[0].Points[0]

			for _, edge := range loop.Edges {
				var curves []bezier.CubicBezier
				if edge.ID > 0 {
					curves = edgeCurves[edge.ID]
				} else {
					origCurves := edgeCurves[-edge.ID]
					curves = reverseCurves(origCurves)
				}

				for _, c := range curves {
					if c.IsCollinear(0.2) {
						pathSegments = append(pathSegments, PathSegment{
							IsLine: true,
							P0:     c.P0,
							P3:     c.P3,
						})
					} else {
						pathSegments = append(pathSegments, PathSegment{
							IsLine: false,
							P0:     c.P0,
							P1:     c.P1,
							P2:     c.P2,
							P3:     c.P3,
						})
					}
				}
			}

			if len(pathSegments) > 0 {
				colorLoops[seg.ColorIndex] = append(colorLoops[seg.ColorIndex], PathLoop{
					StartPoint: startPt,
					Segments:   pathSegments,
				})
			}
		}
	}

	// 3. Assemble ColorLayers in palette order
	var layers []ColorLayer
	for colorIdx := 0; colorIdx < len(sm.Palette); colorIdx++ {
		loops, ok := colorLoops[colorIdx]
		if !ok || len(loops) == 0 {
			continue
		}
		if opts.TransparentBackground && colorIdx == len(sm.Palette)-1 {
			continue
		}

		layers = append(layers, ColorLayer{
			ColorIndex: colorIdx,
			Color:      sm.Palette[colorIdx],
			Loops:      loops,
		})
	}

	return &VectorDocument{
		Width:  w,
		Height: h,
		Layers: layers,
	}
}

func reverseCurves(curves []bezier.CubicBezier) []bezier.CubicBezier {
	n := len(curves)
	rev := make([]bezier.CubicBezier, n)
	for i, c := range curves {
		rev[n-1-i] = bezier.CubicBezier{
			P0: c.P3,
			P1: c.P2,
			P2: c.P1,
			P3: c.P0,
		}
	}
	return rev
}

