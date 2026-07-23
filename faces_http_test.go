package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestFaceHandler(t *testing.T) *FaceHandler {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/faces.db"

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("create faces db: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE faces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sha256 TEXT NOT NULL, relpath TEXT NOT NULL, face_idx INTEGER NOT NULL,
			bbox TEXT NOT NULL, embedding BLOB NOT NULL, gender INTEGER, age INTEGER,
			thumb_path TEXT, scanned_at REAL NOT NULL, UNIQUE(sha256, face_idx)
		);
		CREATE TABLE scan_progress (sha256 TEXT PRIMARY KEY, status TEXT NOT NULL, face_count INTEGER DEFAULT 0, scanned_at REAL);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	emb := makeEmbedding([]float32{1.0, 0.0, 0.0, 0.0})
	_, err = conn.Exec(`INSERT INTO faces (sha256, relpath, face_idx, bbox, embedding, gender, age, thumb_path, scanned_at) VALUES
		('sha001', 'cat/2024-01/IMG_001.jpg', 0, '[10,20,30,40]', ?, 1, 30, '', 1700000000)`, emb)
	if err != nil {
		t.Fatalf("insert face: %v", err)
	}
	_, err = conn.Exec(`INSERT INTO scan_progress VALUES ('sha001', 'done', 1, 1700000000)`)
	if err != nil {
		t.Fatalf("insert progress: %v", err)
	}
	conn.Close()

	fdb, err := OpenFaceDB(dbPath)
	if err != nil {
		t.Fatalf("OpenFaceDB: %v", err)
	}
	return NewFaceHandler(fdb, tmpDir+"/media")
}

func TestFaceHandlerAvailable(t *testing.T) {
	fh := setupTestFaceHandler(t)
	if !fh.Available() {
		t.Error("expected Available() to return true")
	}

	var nilFh *FaceHandler
	if nilFh.Available() {
		t.Error("nil handler should not be available")
	}
}

func TestFaceHandlerRegisterRoutes(t *testing.T) {
	fh := setupTestFaceHandler(t)
	mux := http.NewServeMux()
	fh.RegisterRoutes(mux)

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/faces/status")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestHandleFaces(t *testing.T) {
	fh := setupTestFaceHandler(t)

	req := httptest.NewRequest("GET", "/api/faces?limit=10", nil)
	rec := httptest.NewRecorder()
	fh.handleFaces(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)
	if result["total"].(float64) != 1 {
		t.Errorf("expected total 1, got %v", result["total"])
	}
}

func TestHandleFacesDisabled(t *testing.T) {
	var fh *FaceHandler
	req := httptest.NewRequest("GET", "/api/faces", nil)
	rec := httptest.NewRecorder()
	fh.handleFaces(rec, req)

	if rec.Code != 503 {
		t.Errorf("expected 503 when disabled, got %d", rec.Code)
	}
}

