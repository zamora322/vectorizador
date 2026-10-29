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
	"strings"
	"sync"

	_ "golang.org/x/image/bmp"
	"golang.org/x/net/websocket"

	"github.com/zamora322/vectorizador/pkg/engine"
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

	// 5. WebSocket endpoint for live slider & editing interaction
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

	paletteHex := make([]string, len(res.Palette))
	for i, c := range res.Palette {
		paletteHex[i] = c.Hex()
	}

	segImg := res.SegmentMap.ToRGBA()
	var segBuf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	enc.Encode(&segBuf, segImg)
	segDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(segBuf.Bytes())

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"format":       format,
		"filename":     header.Filename,
		"width":        img.Bounds().Dx(),
		"height":       img.Bounds().Dy(),
		"palette":      paletteHex,
		"segmentation": segDataURL,
		"svg":          res.SVG,
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

	cfg := engine.DefaultConfig()

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

	paletteHex := make([]string, len(res.Palette))
	for i, c := range res.Palette {
		paletteHex[i] = c.Hex()
	}

	segImg := res.SegmentMap.ToRGBA()
	var segBuf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	enc.Encode(&segBuf, segImg)
	segDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(segBuf.Bytes())

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"format":       format,
		"filename":     name,
		"width":        img.Bounds().Dx(),
		"height":       img.Bounds().Dy(),
		"palette":      paletteHex,
		"segmentation": segDataURL,
		"svg":          res.SVG,
	})
}

type clientWSMessage struct {
	Action string `json:"action"` // "reprocess", "edit", "undo", "redo"
	Tool   string `json:"tool"`   // "recolor", "merge", "erase"
	X      int    `json:"x"`
	Y      int    `json:"y"`
	X2     int    `json:"x2"`
	Y2     int    `json:"y2"`
	Color  string `json:"color"`  // Hex, e.g. "#ff0000"
	Step   int    `json:"step"`
	Config struct {
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
		// Snapshot for undo
		s.undoStack = append(s.undoStack, s.cachedSegMap.Clone())
		if len(s.undoStack) > 30 {
			s.undoStack = s.undoStack[1:]
		}
		s.redoStack = nil

		switch msg.Tool {
		case "recolor":
			rgb := parseHexColor(msg.Color)
			if !s.cachedSegMap.RecolorSegmentAt(msg.X, msg.Y, rgb) {
				return nil, fmt.Errorf("coordenadas fuera de rango (%d, %d)", msg.X, msg.Y)
			}
		case "merge":
			if !s.cachedSegMap.MergeSegmentsAt(msg.X, msg.Y, msg.X2, msg.Y2) {
				return nil, fmt.Errorf("no se pudieron fusionar los segmentos (%d,%d) y (%d,%d)", msg.X, msg.Y, msg.X2, msg.Y2)
			}
		case "erase":
			if !s.cachedSegMap.EraseSegmentAt(msg.X, msg.Y) {
				return nil, fmt.Errorf("coordenadas fuera de rango (%d, %d)", msg.X, msg.Y)
			}
		default:
			return nil, fmt.Errorf("herramienta de edición no reconocida: %s", msg.Tool)
		}
	}

	// Fast re-vectorization without re-running K-Means
	res, err := engine.VectorizeFromSegmentMap(s.cachedSegMap, s.cachedCfg)
	if err != nil {
		return nil, fmt.Errorf("error al re-vectorizar: %w", err)
	}

	paletteHex := make([]string, len(res.Palette))
	for i, c := range res.Palette {
		paletteHex[i] = c.Hex()
	}

	segImg := res.SegmentMap.ToRGBA()
	var segBuf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	enc.Encode(&segBuf, segImg)
	segDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(segBuf.Bytes())

	return map[string]interface{}{
		"type":         "edit_result",
		"canUndo":      len(s.undoStack) > 0,
		"canRedo":      len(s.redoStack) > 0,
		"segmentation": segDataURL,
		"svg":          res.SVG,
		"palette":      paletteHex,
	}, nil
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
		if msg.Action == "edit" || msg.Action == "undo" || msg.Action == "redo" {
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
			Mode:                  "logo_aa",
			Colors:                msg.Config.Colors,
			MinPixels:             msg.Config.MinPixels,
			AntiAliasingReject:    msg.Config.RejectAA,
			Smoothness:            msg.Config.Smoothness,
			DetectCorners:         msg.Config.DetectCorners,
			CurveTolerance:        msg.Config.Tolerance,
			AntiSeamStroke:        msg.Config.AntiSeam,
			TransparentBackground: msg.Config.TransparentBG,
		}

		if cfg.Colors <= 0 {
			cfg.Colors = 6
		}
		if cfg.MinPixels <= 0 {
			cfg.MinPixels = 4
		}

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

		paletteHex := make([]string, len(result.Palette))
		for i, c := range result.Palette {
			paletteHex[i] = c.Hex()
		}
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

func sendJSON(ws *websocket.Conn, v interface{}) {
	data, err := json.Marshal(v)
	if err == nil {
		websocket.Message.Send(ws, string(data))
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
