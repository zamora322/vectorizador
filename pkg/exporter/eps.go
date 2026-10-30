package exporter

import (
	"bytes"
	"fmt"
)

// ExportEPS converts a VectorDocument into Encapsulated PostScript (EPS 3.0).
func ExportEPS(doc *VectorDocument, opts Options) string {
	w, h := doc.Width, doc.Height
	hF := float64(h)

	var buf bytes.Buffer
	buf.WriteString("%!PS-Adobe-3.0 EPSF-3.0\n")
	buf.WriteString("%%Creator: Vectorizador\n")
	buf.WriteString("%%Title: Vector Output\n")
	buf.WriteString("%%Pages: 1\n")
	buf.WriteString(fmt.Sprintf("%%%%BoundingBox: 0 0 %d %d\n", w, h))
	buf.WriteString(fmt.Sprintf("%%%%HiResBoundingBox: 0.0000 0.0000 %.4f %.4f\n", float64(w), hF))
	buf.WriteString("%%LanguageLevel: 2\n")
	buf.WriteString("%%EndComments\n")
	buf.WriteString("%%Page: 1 1\n")

	// PostScript macro dictionary (compatible with Adobe Illustrator and CorelDRAW)
	buf.WriteString("/sc { setrgbcolor } def\n")
	buf.WriteString("/cp { closepath } def\n")
	buf.WriteString("/np { newpath } def\n")
	buf.WriteString("/lt { lineto } def\n")
	buf.WriteString("/ct { curveto } def\n")
	buf.WriteString("/mt { moveto } def\n")
	buf.WriteString("/Lb { 10 { pop } repeat } def\n") // Adobe Illustrator compatibility dummy
	buf.WriteString("/Ln { pop } def\n")               // Adobe Illustrator compatibility dummy
	if opts.AntiSeamStroke {
		buf.WriteString("/er { gsave eofill grestore 0.09375 setlinewidth stroke } def\n")
	} else {
		buf.WriteString("/er { eofill } def\n")
	}

	for _, layer := range doc.Layers {
		if len(layer.Loops) == 0 {
			continue
		}

		hex := layer.Color.Hex()
		r := float64(layer.Color.R) / 255.0
		g := float64(layer.Color.G) / 255.0
		b := float64(layer.Color.B) / 255.0

		// Adobe Illustrator Layer markers for seamless import into AI and CorelDRAW
		buf.WriteString("%AI5_BeginLayer\n")
		buf.WriteString("1 1 1 1 0 0 0 79 128 255 Lb\n")
		buf.WriteString(fmt.Sprintf("(Layer %d - %s) Ln\n", layer.ColorIndex, hex))
		buf.WriteString(fmt.Sprintf("%.4f %.4f %.4f sc\n", r, g, b))

		for _, loop := range layer.Loops {
			if len(loop.Segments) == 0 {
				continue
			}

			buf.WriteString("np\n")
			startY := hF - loop.StartPoint.Y
			buf.WriteString(fmt.Sprintf("%.2f %.2f mt\n", loop.StartPoint.X, startY))

			for _, seg := range loop.Segments {
				if seg.IsLine {
					endY := hF - seg.P3.Y
					buf.WriteString(fmt.Sprintf("%.2f %.2f lt\n", seg.P3.X, endY))
				} else {
					y1 := hF - seg.P1.Y
					y2 := hF - seg.P2.Y
					y3 := hF - seg.P3.Y
					buf.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f ct\n",
						seg.P1.X, y1,
						seg.P2.X, y2,
						seg.P3.X, y3))
				}
			}

			buf.WriteString("cp\n")
			buf.WriteString("er\n")
		}

		buf.WriteString("%AI5_EndLayer--\n")
	}

	buf.WriteString("%%Trailer\n")
	buf.WriteString("%%EOF\n")

	return buf.String()
}
