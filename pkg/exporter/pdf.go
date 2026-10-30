package exporter

import (
	"bytes"
	"compress/zlib"
	"fmt"
)

// ExportPDF converts a VectorDocument into a standalone PDF 1.4 document.
func ExportPDF(doc *VectorDocument, opts Options) []byte {
	w, h := doc.Width, doc.Height
	hF := float64(h)

	// 1. Build graphics content stream
	var stream bytes.Buffer
	for _, layer := range doc.Layers {
		if len(layer.Loops) == 0 {
			continue
		}

		r := float64(layer.Color.R) / 255.0
		g := float64(layer.Color.G) / 255.0
		b := float64(layer.Color.B) / 255.0

		stream.WriteString("q\n")
		stream.WriteString(fmt.Sprintf("%.4f %.4f %.4f rg\n", r, g, b))
		stream.WriteString(fmt.Sprintf("%.4f %.4f %.4f RG\n", r, g, b))
		if opts.AntiSeamStroke {
			stream.WriteString("0.09375 w\n")
		}

		for _, loop := range layer.Loops {
			if len(loop.Segments) == 0 {
				continue
			}

			startY := hF - loop.StartPoint.Y
			stream.WriteString(fmt.Sprintf("%.2f %.2f m\n", loop.StartPoint.X, startY))

			for _, seg := range loop.Segments {
				if seg.IsLine {
					endY := hF - seg.P3.Y
					stream.WriteString(fmt.Sprintf("%.2f %.2f l\n", seg.P3.X, endY))
				} else {
					y1 := hF - seg.P1.Y
					y2 := hF - seg.P2.Y
					y3 := hF - seg.P3.Y
					stream.WriteString(fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f c\n",
						seg.P1.X, y1,
						seg.P2.X, y2,
						seg.P3.X, y3))
				}
			}

			stream.WriteString("h\n")
		}

		if opts.AntiSeamStroke {
			stream.WriteString("B*\n") // Fill evenodd and stroke
		} else {
			stream.WriteString("f*\n") // Fill evenodd
		}
		stream.WriteString("Q\n")
	}

	streamBytes := stream.Bytes()
	isFlate := opts.CompressPDF
	if isFlate {
		var zbuf bytes.Buffer
		zw := zlib.NewWriter(&zbuf)
		_, err := zw.Write(streamBytes)
		if err == nil && zw.Close() == nil {
			streamBytes = zbuf.Bytes()
		} else {
			isFlate = false
		}
	}

	// 2. Assemble PDF document structure and record xref offsets
	var out bytes.Buffer
	offsets := make([]int, 5)

	out.WriteString("%PDF-1.4\n")
	out.WriteString("%\xe2\xe3\xcf\xd3\n")

	// Object 1: Catalog
	offsets[1] = out.Len()
	out.WriteString("1 0 obj\n")
	out.WriteString("<<\n")
	out.WriteString("  /Type /Catalog\n")
	out.WriteString("  /Pages 2 0 R\n")
	out.WriteString(">>\n")
	out.WriteString("endobj\n")

	// Object 2: Pages
	offsets[2] = out.Len()
	out.WriteString("2 0 obj\n")
	out.WriteString("<<\n")
	out.WriteString("  /Type /Pages\n")
	out.WriteString("  /Kids [3 0 R]\n")
	out.WriteString("  /Count 1\n")
	out.WriteString(">>\n")
	out.WriteString("endobj\n")

	// Object 3: Page
	offsets[3] = out.Len()
	out.WriteString("3 0 obj\n")
	out.WriteString("<<\n")
	out.WriteString("  /Type /Page\n")
	out.WriteString("  /Parent 2 0 R\n")
	out.WriteString(fmt.Sprintf("  /MediaBox [0 0 %d %d]\n", w, h))
	out.WriteString("  /Contents 4 0 R\n")
	out.WriteString("  /Resources << >>\n")
	out.WriteString(">>\n")
	out.WriteString("endobj\n")

	// Object 4: Contents Stream
	offsets[4] = out.Len()
	out.WriteString("4 0 obj\n")
	out.WriteString("<<\n")
	out.WriteString(fmt.Sprintf("  /Length %d\n", len(streamBytes)))
	if isFlate {
		out.WriteString("  /Filter /FlateDecode\n")
	}
	out.WriteString(">>\n")
	out.WriteString("stream\n")
	out.Write(streamBytes)
	out.WriteString("\nendstream\n")
	out.WriteString("endobj\n")

	// Cross-reference table
	startxref := out.Len()
	out.WriteString("xref\n")
	out.WriteString("0 5\n")
	out.WriteString("0000000000 65535 f \n")
	for i := 1; i <= 4; i++ {
		out.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}

	// Trailer
	out.WriteString("trailer\n")
	out.WriteString("<<\n")
	out.WriteString("  /Size 5\n")
	out.WriteString("  /Root 1 0 R\n")
	out.WriteString(">>\n")
	out.WriteString("startxref\n")
	out.WriteString(fmt.Sprintf("%d\n", startxref))
	out.WriteString("%%EOF\n")

	return out.Bytes()
}

