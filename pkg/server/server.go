package server

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	_ "golang.org/x/image/bmp"
	"golang.org/x/net/websocket"

	"github.com/zamora322/vectorizador/pkg/engine"
	"github.com/zamora322/vectorizador/pkg/exporter"
	"github.com/zamora322/vectorizador/pkg/palette"
	"github.com/zamora322/vectorizador/pkg/segmentation"
)

//go:embed all:web/*
var webFS embed.FS

// Server wraps the HTTP and WebSocket endpoints for the wizard.
type Server struct {
	port         int
	mu           sync.RWMutex
	cachedImg    image.Image
	cachedCfg    engine.Config
	cachedSegMap *segmentation.SegmentMap
	undoStack    []*segmentation.SegmentMap
	redoStack    []*segmentation.SegmentMap
	wsMu         sync.Mutex
	activeWS     map[*websocket.Conn]bool
}

// NewServer creates a new server instance.
func NewServer(port int) *Server {
	return &Server{
		port:      port,
		cachedCfg: engine.DefaultConfig(),
		activeWS:  make(map[*websocket.Conn]bool),
	}
}

// Start launches the web server and optionally opens the default browser.
func (s *Server) Start(autoOpen bool) error {
	mux := http.NewServeMux()

	// 1. Static Web UI files
	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		return fmt.Errorf("failed to load embedded web assets: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	// 2. Samples listing and file endpoint
	mux.HandleFunc("/api/samples", s.handleListSamples)
	mux.HandleFunc("/api/samples/load", s.handleLoadSample)
	mux.HandleFunc("/api/samples/", s.handleSampleImage)

	// 3. Reliable HTTP multipart upload endpoint for files of any size
	mux.HandleFunc("/api/upload", s.handleUpload)

	// 4. Interactive segment editing REST endpoint
	mux.HandleFunc("/api/segments/edit", s.handleSegmentEditHTTP)

	// 5. Quick Palettes candidate suggestions
	mux.HandleFunc("/api/palettes/suggest", s.handleSuggestPalettes)

	// 6. Multi-format vector export endpoint (SVG, EPS, PDF, DXF)
	mux.HandleFunc("/api/export", s.handleExport)

	// 7. WebSocket endpoint for live slider & editing interaction
	mux.Handle("/ws", websocket.Handler(s.handleWebSocket))

	addr := fmt.Sprintf(":%d", s.port)
	url := fmt.Sprintf("http://localhost:%d", s.port)
	log.Printf("🚀 Vectorizador Studio iniciado en: %s", url)

	if autoOpen {
		go openBrowser(url)
	}

	return http.ListenAndServe(addr, mux)
}

func (s *Server) broadcastProgress(stage string, percent int) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()

	msg := map[string]interface{}{
		"type":    "progress",
		"stage":   stage,
		"percent": percent,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	payload := string(data)
	for ws := range s.activeWS {
		websocket.Message.Send(ws, payload)
	}
}

func (s *Server) handleListSamples(w http.ResponseWriter, r *http.Request) {
	seen := make(map[string]bool)
	var samples []string

	// Scan testdata directory
	if entries, err := os.ReadDir("testdata"); err == nil {
		for _, e := range entries {
			if !e.IsDir() && isImageFile(e.Name()) {
				seen[e.Name()] = true
				samples = append(samples, e.Name())
			}
		}
	}

	// Scan embedded web/samples
	if entries, err := webFS.ReadDir("web/samples"); err == nil {
		for _, e := range entries {
			if !e.IsDir() && isImageFile(e.Name()) && !seen[e.Name()] {
				samples = append(samples, e.Name())
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(samples)
}

func isImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".bmp" || ext == ".gif"
}

func (s *Server) handleSampleImage(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)

	diskPath := filepath.Join("testdata", name)
	data, err := os.ReadFile(diskPath)
	if err != nil {
		samplePath := "web/samples/" + name
		data, err = webFS.ReadFile(samplePath)
		if err != nil {
			http.Error(w, "Muestra no encontrada", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(data)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Método no permitido"})
		return
	}

	// 64MB limit for upload
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Archivo demasiado grande o datos inválidos: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "No se recibió archivo en el formulario: " + err.Error()})
		return
	}
	defer file.Close()

	img, format, err := image.Decode(file)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": fmt.Sprintf("Formato de imagen no soportado o archivo corrupto (%s): %v", header.Filename, err)})
		return
	}

	s.mu.Lock()
	s.cachedImg = img
	s.mu.Unlock()

	// Parse optional config
	cfg := engine.DefaultConfig()
	if cfgStr := r.FormValue("config"); cfgStr != "" {
		var clientCfg struct {
			Colors        int     `json:"colors"`
			MinPixels     int     `json:"min_pixels"`
			RejectAA      bool    `json:"reject_aa"`
			Smoothness    float64 `json:"smoothness"`
			DetectCorners bool    `json:"detect_corners"`
			Tolerance     float64 `json:"tolerance"`
			AntiSeam      bool    `json:"anti_seam"`
			TransparentBG bool    `json:"transparent_bg"`
		}
		if err := json.Unmarshal([]byte(cfgStr), &clientCfg); err == nil {
			if clientCfg.Colors > 0 {
				cfg.Colors = clientCfg.Colors
			}
			if clientCfg.MinPixels > 0 {
				cfg.MinPixels = clientCfg.MinPixels
			}
			cfg.AntiAliasingReject = clientCfg.RejectAA
			cfg.Smoothness = clientCfg.Smoothness
			cfg.DetectCorners = clientCfg.DetectCorners
			cfg.CurveTolerance = clientCfg.Tolerance
			cfg.AntiSeamStroke = clientCfg.AntiSeam
			cfg.TransparentBackground = clientCfg.TransparentBG
		}
	}

	detectedColors := palette.DetectOptimalColorCount(img)
	if cfg.Colors <= 0 {
		cfg.Colors = detectedColors
	}

	res, err := engine.Vectorize(img, cfg, func(stage string, percent int) {
		s.broadcastProgress(stage, percent)
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Error al vectorizar: " + err.Error()})
		return
	}

	s.mu.Lock()
	s.cachedCfg = cfg
	s.cachedSegMap = res.SegmentMap.Clone()
	s.undoStack = nil
	s.redoStack = nil
	s.mu.Unlock()

	paletteHex := extractPaletteHex(res)

	segImg := res.SegmentMap.ToRGBA()
	var segBuf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	enc.Encode(&segBuf, segImg)
	segDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(segBuf.Bytes())

	suggestedData := s.generateSuggestedPalettes(img)
	candidates := suggestedData["candidates"]

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"format":          format,
		"filename":        header.Filename,
		"width":           img.Bounds().Dx(),
		"height":          img.Bounds().Dy(),
		"palette":         paletteHex,
		"detected_colors": detectedColors,
		"candidates":      candidates,
		"segmentation":    segDataURL,
		"svg":             res.SVG,
	})
}

