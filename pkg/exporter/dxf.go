package exporter

import (
	"bytes"
	"fmt"
	"math"

	"github.com/zamora322/vectorizador/pkg/palette"
	"github.com/zamora322/vectorizador/pkg/topology"
)

// ExportDXF converts a VectorDocument into AutoCAD 2000 DXF format (AC1015).
func ExportDXF(doc *VectorDocument, opts Options) string {
	w, h := doc.Width, doc.Height
	hF := float64(h)

	tol := opts.DXFPolylineTolerance
	if tol <= 0 {
		tol = 0.25
	}

	var buf bytes.Buffer

	// 1. SECTION HEADER
	buf.WriteString("0\nSECTION\n2\nHEADER\n")
	buf.WriteString("9\n$ACADVER\n1\nAC1015\n")
	buf.WriteString("9\n$EXTMIN\n10\n0.0\n20\n0.0\n30\n0.0\n")
	buf.WriteString(fmt.Sprintf("9\n$EXTMAX\n10\n%.4f\n20\n%.4f\n30\n0.0\n", float64(w), hF))
	buf.WriteString("0\nENDSEC\n")

	// 2. SECTION TABLES
	buf.WriteString("0\nSECTION\n2\nTABLES\n")

	// Line types table
	buf.WriteString("0\nTABLE\n2\nLTYPE\n70\n1\n")
	buf.WriteString("0\nLTYPE\n2\nCONTINUOUS\n70\n0\n3\nSolid line\n72\n65\n73\n0\n40\n0.0\n")
	buf.WriteString("0\nENDTAB\n")

	// Layers table
	buf.WriteString("0\nTABLE\n2\nLAYER\n")
	buf.WriteString(fmt.Sprintf("70\n%d\n", len(doc.Layers)+1))

	// Layer 0 (default CAD layer)
	buf.WriteString("0\nLAYER\n2\n0\n70\n0\n62\n7\n6\nCONTINUOUS\n")

	for _, layer := range doc.Layers {
		hex := layer.Color.Hex()[1:] // Strip '#'
		layerName := fmt.Sprintf("COLOR_%s", hex)
		aci := rgbToACI(layer.Color)
		trueColor := (int(layer.Color.R) << 16) | (int(layer.Color.G) << 8) | int(layer.Color.B)

		buf.WriteString("0\nLAYER\n")
		buf.WriteString(fmt.Sprintf("2\n%s\n", layerName))
		buf.WriteString("70\n0\n")
		buf.WriteString(fmt.Sprintf("62\n%d\n", aci))
		buf.WriteString(fmt.Sprintf("420\n%d\n", trueColor))
		buf.WriteString("6\nCONTINUOUS\n")
	}
	buf.WriteString("0\nENDTAB\n")
	buf.WriteString("0\nENDSEC\n")

	// 3. SECTION BLOCKS
	buf.WriteString("0\nSECTION\n2\nBLOCKS\n")
	buf.WriteString("0\nENDSEC\n")

	// 4. SECTION ENTITIES
	buf.WriteString("0\nSECTION\n2\nENTITIES\n")

	handle := 0x100
	for _, layer := range doc.Layers {
		hex := layer.Color.Hex()[1:]
		layerName := fmt.Sprintf("COLOR_%s", hex)

		for _, loop := range layer.Loops {
			if len(loop.Segments) == 0 {
				continue
			}

			// Sample vertices along loop
			startPt := topology.Point{X: loop.StartPoint.X, Y: hF - loop.StartPoint.Y}
			vertices := []topology.Point{startPt}

			for _, seg := range loop.Segments {
				if seg.IsLine {
					vertices = append(vertices, topology.Point{X: seg.P3.X, Y: hF - seg.P3.Y})
				} else {
					p0 := topology.Point{X: seg.P0.X, Y: hF - seg.P0.Y}
					p1 := topology.Point{X: seg.P1.X, Y: hF - seg.P1.Y}
					p2 := topology.Point{X: seg.P2.X, Y: hF - seg.P2.Y}
					p3 := topology.Point{X: seg.P3.X, Y: hF - seg.P3.Y}
					subdivideBezier(p0, p1, p2, p3, tol, 0, &vertices)
				}
			}

			// If last vertex is identical to first, remove it since flag 70=1 closes polyline
			if len(vertices) > 1 && vertices[len(vertices)-1].Distance(vertices[0]) < 1e-4 {
				vertices = vertices[:len(vertices)-1]
			}

			if len(vertices) < 3 {
				continue
			}

			handle++
			buf.WriteString("0\nLWPOLYLINE\n")
			buf.WriteString(fmt.Sprintf("5\n%X\n", handle))
			buf.WriteString("100\nAcDbEntity\n")
			buf.WriteString(fmt.Sprintf("8\n%s\n", layerName))
			buf.WriteString("100\nAcDbPolyline\n")
			buf.WriteString(fmt.Sprintf("90\n%d\n", len(vertices)))
			buf.WriteString("70\n1\n") // Flag 1 = Closed polyline
			buf.WriteString("43\n0.0\n")

			for _, v := range vertices {
				buf.WriteString(fmt.Sprintf("10\n%.4f\n20\n%.4f\n", v.X, v.Y))
			}
		}
	}
	buf.WriteString("0\nENDSEC\n")

	// 5. EOF
	buf.WriteString("0\nEOF\n")

	return buf.String()
}

