package engine

import (
	"image"

	"github.com/zamora322/vectorizador/pkg/palette"
	"github.com/zamora322/vectorizador/pkg/segmentation"
	"github.com/zamora322/vectorizador/pkg/smoothing"
	"github.com/zamora322/vectorizador/pkg/svg"
	"github.com/zamora322/vectorizador/pkg/topology"
)

// Config holds all parameters controlling the vectorization pipeline.
type Config struct {
	Mode                  string  // "logo_aa" (default), "logo", "photo"
	Colors                int     // Number of colors (0 = auto-detect)
	Smoothness            float64 // Path smoothing factor (0.0 to 1.0, default 0.5)
	MinPixels             int     // Minimum segment size (default 4)
	DetectCorners         bool    // Preserve sharp corners (default true)
	AntiAliasingReject    bool    // Remove anti-aliasing edge slivers (default true)
	AntiSeamStroke        bool    // 0.09375pt stroke (default true)
	TransparentBackground bool    // Skip background color region (default false)
	CurveTolerance        float64 // Max pixel error for Bézier fitting (default 0.75)
}

// DefaultConfig returns production default vectorization parameters.
func DefaultConfig() Config {
	return Config{
		Mode:                  "logo_aa",
		Colors:                8,
		Smoothness:            0.5,
		MinPixels:             4,
		DetectCorners:         true,
		AntiAliasingReject:    true,
		AntiSeamStroke:        true,
		TransparentBackground: false,
		CurveTolerance:        0.75,
	}
}

// ProgressFunc reports pipeline execution milestones (0-100%).
type ProgressFunc func(stage string, percent int)

// Result contains the output of each pipeline stage.
type Result struct {
	Palette    palette.Palette
	SegmentMap *segmentation.SegmentMap
	Graph      *topology.PlanarGraph
	SVG        string
}

// Vectorize runs the full end-to-end vectorization pipeline.
func Vectorize(img image.Image, cfg Config, progress ProgressFunc) (*Result, error) {
	if progress == nil {
		progress = func(string, int) {}
	}

	// 1. Palette Selection
	progress("Extrayendo paleta cromática...", 15)
	colors := cfg.Colors
	if colors <= 0 {
		colors = 8
	}
	pal := palette.ExtractPalette(img, colors, 15)

	// 2. Image Segmentation
	progress("Segmentando regiones y analizando anti-aliasing...", 35)
	sm := segmentation.SegmentImage(img, pal, cfg.MinPixels, cfg.AntiAliasingReject)

	// 3. Topology & Boundary Tracing
	progress("Trazando grafo planar de fronteras compartidas...", 60)
	graph := topology.TraceTopology(sm)

	// 4. Path Smoothing & Corner Detection
	progress("Suavizando contornos y detectando esquinas vivas...", 80)
	smoothing.SmoothGraph(graph, cfg.Smoothness, cfg.DetectCorners)

	// 5. Bézier Fitting & SVG Generation
	progress("Ajustando curvas Bézier cúbicas y generando SVG...", 95)
	svgOpts := svg.SVGOptions{
		AntiSeamStroke:        cfg.AntiSeamStroke,
		TransparentBackground: cfg.TransparentBackground,
		GroupLayersByColor:    true,
		CurveTolerance:        cfg.CurveTolerance,
	}
	svgOutput := svg.GenerateSVG(graph, sm, svgOpts)

	progress("Completado", 100)

	return &Result{
		Palette:    pal,
		SegmentMap: sm,
		Graph:      graph,
		SVG:        svgOutput,
	}, nil
}
