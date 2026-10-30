package exporter

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/zamora322/vectorizador/pkg/palette"
	"github.com/zamora322/vectorizador/pkg/topology"
)

func createTestDocument() *VectorDocument {
	// A document with 2 color layers:
	// Layer 0: Blue rectangle (straight lines)
	// Layer 1: Red curved shape (cubic Béziers)
	return &VectorDocument{
		Width:  200,
		Height: 150,
		Layers: []ColorLayer{
			{
				ColorIndex: 0,
				Color:      palette.RGB{R: 0, G: 120, B: 255},
				Loops: []PathLoop{
					{
						StartPoint: topology.Point{X: 10, Y: 10},
						Segments: []PathSegment{
							{IsLine: true, P0: topology.Point{X: 10, Y: 10}, P3: topology.Point{X: 100, Y: 10}},
							{IsLine: true, P0: topology.Point{X: 100, Y: 10}, P3: topology.Point{X: 100, Y: 80}},
							{IsLine: true, P0: topology.Point{X: 100, Y: 80}, P3: topology.Point{X: 10, Y: 80}},
							{IsLine: true, P0: topology.Point{X: 10, Y: 80}, P3: topology.Point{X: 10, Y: 10}},
						},
					},
				},
			},
			{
				ColorIndex: 1,
				Color:      palette.RGB{R: 255, G: 50, B: 50},
				Loops: []PathLoop{
					{
						StartPoint: topology.Point{X: 50, Y: 50},
						Segments: []PathSegment{
							{
								IsLine: false,
								P0:     topology.Point{X: 50, Y: 50},
								P1:     topology.Point{X: 80, Y: 20},
								P2:     topology.Point{X: 120, Y: 20},
								P3:     topology.Point{X: 150, Y: 50},
							},
							{
								IsLine: false,
								P0:     topology.Point{X: 150, Y: 50},
								P1:     topology.Point{X: 120, Y: 90},
								P2:     topology.Point{X: 80, Y: 90},
								P3:     topology.Point{X: 50, Y: 50},
							},
						},
					},
				},
			},
		},
	}
}

func TestExportSVG(t *testing.T) {
	doc := createTestDocument()
	opts := DefaultOptions()

	svgStr := ExportSVG(doc, opts)
	if !strings.Contains(svgStr, "<svg") || !strings.Contains(svgStr, "</svg>") {
		t.Fatalf("ExportSVG produced invalid SVG root: %s", svgStr)
	}
	if !strings.Contains(svgStr, "#0078ff") || !strings.Contains(svgStr, "#ff3232") {
		t.Errorf("ExportSVG missing color hex values: %s", svgStr)
	}
	if !strings.Contains(svgStr, "stroke-width=\"0.09375\"") {
		t.Errorf("ExportSVG missing anti-seam stroke: %s", svgStr)
	}
}

func TestExportEPS(t *testing.T) {
	doc := createTestDocument()
	opts := DefaultOptions()

	epsStr := ExportEPS(doc, opts)
	if !strings.HasPrefix(epsStr, "%!PS-Adobe-3.0 EPSF-3.0") {
		t.Fatalf("ExportEPS missing standard header: %s", epsStr[:100])
	}
	if !strings.Contains(epsStr, "%%BoundingBox: 0 0 200 150") {
		t.Errorf("ExportEPS missing expected BoundingBox: %s", epsStr)
	}
	if !strings.Contains(epsStr, "%%EOF") {
		t.Errorf("ExportEPS missing %%EOF trailer")
	}

	// Validate with Ghostscript if available
	gsPath, err := exec.LookPath("gs")
	if err == nil {
		tmpFile, err := os.CreateTemp("", "test_*.eps")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(epsStr); err != nil {
			t.Fatal(err)
		}
		tmpFile.Close()

		cmd := exec.Command(gsPath, "-q", "-dNODISPLAY", "-dBATCH", tmpFile.Name())
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Errorf("Ghostscript failed to interpret generated EPS: %v, output: %s", err, out.String())
		}
	}
}

func TestExportPDF(t *testing.T) {
	doc := createTestDocument()
	opts := DefaultOptions()

	pdfBytes := ExportPDF(doc, opts)
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-1.4")) {
		t.Fatalf("ExportPDF missing %%PDF-1.4 header")
	}
	if !bytes.Contains(pdfBytes, []byte("%%EOF")) {
		t.Fatalf("ExportPDF missing %%EOF marker")
	}

	// Validate with pdfinfo if available
	pdfinfoPath, err := exec.LookPath("pdfinfo")
	if err == nil {
		tmpFile, err := os.CreateTemp("", "test_*.pdf")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.Write(pdfBytes); err != nil {
			t.Fatal(err)
		}
		tmpFile.Close()

		cmd := exec.Command(pdfinfoPath, tmpFile.Name())
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Errorf("pdfinfo failed to parse generated PDF: %v, output: %s", err, out.String())
		}
		if !strings.Contains(out.String(), "Pages:") || !strings.Contains(out.String(), "200 x 150 pts") {
			t.Errorf("pdfinfo output unexpected: %s", out.String())
		}
	}
}

func TestExportDXF(t *testing.T) {
	doc := createTestDocument()
	opts := DefaultOptions()

	dxfStr := ExportDXF(doc, opts)
	if !strings.HasPrefix(dxfStr, "0\nSECTION\n2\nHEADER\n") {
		t.Fatalf("ExportDXF missing HEADER section: %s", dxfStr[:100])
	}
	if !strings.Contains(dxfStr, "AC1015") {
		t.Errorf("ExportDXF missing AutoCAD 2000 AC1015 version string")
	}
	if !strings.Contains(dxfStr, "LWPOLYLINE") {
		t.Errorf("ExportDXF missing LWPOLYLINE entity")
	}
	if !strings.HasSuffix(strings.TrimSpace(dxfStr), "0\nEOF") {
		t.Errorf("ExportDXF missing 0\\nEOF trailer")
	}

	// Validate with inkscape if available
	inkscapePath, err := exec.LookPath("inkscape")
	if err == nil {
		tmpFile, err := os.CreateTemp("", "test_*.dxf")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(dxfStr); err != nil {
			t.Fatal(err)
		}
		tmpFile.Close()

		cmd := exec.Command(inkscapePath, "--query-all", tmpFile.Name())
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Logf("Inkscape DXF query warning (non-fatal): %v, output: %s", err, out.String())
		}
	}
}

func TestExportDispatcher(t *testing.T) {
	doc := createTestDocument()
	opts := DefaultOptions()

	for _, fmtName := range SupportedFormats {
		data, err := Export(fmtName, doc, opts)
		if err != nil {
			t.Errorf("Export failed for format '%s': %v", fmtName, err)
		}
		if len(data) == 0 {
			t.Errorf("Export produced empty data for format '%s'", fmtName)
		}
	}

	_, err := Export("invalid_format", doc, opts)
	if err == nil {
		t.Errorf("Export expected error on invalid format")
	}
}
