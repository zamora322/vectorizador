package exporter

import (
	"bytes"
	"fmt"
	"strings"
)

// ExportSVG converts a VectorDocument into an SVG document string.
func ExportSVG(doc *VectorDocument, opts Options) string {
	w, h := doc.Width, doc.Height

	var buf bytes.Buffer
	buf.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" ?>\n")
	buf.WriteString("<!DOCTYPE svg PUBLIC \"-//W3C//DTD SVG 1.1//EN\" \"http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd\">\n")
	buf.WriteString(fmt.Sprintf("<svg width=\"%dpt\" height=\"%dpt\" viewBox=\"0 0 %d %d\" version=\"1.1\" xmlns=\"http://www.w3.org/2000/svg\">\n", w, h, w, h))

	for _, layer := range doc.Layers {
		hex := layer.Color.Hex()

		strokeAttr := ""
		if opts.AntiSeamStroke {
			strokeAttr = fmt.Sprintf(" stroke=\"%s\" stroke-width=\"0.09375\"", hex)
		}

		var pathCommands []string
		for _, loop := range layer.Loops {
			if len(loop.Segments) == 0 {
				continue
			}

			var subpath strings.Builder
			subpath.WriteString(fmt.Sprintf("M %.2f,%.2f", loop.StartPoint.X, loop.StartPoint.Y))

			for _, seg := range loop.Segments {
				if seg.IsLine {
					subpath.WriteString(fmt.Sprintf(" L %.2f,%.2f", seg.P3.X, seg.P3.Y))
				} else {
					subpath.WriteString(fmt.Sprintf(" C %.2f,%.2f %.2f,%.2f %.2f,%.2f",
						seg.P1.X, seg.P1.Y,
						seg.P2.X, seg.P2.Y,
						seg.P3.X, seg.P3.Y))
				}
			}

			subpath.WriteString(" Z")
			pathCommands = append(pathCommands, subpath.String())
		}

		if len(pathCommands) > 0 {
			combinedD := strings.Join(pathCommands, " ")
			buf.WriteString(fmt.Sprintf("  <g id=\"layer_%d_%s\">\n", layer.ColorIndex, hex[1:]))
			buf.WriteString(fmt.Sprintf("    <path fill=\"%s\"%s opacity=\"1.00\" fill-rule=\"evenodd\" d=\"%s\" />\n",
				hex, strokeAttr, combinedD))
			buf.WriteString("  </g>\n")
		}
	}

	buf.WriteString("</svg>\n")
	return buf.String()
}