func (s *Server) handleLoadSample(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	name := r.URL.Query().Get("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Parámetro 'name' requerido"})
		return
	}

	diskPath := filepath.Join("testdata", name)
	var (
		img    image.Image
		format string
		err    error
	)

	f, err := os.Open(diskPath)
	if err == nil {
		defer f.Close()
		img, format, err = image.Decode(f)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Error al decodificar muestra: " + err.Error()})
			return
		}
	} else {
		// Fallback to embedded web/samples
		ef, err2 := webFS.Open("web/samples/" + name)
		if err2 == nil {
			defer ef.Close()
			img, format, err = image.Decode(ef)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Error al decodificar muestra embebida: " + err.Error()})
				return
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Muestra no encontrada: " + name})
			return
		}
	}

	s.mu.Lock()
	s.cachedImg = img
	s.mu.Unlock()

	var cfg engine.Config
	if strings.Contains(strings.ToLower(name), "sharp") {
		cfg = engine.PresetConfig(engine.ModeLogoSharp, engine.QualityMedium)
	} else if strings.Contains(strings.ToLower(name), "curved") {
		cfg = engine.PresetConfig(engine.ModePhoto, engine.QualityMedium)
	} else {
		cfg = engine.PresetConfig(engine.ModeLogoAA, engine.QualityMedium)
	}

	detectedColors := palette.DetectOptimalColorCount(img)
	if cfg.Colors <= 0 {
		cfg.Colors = detectedColors
	}

	res, err := engine.Vectorize(img, cfg, func(stage string, percent int) {
		s.broadcastProgress(stage, percent)
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Error en el motor: " + err.Error()})
		return
	}

	s.mu.Lock()
	s.cachedCfg = cfg
	s.cachedSegMap = res.SegmentMap.Clone()
	s.undoStack = nil
	s.redoStack = nil
	s.mu.Unlock()

	paletteHex := extractPaletteHex(res)

	segImg := res.SegmentMap.ToRGBA()
	var segBuf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	enc.Encode(&segBuf, segImg)
	segDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(segBuf.Bytes())

	suggestedData := s.generateSuggestedPalettes(img)
	candidates := suggestedData["candidates"]

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"format":          format,
		"filename":        name,
		"mode":            string(cfg.Mode),
		"quality":         string(cfg.Quality),
		"width":           img.Bounds().Dx(),
		"height":          img.Bounds().Dy(),
		"palette":         paletteHex,
		"detected_colors": detectedColors,
		"candidates":      candidates,
		"segmentation":    segDataURL,
		"svg":             res.SVG,
	})
}

