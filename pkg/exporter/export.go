package exporter

import (
	"fmt"
	"strings"
)

// SupportedFormats lists all supported vector and CAD output formats.
var SupportedFormats = []string{"svg", "eps", "pdf", "dxf"}

// Export serializes the VectorDocument to the specified format ("svg", "eps", "pdf", "dxf").
func Export(format string, doc *VectorDocument, opts Options) ([]byte, error) {
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "svg":
		return []byte(ExportSVG(doc, opts)), nil
	case "eps":
		return []byte(ExportEPS(doc, opts)), nil
	case "pdf":
		return ExportPDF(doc, opts), nil
	case "dxf":
		return []byte(ExportDXF(doc, opts)), nil
	default:
		return nil, fmt.Errorf("unsupported export format '%s' (supported: svg, eps, pdf, dxf)", format)
	}
}

// ContentType returns the standard HTTP MIME type for a given format.
func ContentType(format string) string {
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "svg":
		return "image/svg+xml"
	case "eps":
		return "application/postscript"
	case "pdf":
		return "application/pdf"
	case "dxf":
		return "image/vnd.dxf"
	default:
		return "application/octet-stream"
	}
}

