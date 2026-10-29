package svg

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/zamora322/vectorizador/pkg/palette"
	"github.com/zamora322/vectorizador/pkg/segmentation"
	"github.com/zamora322/vectorizador/pkg/topology"
)

func TestGenerateSVG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	for y := 2; y < 8; y++ {
		for x := 2; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	pal := palette.Palette{
		{R: 255, G: 0, B: 0},
		{R: 255, G: 255, B: 255},
	}

	sm := segmentation.SegmentImage(img, pal, 1, false)
	graph := topology.TraceTopology(sm)

	svgStr := GenerateSVG(graph, sm, DefaultSVGOptions())

	if !strings.Contains(svgStr, "<svg") || !strings.Contains(svgStr, "</svg>") {
		t.Fatalf("SVG output missing opening or closing tag")
	}
	if !strings.Contains(svgStr, "stroke-width=\"0.09375\"") {
		t.Errorf("SVG missing anti-seam stroke")
	}
}