type clientWSMessage struct {
	Action      string               `json:"action"` // "reprocess", "edit", "undo", "redo"
	Tool        string               `json:"tool"`   // "recolor", "merge", "erase", "zap", "pencil", "undo", "redo"
	X           int                  `json:"x"`
	Y           int                  `json:"y"`
	X2          int                  `json:"x2"`
	Y2          int                  `json:"y2"`
	TargetX     int                  `json:"target_x"`
	TargetY     int                  `json:"target_y"`
	Color       string               `json:"color"`        // Hex, e.g. "#ff0000"
	StrokeWidth int                  `json:"stroke_width"` // For pencil
	Points      []segmentation.Point `json:"points"`       // For pencil stroke
	Step        int                  `json:"step"`
	Config      struct {
		Mode          string  `json:"mode"`
		Quality       string  `json:"quality"`
		Colors        int     `json:"colors"`
		MinPixels     int     `json:"min_pixels"`
		RejectAA      bool    `json:"reject_aa"`
		Smoothness    float64 `json:"smoothness"`
		DetectCorners bool    `json:"detect_corners"`
		Tolerance     float64 `json:"tolerance"`
		AntiSeam      bool    `json:"anti_seam"`
		TransparentBG bool    `json:"transparent_bg"`
	} `json:"config"`
}

func (s *Server) handleSegmentEditHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Método no permitido"})
		return
	}

	var msg clientWSMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "JSON inválido: " + err.Error()})
		return
	}

	resMap, err := s.executeSegmentAction(msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	resMap["success"] = true
	s.broadcastJSON(resMap)
	json.NewEncoder(w).Encode(resMap)
}