func TestHandleFaceStatus(t *testing.T) {
	fh := setupTestFaceHandler(t)

	req := httptest.NewRequest("GET", "/api/faces/status", nil)
	req.URL.Path = "/api/faces/status"
	rec := httptest.NewRecorder()
	fh.handleFaceRoute(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleFaceByID(t *testing.T) {
	fh := setupTestFaceHandler(t)

	req := httptest.NewRequest("GET", "/api/faces/1", nil)
	req.URL.Path = "/api/faces/1"
	rec := httptest.NewRecorder()
	fh.handleFaceRoute(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleFaceSimilar(t *testing.T) {
	fh := setupTestFaceHandler(t)

	req := httptest.NewRequest("GET", "/api/faces/1/similar?threshold=0.0", nil)
	req.URL.Path = "/api/faces/1/similar"
	rec := httptest.NewRecorder()
	fh.handleFaceRoute(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleFaceInvalidID(t *testing.T) {
	fh := setupTestFaceHandler(t)

	req := httptest.NewRequest("GET", "/api/faces/abc", nil)
	req.URL.Path = "/api/faces/abc"
	rec := httptest.NewRecorder()
	fh.handleFaceRoute(rec, req)

	if rec.Code != 400 {
		t.Errorf("expected 400 for invalid id, got %d", rec.Code)
	}
}

func TestHandleFaceRouteDisabled(t *testing.T) {
	var fh *FaceHandler
	req := httptest.NewRequest("GET", "/api/faces/1", nil)
	rec := httptest.NewRecorder()
	fh.handleFaceRoute(rec, req)

	if rec.Code != 503 {
		t.Errorf("expected 503 when disabled, got %d", rec.Code)
	}
}

func TestHandleFaceThumb(t *testing.T) {
	fh := setupTestFaceHandler(t)

	// Face has no thumb_path, should redirect to full thumb
	req := httptest.NewRequest("GET", "/api/face-thumb/1", nil)
	req.URL.Path = "/api/face-thumb/1"
	rec := httptest.NewRecorder()
	fh.handleFaceThumb(rec, req)

	// Should redirect (302) since thumb_path is empty
	if rec.Code != 302 {
		t.Errorf("expected 302 redirect, got %d", rec.Code)
	}
}

func TestHandleFaceThumbWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/faces.db"

	conn, _ := sql.Open("sqlite", dbPath)
	conn.Exec(`
		CREATE TABLE faces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sha256 TEXT NOT NULL, relpath TEXT NOT NULL, face_idx INTEGER NOT NULL,
			bbox TEXT NOT NULL, embedding BLOB NOT NULL, gender INTEGER, age INTEGER,
			thumb_path TEXT, scanned_at REAL NOT NULL, UNIQUE(sha256, face_idx)
		);
		CREATE TABLE scan_progress (sha256 TEXT PRIMARY KEY, status TEXT NOT NULL, face_count INTEGER DEFAULT 0, scanned_at REAL);
	`)

	// Create a real thumb file
	thumbPath := filepath.Join(tmpDir, "face_thumb.jpg")
	os.WriteFile(thumbPath, []byte("fake-jpeg-data"), 0644)

	emb := makeEmbedding([]float32{1.0, 0.0})
	conn.Exec(`INSERT INTO faces (sha256, relpath, face_idx, bbox, embedding, gender, age, thumb_path, scanned_at) VALUES
		('sha001', 'cat/2024-01/IMG_001.jpg', 0, '[10,20,30,40]', ?, 1, 30, ?, 1700000000)`, emb, thumbPath)
	conn.Close()

	fdb, _ := OpenFaceDB(dbPath)
	fh := NewFaceHandler(fdb, tmpDir+"/media")

	req := httptest.NewRequest("GET", "/api/face-thumb/1", nil)
	req.URL.Path = "/api/face-thumb/1"
	rec := httptest.NewRecorder()
	fh.handleFaceThumb(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200 for existing thumb, got %d", rec.Code)
	}
}

func TestHandleFaceThumbInvalidID(t *testing.T) {
	fh := setupTestFaceHandler(t)

	req := httptest.NewRequest("GET", "/api/face-thumb/abc", nil)
	req.URL.Path = "/api/face-thumb/abc"
	rec := httptest.NewRecorder()
	fh.handleFaceThumb(rec, req)

	if rec.Code != 400 {
		t.Errorf("expected 400 for invalid id, got %d", rec.Code)
	}
}

func TestHandleFaceThumbNotFound(t *testing.T) {
	fh := setupTestFaceHandler(t)

	req := httptest.NewRequest("GET", "/api/face-thumb/999", nil)
	req.URL.Path = "/api/face-thumb/999"
	rec := httptest.NewRecorder()
	fh.handleFaceThumb(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404 for missing face, got %d", rec.Code)
	}
}

func TestHandleFaceThumbDisabled(t *testing.T) {
	var fh *FaceHandler
	req := httptest.NewRequest("GET", "/api/face-thumb/1", nil)
	rec := httptest.NewRecorder()
	fh.handleFaceThumb(rec, req)

	if rec.Code != 503 {
		t.Errorf("expected 503 when disabled, got %d", rec.Code)
	}
}

func TestFileExists(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(tmpFile, []byte("test"), 0644)

	if !fileExists(tmpFile) {
		t.Error("expected true for existing file")
	}
	if fileExists("/nonexistent/path/file.txt") {
		t.Error("expected false for non-existing file")
	}
}
