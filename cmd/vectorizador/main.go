package main

import (
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zamora322/vectorizador/pkg/engine"
	"github.com/zamora322/vectorizador/pkg/exporter"
	"github.com/zamora322/vectorizador/pkg/server"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		runServer(os.Args[2:])
		return
	}

	runCLI()
}

func runServer(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 8080, "Puerto HTTP para el servidor del Asistente Web")
	noOpen := fs.Bool("no-open", false, "No abrir el navegador automáticamente")
	fs.Parse(args)

	srv := server.NewServer(*port)
	if err := srv.Start(!*noOpen); err != nil {
		log.Fatalf("Error al iniciar servidor: %v", err)
	}
}

func runCLI() {
	inputPath := flag.String("i", "", "Ruta de la imagen de entrada (PNG, JPG, BMP)")
	outputPath := flag.String("o", "output.svg", "Ruta de guardado (extensión .svg, .eps, .pdf, .dxf)")
	formatFlag := flag.String("format", "", "Formatos a exportar: svg, eps, pdf, dxf, all (o separados por coma)")
	modeFlag := flag.String("mode", "logo_aa", "Modo canónico: logo_aa (con anti-aliasing), logo_sharp (bordes nítidos / pixel art), photo (fotografía / tonos continuos)")
	qualityFlag := flag.String("quality", "medium", "Preset de calidad: high (alta), medium (media), low (baja)")
	colors := flag.Int("colors", 8, "Número de colores de paleta (2 a 24)")
	smoothness := flag.Float64("smoothness", 0.5, "Factor de suavizado de contornos (0.0 a 1.0)")
	minPixels := flag.Int("min-pixels", 4, "Tamaño mínimo de segmento en píxeles")
	detectCorners := flag.Bool("corners", true, "Detectar y preservar esquinas vivas")
	rejectAA := flag.Bool("reject-aa", true, "Rechazar artefactos de anti-aliasing (SubPixel)")
	antiSeam := flag.Bool("anti-seam", true, "Aplicar trazo anti-seam (0.09375pt)")
	transparentBG := flag.Bool("transparent-bg", false, "Omitir el fondo para dejarlo transparente")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Vectorizador - Motor de Vectorización Open-Source\n\n")
		fmt.Fprintf(os.Stderr, "Uso:\n")
		fmt.Fprintf(os.Stderr, "  Modo CLI:\n")
		fmt.Fprintf(os.Stderr, "    vectorizador -i logo.png -mode logo_aa -quality medium -o logo.svg\n")
		fmt.Fprintf(os.Stderr, "    vectorizador -i pixelart.png -mode logo_sharp -quality high -o pixel.svg\n")
		fmt.Fprintf(os.Stderr, "    vectorizador -i foto.jpg -mode photo -quality high -format all\n\n")
		fmt.Fprintf(os.Stderr, "  Modo Asistente Web (Wizard):\n")
		fmt.Fprintf(os.Stderr, "    vectorizador serve [-port 8080]\n\n")
		fmt.Fprintf(os.Stderr, "Opciones CLI:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *inputPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	// 1. Load image
	file, err := os.Open(*inputPath)
	if err != nil {
		log.Fatalf("Error al abrir imagen '%s': %v", *inputPath, err)
	}
	defer file.Close()

	img, format, err := image.Decode(file)
	if err != nil {
		log.Fatalf("Error al decodificar imagen: %v", err)
	}

	bounds := img.Bounds()
	fmt.Printf("🖼️  Cargada imagen '%s' (%s, %dx%d px)\n", filepath.Base(*inputPath), format, bounds.Dx(), bounds.Dy())

	// 2. Configure pipeline from canonical mode & quality preset
	m := engine.NormalizeMode(*modeFlag)
	q := engine.NormalizeQuality(*qualityFlag)
	cfg := engine.PresetConfig(m, q)

	if *transparentBG {
		cfg.TransparentBackground = true
	}
	if !*antiSeam {
		cfg.AntiSeamStroke = false
	}

	// Override specific parameters only if user explicitly passed them in CLI
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "colors":
			cfg.Colors = *colors
		case "smoothness":
			cfg.Smoothness = *smoothness
		case "min-pixels":
			cfg.MinPixels = *minPixels
		case "corners":
			cfg.DetectCorners = *detectCorners
		case "reject-aa":
			cfg.AntiAliasingReject = *rejectAA
		}
	})

	fmt.Printf("⚙️   Modo: %s | Calidad: %s\n", cfg.Mode.Description(), cfg.Quality.Description())

	start := time.Now()

	// 3. Run vectorization
	res, err := engine.Vectorize(img, cfg, func(stage string, percent int) {
		fmt.Printf("   [%3d%%] %s\n", percent, stage)
	})
	if err != nil {
		log.Fatalf("Error durante la vectorización: %v", err)
	}

	// 4. Determine formats to export
	formatsToExport := make(map[string]string) // format -> target path
	if *formatFlag != "" {
		reqFmts := strings.Split(*formatFlag, ",")
		baseName := strings.TrimSuffix(*outputPath, filepath.Ext(*outputPath))
		if baseName == "" || baseName == "output" {
			baseName = strings.TrimSuffix(filepath.Base(*inputPath), filepath.Ext(*inputPath))
		}

		for _, rf := range reqFmts {
			rf = strings.ToLower(strings.TrimSpace(rf))
			if rf == "all" {
				for _, f := range exporter.SupportedFormats {
					formatsToExport[f] = fmt.Sprintf("%s.%s", baseName, f)
				}
				break
			}
			if rf != "" {
				formatsToExport[rf] = fmt.Sprintf("%s.%s", baseName, rf)
			}
		}
	} else {
		// Detect format from -o extension
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(*outputPath), "."))
		if ext == "" {
			ext = "svg"
		}
		formatsToExport[ext] = *outputPath
	}

	// 5. Save outputs
	expOpts := exporter.Options{
		AntiSeamStroke:        *antiSeam,
		TransparentBackground: *transparentBG,
		CurveTolerance:        0.75,
		DXFPolylineTolerance:  0.25,
		CompressPDF:           true,
	}

	for fmtName, targetFile := range formatsToExport {
		data, err := res.Export(fmtName, expOpts)
		if err != nil {
			log.Fatalf("Error al exportar formato '%s': %v", fmtName, err)
		}

		if err := os.WriteFile(targetFile, data, 0644); err != nil {
			log.Fatalf("Error al escribir archivo '%s': %v", targetFile, err)
		}
		fmt.Printf("   💾 Exportado [%-3s]: %s (%d bytes)\n", strings.ToUpper(fmtName), targetFile, len(data))
	}

	elapsed := time.Since(start)
	fmt.Printf("\n✨ Vectorización completada en %s!\n", elapsed.Round(time.Millisecond))
	fmt.Printf("   Colores: %d | Segmentos: %d | Aristas: %d\n", len(res.Palette), len(res.SegmentMap.Segments), len(res.Graph.Edges))
}