func (s *Server) executeSegmentAction(msg clientWSMessage) (map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cachedSegMap == nil {
		return nil, fmt.Errorf("no hay segmentación cargada en el motor. Por favor carga una imagen primero")
	}

	// Auto-infer action if missing or tool provided directly
	if msg.Action == "" {
		if msg.Tool == "undo" || msg.Tool == "redo" {
			msg.Action = msg.Tool
		} else if msg.Tool == "recolor" || msg.Tool == "merge" || msg.Tool == "erase" || msg.Tool == "zap" || msg.Tool == "pencil" {
			msg.Action = "edit"
		}
	}

	// Support TargetX / TargetY aliases for merge
	if msg.X2 == 0 && msg.Y2 == 0 {
		if msg.TargetX != 0 || msg.TargetY != 0 {
			msg.X2 = msg.TargetX
			msg.Y2 = msg.TargetY
		}
	}

	switch msg.Action {
	case "undo":
		if len(s.undoStack) == 0 {
			return nil, fmt.Errorf("no hay más acciones para deshacer")
		}
		s.redoStack = append(s.redoStack, s.cachedSegMap.Clone())
		s.cachedSegMap = s.undoStack[len(s.undoStack)-1]
		s.undoStack = s.undoStack[:len(s.undoStack)-1]

	case "redo":
		if len(s.redoStack) == 0 {
			return nil, fmt.Errorf("no hay más acciones para rehacer")
		}
		s.undoStack = append(s.undoStack, s.cachedSegMap.Clone())
		s.cachedSegMap = s.redoStack[len(s.redoStack)-1]
		s.redoStack = s.redoStack[:len(s.redoStack)-1]

	case "edit":
		snapshot := s.cachedSegMap.Clone()
		var editErr error

		switch msg.Tool {
		case "recolor":
			if strings.ToLower(msg.Color) == "transparent" {
				if !s.cachedSegMap.EraseSegmentAt(msg.X, msg.Y) {
					editErr = fmt.Errorf("coordenadas fuera de rango (%d, %d)", msg.X, msg.Y)
				}
			} else {
				rgb := parseHexColor(msg.Color)
				if !s.cachedSegMap.RecolorSegmentAt(msg.X, msg.Y, rgb) {
					editErr = fmt.Errorf("coordenadas fuera de rango (%d, %d)", msg.X, msg.Y)
				}
			}
		case "merge":
			if !s.cachedSegMap.MergeSegmentsAt(msg.X, msg.Y, msg.X2, msg.Y2) {
				editErr = fmt.Errorf("no se pudieron fusionar los segmentos (%d,%d) y (%d,%d)", msg.X, msg.Y, msg.X2, msg.Y2)
			}
		case "erase":
			if !s.cachedSegMap.EraseSegmentAt(msg.X, msg.Y) {
				editErr = fmt.Errorf("coordenadas fuera de rango (%d, %d)", msg.X, msg.Y)
			}
		case "zap":
			if !s.cachedSegMap.ZapSegmentAt(msg.X, msg.Y) {
				editErr = fmt.Errorf("no se pudo eliminar y redistribuir el segmento en (%d, %d)", msg.X, msg.Y)
			}
		case "pencil":
			rgb := parseHexColor(msg.Color)
			width := msg.StrokeWidth
			if width < 1 {
				width = 2
			}
			pts := msg.Points
			if len(pts) == 0 && (msg.X != 0 || msg.Y != 0) {
				pts = []segmentation.Point{{X: msg.X, Y: msg.Y}}
			}
			if !s.cachedSegMap.PencilStroke(pts, width, rgb) {
				editErr = fmt.Errorf("no se pudo aplicar el trazo de lápiz")
			}
		default:
			editErr = fmt.Errorf("herramienta de edición no reconocida: %s", msg.Tool)
		}

		if editErr != nil {
			return nil, editErr
		}

		// Snapshot is only preserved on undo stack when edit succeeds
		s.undoStack = append(s.undoStack, snapshot)
		if len(s.undoStack) > 30 {
			s.undoStack = s.undoStack[1:]
		}
		s.redoStack = nil

	default:
		return nil, fmt.Errorf("acción no reconocida: %s", msg.Action)
	}

	// Fast re-vectorization without re-running K-Means
	res, err := engine.VectorizeFromSegmentMap(s.cachedSegMap, s.cachedCfg)
	if err != nil {
		return nil, fmt.Errorf("error al re-vectorizar: %w", err)
	}

	paletteHex := extractPaletteHex(res)

	segImg := res.SegmentMap.ToRGBA()
	var segBuf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	enc.Encode(&segBuf, segImg)
	segDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(segBuf.Bytes())

	return map[string]interface{}{
		"type":         "edit_result",
		"canUndo":      len(s.undoStack) > 0,
		"canRedo":      len(s.redoStack) > 0,
		"can_undo":     len(s.undoStack) > 0,
		"can_redo":     len(s.redoStack) > 0,
		"segmentation": segDataURL,
		"svg":          res.SVG,
		"palette":      paletteHex,
	}, nil
}