// subdivideBezier recursively samples cubic Bézier control points into smooth vertices.
func subdivideBezier(p0, p1, p2, p3 topology.Point, tol float64, depth int, pts *[]topology.Point) {
	if depth >= 6 || isFlat(p0, p1, p2, p3, tol) {
		*pts = append(*pts, p3)
		return
	}

	// De Casteljau subdivision at t = 0.5
	p01 := midpoint(p0, p1)
	p12 := midpoint(p1, p2)
	p23 := midpoint(p2, p3)
	p012 := midpoint(p01, p12)
	p123 := midpoint(p12, p23)
	mid := midpoint(p012, p123)

	subdivideBezier(p0, p01, p012, mid, tol, depth+1, pts)
	subdivideBezier(mid, p123, p23, p3, tol, depth+1, pts)
}

func midpoint(a, b topology.Point) topology.Point {
	return topology.Point{
		X: (a.X + b.X) * 0.5,
		Y: (a.Y + b.Y) * 0.5,
	}
}

func isFlat(p0, p1, p2, p3 topology.Point, tol float64) bool {
	d1 := pointToLineDist(p1, p0, p3)
	d2 := pointToLineDist(p2, p0, p3)
	return d1 <= tol && d2 <= tol
}

func pointToLineDist(p, l0, l1 topology.Point) float64 {
	dx := l1.X - l0.X
	dy := l1.Y - l0.Y
	lenSq := dx*dx + dy*dy
	if lenSq < 1e-6 {
		return p.Distance(l0)
	}
	num := math.Abs(dy*p.X - dx*p.Y + l1.X*l0.Y - l1.Y*l0.X)
	return num / math.Sqrt(lenSq)
}

// rgbToACI maps an RGB color to the closest standard AutoCAD Color Index (1-7).
func rgbToACI(c palette.RGB) int {
	// Standard AutoCAD colors
	aciTable := []struct {
		aci int
		r, g, b int
	}{
		{1, 255, 0, 0},     // Red
		{2, 255, 255, 0},   // Yellow
		{3, 0, 255, 0},     // Green
		{4, 0, 255, 255},   // Cyan
		{5, 0, 0, 255},     // Blue
		{6, 255, 0, 255},   // Magenta
		{7, 255, 255, 255}, // White / Black
	}

	bestACI := 7
	minDist := math.MaxFloat64

	for _, entry := range aciTable {
		dr := float64(int(c.R) - entry.r)
		dg := float64(int(c.G) - entry.g)
		db := float64(int(c.B) - entry.b)
		dist := dr*dr + dg*dg + db*db
		if dist < minDist {
			minDist = dist
			bestACI = entry.aci
		}
	}

	return bestACI
}

