package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerEndpoints(t *testing.T) {
	srv := NewServer(0)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/samples", srv.handleListSamples)
	mux.HandleFunc("/api/samples/load", srv.handleLoadSample)
	mux.HandleFunc("/api/palettes/suggest", srv.handleSuggestPalettes)
	mux.HandleFunc("/api/export", srv.handleExport)
	mux.HandleFunc("/api/segments/edit", srv.handleSegmentEditHTTP)

	// 1. Test /api/samples
	req1 := httptest.NewRequest("GET", "/api/samples", nil)
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("Expected 200 from /api/samples, got %d", w1.Code)
	}

	var samples []string
	if err := json.Unmarshal(w1.Body.Bytes(), &samples); err != nil {
		t.Fatalf("Failed to parse samples JSON: %v", err)
	}
	if len(samples) == 0 {
		t.Fatalf("Expected sample images, got 0")
	}

	// 2. Test /api/samples/load
	req2 := httptest.NewRequest("POST", "/api/samples/load?name="+samples[0], nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("Expected 200 from /api/samples/load, got %d: %s", w2.Code, w2.Body.String())
	}

	var loadRes map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &loadRes); err != nil {
		t.Fatalf("Failed to parse load result: %v", err)
	}
	if loadRes["success"] != true {
		t.Fatalf("Expected success true, got: %v", loadRes)
	}

	// 3. Test /api/export for all 4 formats
	formats := []string{"svg", "pdf", "eps", "dxf"}
	for _, fmtName := range formats {
		reqExport := httptest.NewRequest("GET", "/api/export?format="+fmtName, nil)
		wExport := httptest.NewRecorder()
		mux.ServeHTTP(wExport, reqExport)

		if wExport.Code != http.StatusOK {
			t.Errorf("Expected 200 from /api/export?format=%s, got %d: %s", fmtName, wExport.Code, wExport.Body.String())
		}
		if wExport.Body.Len() == 0 {
			t.Errorf("Expected non-empty body from /api/export?format=%s", fmtName)
		}
		disp := wExport.Header().Get("Content-Disposition")
		if !strings.Contains(disp, "."+fmtName) {
			t.Errorf("Expected Content-Disposition to contain .%s, got %s", fmtName, disp)
		}
	}

	// 4. Test /api/segments/edit (Recolor, Erase, Undo, Redo, and auto-inference)
	editPayload := strings.NewReader(`{"action":"edit","tool":"recolor","x":5,"y":5,"color":"#123456"}`)
	reqEdit := httptest.NewRequest("POST", "/api/segments/edit", editPayload)
	wEdit := httptest.NewRecorder()
	mux.ServeHTTP(wEdit, reqEdit)
	if wEdit.Code != http.StatusOK {
		t.Fatalf("Expected 200 from /api/segments/edit recolor, got %d: %s", wEdit.Code, wEdit.Body.String())
	}

	var editRes map[string]interface{}
	if err := json.Unmarshal(wEdit.Body.Bytes(), &editRes); err != nil {
		t.Fatalf("Failed to parse edit result: %v", err)
	}
	if editRes["canUndo"] != true || editRes["can_undo"] != true {
		t.Errorf("Expected canUndo true after edit, got %v", editRes)
	}

	// Test erase
	erasePayload := strings.NewReader(`{"action":"edit","tool":"erase","x":5,"y":5}`)
	reqErase := httptest.NewRequest("POST", "/api/segments/edit", erasePayload)
	wErase := httptest.NewRecorder()
	mux.ServeHTTP(wErase, reqErase)
	if wErase.Code != http.StatusOK {
		t.Fatalf("Expected 200 from erase, got %d: %s", wErase.Code, wErase.Body.String())
	}

	// Test undo
	undoPayload := strings.NewReader(`{"action":"undo"}`)
	reqUndo := httptest.NewRequest("POST", "/api/segments/edit", undoPayload)
	wUndo := httptest.NewRecorder()
	mux.ServeHTTP(wUndo, reqUndo)
	if wUndo.Code != http.StatusOK {
		t.Fatalf("Expected 200 from undo, got %d: %s", wUndo.Code, wUndo.Body.String())
	}

	// Test redo
	redoPayload := strings.NewReader(`{"action":"redo"}`)
	reqRedo := httptest.NewRequest("POST", "/api/segments/edit", redoPayload)
	wRedo := httptest.NewRecorder()
	mux.ServeHTTP(wRedo, reqRedo)
	if wRedo.Code != http.StatusOK {
		t.Fatalf("Expected 200 from redo, got %d: %s", wRedo.Code, wRedo.Body.String())
	}

	// Test auto-inference without explicit action
	inferPayload := strings.NewReader(`{"tool":"recolor","x":5,"y":5,"color":"#654321"}`)
	reqInfer := httptest.NewRequest("POST", "/api/segments/edit", inferPayload)
	wInfer := httptest.NewRecorder()
	mux.ServeHTTP(wInfer, reqInfer)
	if wInfer.Code != http.StatusOK {
		t.Fatalf("Expected 200 from inferred edit, got %d: %s", wInfer.Code, wInfer.Body.String())
	}

	// 5. Test Zap tool
	zapPayload := strings.NewReader(`{"action":"edit","tool":"zap","x":5,"y":5}`)
	reqZap := httptest.NewRequest("POST", "/api/segments/edit", zapPayload)
	wZap := httptest.NewRecorder()
	mux.ServeHTTP(wZap, reqZap)
	if wZap.Code != http.StatusOK {
		t.Fatalf("Expected 200 from Zap tool, got %d: %s", wZap.Code, wZap.Body.String())
	}

	// 6. Test Pencil tool
	pencilPayload := strings.NewReader(`{"action":"edit","tool":"pencil","stroke_width":2,"color":"#00ff00","points":[{"x":2,"y":2},{"x":8,"y":8}]}`)
	reqPencil := httptest.NewRequest("POST", "/api/segments/edit", pencilPayload)
	wPencil := httptest.NewRecorder()
	mux.ServeHTTP(wPencil, reqPencil)
	if wPencil.Code != http.StatusOK {
		t.Fatalf("Expected 200 from Pencil tool, got %d: %s", wPencil.Code, wPencil.Body.String())
	}

	// 7. Test /api/palettes/suggest
	reqSuggest := httptest.NewRequest("GET", "/api/palettes/suggest", nil)
	wSuggest := httptest.NewRecorder()
	mux.ServeHTTP(wSuggest, reqSuggest)
	if wSuggest.Code != http.StatusOK {
		t.Fatalf("Expected 200 from /api/palettes/suggest, got %d: %s", wSuggest.Code, wSuggest.Body.String())
	}
	var suggestRes map[string]interface{}
	if err := json.Unmarshal(wSuggest.Body.Bytes(), &suggestRes); err != nil {
		t.Fatalf("Failed to parse suggest response: %v", err)
	}
	if suggestRes["success"] != true {
		t.Fatalf("Expected success true from /api/palettes/suggest")
	}
	candidates, ok := suggestRes["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		t.Fatalf("Expected non-empty candidates list in suggest response")
	}
}