func extractPaletteHex(res *engine.Result) []string {
	if res == nil {
		return nil
	}
	hasTransparent := false
	if res.SegmentMap != nil {
		for _, seg := range res.SegmentMap.Segments {
			if seg.Transparent {
				hasTransparent = true
				break
			}
		}
	}
	paletteHex := make([]string, 0, len(res.Palette)+1)
	if hasTransparent {
		paletteHex = append(paletteHex, "transparent")
	}
	for _, c := range res.Palette {
		paletteHex = append(paletteHex, c.Hex())
	}
	return paletteHex
}

func parseHexColor(s string) palette.RGB {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return palette.RGB{R: 0, G: 0, B: 0}
	}
	var r, g, b uint8
	fmt.Sscanf(s[0:2], "%x", &r)
	fmt.Sscanf(s[2:4], "%x", &g)
	fmt.Sscanf(s[4:6], "%x", &b)
	return palette.RGB{R: r, G: g, B: b}
}

func (s *Server) broadcastJSON(v interface{}) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()

	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	payload := string(data)
	for ws := range s.activeWS {
		websocket.Message.Send(ws, payload)
	}
}

func (s *Server) handleWebSocket(ws *websocket.Conn) {
	s.wsMu.Lock()
	if s.activeWS == nil {
		s.activeWS = make(map[*websocket.Conn]bool)
	}
	s.activeWS[ws] = true
	s.wsMu.Unlock()

	defer func() {
		s.wsMu.Lock()
		delete(s.activeWS, ws)
		s.wsMu.Unlock()
		ws.Close()
	}()

	ws.MaxPayloadBytes = 64 << 20

	for {
		var rawMsg string
		err := websocket.Message.Receive(ws, &rawMsg)
		if err != nil {
			if err != io.EOF {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msg clientWSMessage
		if err := json.Unmarshal([]byte(rawMsg), &msg); err != nil {
			log.Printf("JSON unmarshal error: %v", err)
			sendJSON(ws, map[string]interface{}{"type": "error", "error": "Datos WebSocket malformados: " + err.Error()})
			continue
		}

		// Handle live segment editing actions
		if msg.Action == "edit" || msg.Action == "undo" || msg.Action == "redo" ||
			msg.Tool == "recolor" || msg.Tool == "merge" || msg.Tool == "erase" ||
			msg.Tool == "zap" || msg.Tool == "pencil" ||
			msg.Tool == "undo" || msg.Tool == "redo" {
			resMap, err := s.executeSegmentAction(msg)
			if err != nil {
				sendJSON(ws, map[string]interface{}{"type": "error", "error": err.Error()})
			} else {
				s.broadcastJSON(resMap)
			}
			continue
		}

		s.mu.RLock()
		targetImg := s.cachedImg
		s.mu.RUnlock()

		if targetImg == nil {
			sendJSON(ws, map[string]interface{}{"type": "error", "error": "No hay ninguna imagen cargada en el servidor. Por favor sube una imagen primero."})
			continue
		}

		cfg := engine.Config{
			Mode:                  engine.NormalizeMode(msg.Config.Mode),
			Quality:               engine.NormalizeQuality(msg.Config.Quality),
			Colors:                msg.Config.Colors,
			MinPixels:             msg.Config.MinPixels,
			AntiAliasingReject:    msg.Config.RejectAA,
			Smoothness:            msg.Config.Smoothness,
			DetectCorners:         msg.Config.DetectCorners,
			CurveTolerance:        msg.Config.Tolerance,
			AntiSeamStroke:        msg.Config.AntiSeam,
			TransparentBackground: msg.Config.TransparentBG,
		}
		cfg.ApplyPreset()

		result, err := engine.Vectorize(targetImg, cfg, func(stage string, percent int) {
			s.broadcastProgress(stage, percent)
		})

		if err != nil {
			sendJSON(ws, map[string]interface{}{"type": "error", "error": err.Error()})
			continue
		}

		s.mu.Lock()
		s.cachedCfg = cfg
		s.cachedSegMap = result.SegmentMap.Clone()
		s.undoStack = nil
		s.redoStack = nil
		s.mu.Unlock()

		paletteHex := extractPaletteHex(result)
		sendJSON(ws, map[string]interface{}{
			"type":   "palette",
			"colors": paletteHex,
		})

		segImg := result.SegmentMap.ToRGBA()
		var segBuf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		if err := enc.Encode(&segBuf, segImg); err == nil {
			segDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(segBuf.Bytes())
			sendJSON(ws, map[string]interface{}{
				"type":    "segmentation",
				"preview": segDataURL,
			})
		}

		sendJSON(ws, map[string]interface{}{
			"type": "svg",
			"svg":  result.SVG,
		})
	}
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "svg"
	}
	format = strings.ToLower(strings.TrimSpace(format))

	s.mu.RLock()
	segMap := s.cachedSegMap
	cfg := s.cachedCfg
	s.mu.RUnlock()

	if segMap == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No hay ninguna imagen vectorizada disponible para exportar.",
		})
		return
	}

	// Read optional overrides from query params
	if as := r.URL.Query().Get("anti_seam"); as != "" {
		cfg.AntiSeamStroke = as == "true"
	}
	if tbg := r.URL.Query().Get("transparent_bg"); tbg != "" {
		cfg.TransparentBackground = tbg == "true"
	}

	res, err := engine.VectorizeFromSegmentMap(segMap, cfg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Error al generar geometría vectorial: " + err.Error(),
		})
		return
	}

	data, err := res.Export(format)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	filename := fmt.Sprintf("vectorizado.%s", format)
	w.Header().Set("Content-Type", exporter.ContentType(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func sendJSON(ws *websocket.Conn, v interface{}) {
	data, err := json.Marshal(v)
	if err == nil {
		websocket.Message.Send(ws, string(data))
	}
}

type PaletteCandidate struct {
	Colors    int      `json:"colors"`
	Palette   []string `json:"palette"`
	IsOptimal bool     `json:"is_optimal"`
}

func (s *Server) handleSuggestPalettes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.mu.RLock()
	img := s.cachedImg
	s.mu.RUnlock()

	if img == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "No hay imagen cargada en el motor"})
		return
	}

	res := s.generateSuggestedPalettes(img)
	json.NewEncoder(w).Encode(res)
}

func (s *Server) generateSuggestedPalettes(img image.Image) map[string]interface{} {
	if img == nil {
		return map[string]interface{}{"success": false, "candidates": []PaletteCandidate{}}
	}

	optimal := palette.DetectOptimalColorCount(img)
	candidatesList := []int{2, 4, 6, 8, 12, 16}

	hasOptimal := false
	for _, c := range candidatesList {
		if c == optimal {
			hasOptimal = true
			break
		}
	}
	if !hasOptimal && optimal > 1 && optimal <= 24 {
		candidatesList = append(candidatesList, optimal)
		sort.Ints(candidatesList)
	}

	var candidates []PaletteCandidate
	for _, k := range candidatesList {
		pal := palette.ExtractPalette(img, k, 10)
		hexes := make([]string, len(pal))
		for i, rgb := range pal {
			hexes[i] = rgb.Hex()
		}
		candidates = append(candidates, PaletteCandidate{
			Colors:    k,
			Palette:   hexes,
			IsOptimal: (k == optimal),
		})
	}

	return map[string]interface{}{
		"success":    true,
		"suggested":  optimal,
		"candidates": candidates,
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}
