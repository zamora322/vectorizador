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
	"time"

	"github.com/zamora322/vectorizador/pkg/engine"
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
	outputPath := flag.String("o", "output.svg", "Ruta de guardado para el archivo SVG")
	colors := flag.Int("colors", 8, "Número de colores de paleta (2 a 24)")
	smoothness := flag.Float64("smoothness", 0.5, "Factor de suavizado de contornos (0.0 a 1.0)")
	minPixels := flag.Int("min-pixels", 4, "Tamaño mínimo de segmento en píxeles")
	detectCorners := flag.Bool("corners", true, "Detectar y preservar esquinas vivas")
	rejectAA := flag.Bool("reject-aa", true, "Rechazar artefactos de anti-aliasing (SubPixel)")
	antiSeam := flag.Bool("anti-seam", true, "Aplicar trazo anti-seam (0.09375pt)")
	transparentBG := flag.Bool("transparent-bg", false, "Omitir el fondo para dejarlo transparente")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Vectorizador - Motor de Vectorización Open-Source \n\n")
		fmt.Fprintf(os.Stderr, "Uso:\n")
		fmt.Fprintf(os.Stderr, "  Modo CLI:\n")
		fmt.Fprintf(os.Stderr, "    vectorizador -i imagen.png -o vector.svg [opciones]\n\n")
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

	// 2. Configure pipeline
	cfg := engine.Config{
		Mode:                  "logo_aa",
		Colors:                *colors,
		Smoothness:            *smoothness,
		MinPixels:             *minPixels,
		DetectCorners:         *detectCorners,
		AntiAliasingReject:    *rejectAA,
		AntiSeamStroke:        *antiSeam,
		TransparentBackground: *transparentBG,
		CurveTolerance:        0.75,
	}

	start := time.Now()

	// 3. Run vectorization
	res, err := engine.Vectorize(img, cfg, func(stage string, percent int) {
		fmt.Printf("   [%3d%%] %s\n", percent, stage)
	})
	if err != nil {
		log.Fatalf("Error durante la vectorización: %v", err)
	}

	// 4. Save SVG output
	if err := os.WriteFile(*outputPath, []byte(res.SVG), 0644); err != nil {
		log.Fatalf("Error al escribir archivo SVG '%s': %v", *outputPath, err)
	}

	elapsed := time.Since(start)
	fmt.Printf("\n✨ Vectorización exitosa en %s!\n", elapsed.Round(time.Millisecond))
	fmt.Printf("   Colores: %d | Segmentos: %d | Aristas: %d\n", len(res.Palette), len(res.SegmentMap.Segments), len(res.Graph.Edges))
	fmt.Printf("   Archivo generado: %s (%d bytes)\n", *outputPath, len(res.SVG))
}
