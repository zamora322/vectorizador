package engine

import (
	"image"
	"log"
	"time"

	"github.com/zamora322/vectorizador/pkg/exporter"
	"github.com/zamora322/vectorizador/pkg/palette"
	"github.com/zamora322/vectorizador/pkg/segmentation"
	"github.com/zamora322/vectorizador/pkg/smoothing"
	"github.com/zamora322/vectorizador/pkg/topology"
)

// Config holds all parameters controlling the vectorization pipeline.
type Config struct {
	Mode                  Mode    `json:"mode"`           // "logo_aa" (default), "logo_sharp", "photo"
	Quality               Quality `json:"quality"`        // "low", "medium" (default), "high"
	Colors                int     `json:"colors"`         // Number of colors (0 = auto-detect)
	Smoothness            float64 `json:"smoothness"`     // Path smoothing factor (0.0 to 1.0)
	MinPixels             int     `json:"min_pixels"`     // Minimum segment size in pixels
	DetectCorners         bool    `json:"detect_corners"` // Preserve sharp corners (acute angles >= 60 deg)
	AntiAliasingReject    bool    `json:"reject_aa"`      // Remove anti-aliasing edge slivers
	AntiSeamStroke        bool    `json:"anti_seam"`      // 0.09375pt stroke to avoid raster seams
	TransparentBackground bool    `json:"transparent_bg"` // Skip background color region
	CurveTolerance        float64 `json:"tolerance"`      // Max pixel error for Bézier fitting
}

// DefaultConfig returns production default vectorization parameters (logo_aa / medium quality).
func DefaultConfig() Config {
	return PresetConfig(ModeLogoAA, QualityMedium)
}

// ProgressFunc reports pipeline execution milestones (0-100%).
type ProgressFunc func(stage string, percent int)

// Result contains the output of each pipeline stage and the unified vector geometry.
type Result struct {
	Palette    palette.Palette
	SegmentMap *segmentation.SegmentMap
	Graph      *topology.PlanarGraph
	Doc        *exporter.VectorDocument
	SVG        string
}

// ToSVG returns the SVG string representation of the result.
func (r *Result) ToSVG(opts ...exporter.Options) string {
	if len(opts) > 0 {
		return exporter.ExportSVG(r.Doc, opts[0])
	}
	return r.SVG
}

// ToEPS returns the Encapsulated PostScript (EPS 3.0) representation of the result.
func (r *Result) ToEPS(opts ...exporter.Options) string {
	opt := exporter.DefaultOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}
	return exporter.ExportEPS(r.Doc, opt)
}

// ToPDF returns the standalone PDF 1.4 document bytes of the result.
func (r *Result) ToPDF(opts ...exporter.Options) []byte {
	opt := exporter.DefaultOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}
	return exporter.ExportPDF(r.Doc, opt)
}

// ToDXF returns the AutoCAD 2000 DXF representation of the result.
func (r *Result) ToDXF(opts ...exporter.Options) string {
	opt := exporter.DefaultOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}
	return exporter.ExportDXF(r.Doc, opt)
}

// Export serializes the result into the requested format ("svg", "eps", "pdf", "dxf").
func (r *Result) Export(format string, opts ...exporter.Options) ([]byte, error) {
	opt := exporter.DefaultOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}
	return exporter.Export(format, r.Doc, opt)
}

// Vectorize runs the full end-to-end vectorization pipeline.
func Vectorize(img image.Image, cfg Config, progress ProgressFunc) (*Result, error) {
	start := time.Now()
	if progress == nil {
		progress = func(string, int) {}
	}

	cfg.ApplyPreset()

	// 1. Palette Selection
	progress("Extrayendo paleta cromática...", 15)
	colors := cfg.Colors
	if colors <= 0 {
		colors = palette.DetectOptimalColorCount(img)
		if cfg.Mode == ModePhoto && colors < 12 {
			colors = 12
		}
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

	// 5. Bézier Fitting & Unified Vector Document Assembly
	progress("Ajustando curvas Bézier cúbicas y ensamblando geometría...", 95)
	expOpts := exporter.Options{
		AntiSeamStroke:        cfg.AntiSeamStroke,
		TransparentBackground: cfg.TransparentBackground,
		CurveTolerance:        cfg.CurveTolerance,
		CompressPDF:           true,
	}
	doc := exporter.BuildDocument(graph, sm, expOpts)
	svgOutput := exporter.ExportSVG(doc, expOpts)

	progress("Completado", 100)
	log.Printf("⚡ [Engine] Vectorización completada en %v (%dx%d px, %d colores, modo=%s/%s)", time.Since(start), img.Bounds().Dx(), img.Bounds().Dy(), len(pal), cfg.Mode, cfg.Quality)

	return &Result{
		Palette:    pal,
		SegmentMap: sm,
		Graph:      graph,
		Doc:        doc,
		SVG:        svgOutput,
	}, nil
}

// VectorizeFromSegmentMap re-traces topology and generates SVG from an existing SegmentMap
// without re-running color quantization or initial pixel segmentation.
func VectorizeFromSegmentMap(sm *segmentation.SegmentMap, cfg Config) (*Result, error) {
	start := time.Now()
	cfg.ApplyPreset()

	// 1. Topology & Boundary Tracing
	graph := topology.TraceTopology(sm)

	// 2. Path Smoothing & Corner Detection
	smoothing.SmoothGraph(graph, cfg.Smoothness, cfg.DetectCorners)

	// 3. Bézier Fitting & Unified Vector Document Assembly
	expOpts := exporter.Options{
		AntiSeamStroke:        cfg.AntiSeamStroke,
		TransparentBackground: cfg.TransparentBackground,
		CurveTolerance:        cfg.CurveTolerance,
		CompressPDF:           true,
	}
	doc := exporter.BuildDocument(graph, sm, expOpts)
	svgOutput := exporter.ExportSVG(doc, expOpts)
	log.Printf("⚡ [Engine] Re-vectorización interactiva completada en %v (sm: %dx%d)", time.Since(start), sm.Width, sm.Height)

	return &Result{
		Palette:    sm.Palette,
		SegmentMap: sm,
		Graph:      graph,
		Doc:        doc,
		SVG:        svgOutput,
	}, nil
}
