package svg

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/zamora322/vectorizador/pkg/bezier"
	"github.com/zamora322/vectorizador/pkg/segmentation"
	"github.com/zamora322/vectorizador/pkg/topology"
)

// SVGOptions defines parameters for SVG generation.
type SVGOptions struct {
	AntiSeamStroke        bool    // 0.09375pt stroke to eliminate rendering seams
	TransparentBackground bool    // Skip background color region if desired
	GroupLayersByColor    bool    // Organize output paths into color-based <g> groups
	CurveTolerance        float64 // Error tolerance in pixels for Bézier curve fitting
}

// DefaultSVGOptions returns standard production settings.
func DefaultSVGOptions() SVGOptions {
	return SVGOptions{
		AntiSeamStroke:        true,
		TransparentBackground: false,
		GroupLayersByColor:    true,
		CurveTolerance:        0.75,
	}
}

// GenerateSVG converts a segmented image and its smoothed topological graph into an SVG string.
func GenerateSVG(graph *topology.PlanarGraph, sm *segmentation.SegmentMap, opts SVGOptions) string {
	w, h := graph.Width, graph.Height

	// 1. Fit Bézier curves for all unique edges in the graph
	edgeCurves := make(map[int][]bezier.CubicBezier)
	for _, e := range graph.Edges {
		curves := bezier.FitCurve(e.Points, opts.CurveTolerance)
		edgeCurves[e.ID] = curves
	}

	// 2. Map fitted curves for each region loop
	// Group path data strings by palette color index
	colorPaths := make(map[int][]string)

	for regID, loops := range graph.RegionLoops {
		seg, ok := sm.Segments[regID]
		if !ok {
			continue
		}

		var dCommands []string
		for _, loop := range loops {
			if len(loop.Edges) == 0 {
				continue
			}

			// Trace path commands for this loop
			var subpath strings.Builder
			firstPt := loop.Edges[0].Points[0]
			subpath.WriteString(fmt.Sprintf("M %.2f,%.2f", firstPt.X, firstPt.Y))

			for _, edge := range loop.Edges {
				var curves []bezier.CubicBezier
				if edge.ID > 0 {
					curves = edgeCurves[edge.ID]
				} else {
					// Reversed edge
					origCurves := edgeCurves[-edge.ID]
					curves = reverseCurves(origCurves)
				}

				for _, c := range curves {
					if c.IsCollinear(0.2) {
						subpath.WriteString(fmt.Sprintf(" L %.2f,%.2f", c.P3.X, c.P3.Y))
					} else {
						subpath.WriteString(" " + c.SVGCommand())
					}
				}
			}

			subpath.WriteString(" Z")
			dCommands = append(dCommands, subpath.String())
		}

		if len(dCommands) > 0 {
			colorPaths[seg.ColorIndex] = append(colorPaths[seg.ColorIndex], strings.Join(dCommands, " "))
		}
	}

	// 3. Assemble SVG XML document
	var buf bytes.Buffer
	buf.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" ?>\n")
	buf.WriteString("<!DOCTYPE svg PUBLIC \"-//W3C//DTD SVG 1.1//EN\" \"http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd\">\n")
	buf.WriteString(fmt.Sprintf("<svg width=\"%dpt\" height=\"%dpt\" viewBox=\"0 0 %d %d\" version=\"1.1\" xmlns=\"http://www.w3.org/2000/svg\">\n", w, h, w, h))

	// Emit layers by color
	for colorIdx, pathList := range colorPaths {
		if opts.TransparentBackground && colorIdx == len(sm.Palette)-1 {
			continue
		}

		col := sm.Palette[colorIdx]
		hex := col.Hex()

		strokeAttr := ""
		if opts.AntiSeamStroke {
			strokeAttr = fmt.Sprintf(" stroke=\"%s\" stroke-width=\"0.09375\"", hex)
		}

		combinedD := strings.Join(pathList, " ")
		buf.WriteString(fmt.Sprintf("  <path fill=\"%s\"%s opacity=\"1.00\" fill-rule=\"evenodd\" d=\"%s\" />\n",
			hex, strokeAttr, combinedD))
	}

	buf.WriteString("</svg>\n")
	return buf.String()
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
