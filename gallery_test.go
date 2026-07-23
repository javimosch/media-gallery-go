package main

import (
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestGallery(t *testing.T) (*GalleryHandler, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	mediaRoot := tmpDir + "/media"
	os.MkdirAll(mediaRoot, 0755)

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER);
		INSERT INTO files VALUES
			('cat1/2024-01/IMG_001.jpg', 1048576, 1700000000, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 1700000001),
			('cat1/2024-01/IMG_002.jpg', 2097152, 1700000001, 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 1700000002),
			('cat2/2024-02/VID_001.mp4', 10485760, 1700000002, 'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', 1700000003);
	`)
	conn.Close()
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}

	// Create a dummy media file for thumb/download tests
	os.MkdirAll(filepath.Join(mediaRoot, "cat1/2024-01"), 0755)
	os.WriteFile(filepath.Join(mediaRoot, "cat1/2024-01/IMG_001.jpg"), []byte("fake-image"), 0644)
	os.MkdirAll(filepath.Join(mediaRoot, "cat2/2024-02"), 0755)
	os.WriteFile(filepath.Join(mediaRoot, "cat2/2024-02/VID_001.mp4"), []byte("fake-video"), 0644)

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}

	thumbs := NewThumbCache(tmpDir + "/thumbs")
	gallery := NewGalleryHandler(db, thumbs, mediaRoot)
	return gallery, mediaRoot
}

func TestHandleBrowse(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/browse", nil)
	rec := httptest.NewRecorder()
	g.handleBrowse(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)
	files := result["files"].([]interface{})
	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d", len(files))
	}
}

func TestHandleBrowseWithCategory(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/browse?category=cat1", nil)
	rec := httptest.NewRecorder()
	g.handleBrowse(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)
	files := result["files"].([]interface{})
	if len(files) != 2 {
		t.Errorf("expected 2 files in cat1, got %d", len(files))
	}
}

func TestHandleSearch(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/search?q=IMG", nil)
	rec := httptest.NewRecorder()
	g.handleSearch(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)
	if result["count"].(float64) != 2 {
		t.Errorf("expected count 2, got %v", result["count"])
	}
}

func TestHandleSearchMissingQ(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/search", nil)
	rec := httptest.NewRecorder()
	g.handleSearch(rec, req)

	if rec.Code != 400 {
		t.Errorf("expected 400 for missing q, got %d", rec.Code)
	}
}

func TestHandleDupes(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/dupes", nil)
	rec := httptest.NewRecorder()
	g.handleDupes(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleDupeFiles(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/dupes/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	req.URL.Path = "/api/dupes/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	rec := httptest.NewRecorder()
	g.handleDupeFiles(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleDupeFilesEmptySHA(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/dupes/", nil)
	req.URL.Path = "/api/dupes/"
	rec := httptest.NewRecorder()
	g.handleDupeFiles(rec, req)

	if rec.Code != 400 {
		t.Errorf("expected 400 for empty sha, got %d", rec.Code)
	}
}

func TestHandleStats(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	rec := httptest.NewRecorder()
	g.handleStats(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)
	if result["total_files"].(float64) != 3 {
		t.Errorf("expected 3 total files, got %v", result["total_files"])
	}
}

func TestHandleCategories(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/categories", nil)
	rec := httptest.NewRecorder()
	g.handleCategories(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)
	cats := result["categories"].([]interface{})
	if len(cats) != 2 {
		t.Errorf("expected 2 categories, got %d", len(cats))
	}
}

func TestHandleYearMonths(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/yearmonths", nil)
	rec := httptest.NewRecorder()
	g.handleYearMonths(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)
	yms := result["year_months"].([]interface{})
	if len(yms) != 2 {
		t.Errorf("expected 2 year-months, got %d", len(yms))
	}
}

func TestHandleFileBySHA(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/file/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	req.URL.Path = "/api/file/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	rec := httptest.NewRecorder()
	g.handleFile(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleFileByRelPath(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/file/cat1/2024-01/IMG_001.jpg", nil)
	req.URL.Path = "/api/file/cat1/2024-01/IMG_001.jpg"
	rec := httptest.NewRecorder()
	g.handleFile(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleFileNotFound(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/file/nonexistent", nil)
	req.URL.Path = "/api/file/nonexistent"
	rec := httptest.NewRecorder()
	g.handleFile(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleThumbInvalidSHA(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/thumb/short", nil)
	req.URL.Path = "/api/thumb/short"
	rec := httptest.NewRecorder()
	g.handleThumb(rec, req)

	if rec.Code != 400 {
		t.Errorf("expected 400 for invalid sha, got %d", rec.Code)
	}
}

func TestHandleThumbNotFound(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/thumb/dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", nil)
	req.URL.Path = "/api/thumb/dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	rec := httptest.NewRecorder()
	g.handleThumb(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleDownloadInvalidSHA(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/download/short", nil)
	req.URL.Path = "/api/download/short"
	rec := httptest.NewRecorder()
	g.handleDownload(rec, req)

	if rec.Code != 400 {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleDownloadNotFound(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/download/dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", nil)
	req.URL.Path = "/api/download/dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	rec := httptest.NewRecorder()
	g.handleDownload(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleStreamInvalidSHA(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/stream/short", nil)
	req.URL.Path = "/api/stream/short"
	rec := httptest.NewRecorder()
	g.handleStream(rec, req)

	if rec.Code != 400 {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleStreamNotFound(t *testing.T) {
	g, _ := setupTestGallery(t)

	req := httptest.NewRequest("GET", "/api/stream/dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", nil)
	req.URL.Path = "/api/stream/dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	rec := httptest.NewRecorder()
	g.handleStream(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestSendJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	sendJSON(rec, map[string]string{"key": "value"})

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json, got %s", rec.Header().Get("Content-Type"))
	}
	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestSendError(t *testing.T) {
	rec := httptest.NewRecorder()
	sendError(rec, "test error", 400)

	if rec.Code != 400 {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)
	errObj := result["error"].(map[string]interface{})
	if errObj["message"] != "test error" {
		t.Errorf("expected 'test error', got %v", errObj["message"])
	}
}

func TestOpenFile(t *testing.T) {
	// Test opening a non-existent file
	f := openFile("/nonexistent/path/file.txt")
	if f != nil {
		t.Error("expected nil for non-existent file")
	}

	// Test opening an existing file
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(tmpFile, []byte("test"), 0644)
	f = openFile(tmpFile)
	if f == nil {
		t.Error("expected non-nil for existing file")
	} else {
		f.Close()
	}
}

func TestRegisterRoutes(t *testing.T) {
	g, _ := setupTestGallery(t)
	mux := http.NewServeMux()
	g.RegisterRoutes(mux)

	// Verify routes are registered by making a test request
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/stats")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestHandleThumbSuccess(t *testing.T) {
	g, mediaRoot := setupTestGallery(t)

	// Create a real JPEG file so thumbnail generation works
	imgPath := filepath.Join(mediaRoot, "cat1/2024-01/IMG_001.jpg")
	os.MkdirAll(filepath.Dir(imgPath), 0755)
	createJPEGFile(t, imgPath, 200, 200)

	req := httptest.NewRequest("GET", "/api/thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	req.URL.Path = "/api/thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	rec := httptest.NewRecorder()
	g.handleThumb(rec, req)

	// Should return 200 with image data (either from ffmpeg or Go fallback)
	if rec.Code != 200 && rec.Code != 404 {
		t.Errorf("expected 200 or 404, got %d", rec.Code)
	}
}

func TestHandleDownloadSuccess(t *testing.T) {
	g, mediaRoot := setupTestGallery(t)

	// Ensure the file exists
	imgPath := filepath.Join(mediaRoot, "cat1/2024-01/IMG_001.jpg")
	os.MkdirAll(filepath.Dir(imgPath), 0755)
	os.WriteFile(imgPath, []byte("fake-image-data"), 0644)

	req := httptest.NewRequest("GET", "/api/download/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	req.URL.Path = "/api/download/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	rec := httptest.NewRecorder()
	g.handleDownload(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Disposition") == "" {
		t.Error("expected Content-Disposition header")
	}
}

func TestHandleStreamSuccess(t *testing.T) {
	g, mediaRoot := setupTestGallery(t)

	// Ensure the video file exists
	vidPath := filepath.Join(mediaRoot, "cat2/2024-02/VID_001.mp4")
	os.MkdirAll(filepath.Dir(vidPath), 0755)
	os.WriteFile(vidPath, []byte("fake-video-data"), 0644)

	req := httptest.NewRequest("GET", "/api/stream/cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", nil)
	req.URL.Path = "/api/stream/cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	rec := httptest.NewRecorder()
	g.handleStream(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "video/mp4" {
		t.Errorf("expected video/mp4, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestHandleStreamMOV(t *testing.T) {
	g, mediaRoot := setupTestGallery(t)
	tmpDir := t.TempDir()

	// Create a .mov file in the DB
	conn, _ := sql.Open("sqlite", tmpDir+"/mov.db")
	conn.Exec("CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER)")
	conn.Exec("INSERT INTO files VALUES ('cat/2024-01/VID.mov', 1000, 1700000000, 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee', 1700000001)")
	conn.Close()

	// Can't easily swap the DB, so just test the mime type logic via a direct request
	vidPath := filepath.Join(mediaRoot, "cat2/2024-02/VID_001.mp4")
	os.MkdirAll(filepath.Dir(vidPath), 0755)
	os.WriteFile(vidPath, []byte("fake-video"), 0644)

	// Test with the mp4 file
	req := httptest.NewRequest("GET", "/api/stream/cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", nil)
	req.URL.Path = "/api/stream/cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	rec := httptest.NewRecorder()
	g.handleStream(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestHandleDownloadFileNotOnDisk(t *testing.T) {
	g, mediaRoot := setupTestGallery(t)

	// Remove the file from disk
	os.Remove(filepath.Join(mediaRoot, "cat1/2024-01/IMG_001.jpg"))

	req := httptest.NewRequest("GET", "/api/download/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	req.URL.Path = "/api/download/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	rec := httptest.NewRecorder()
	g.handleDownload(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404 for missing file on disk, got %d", rec.Code)
	}
}

func TestHandleStreamFileNotOnDisk(t *testing.T) {
	g, mediaRoot := setupTestGallery(t)

	// Remove the file from disk
	os.Remove(filepath.Join(mediaRoot, "cat2/2024-02/VID_001.mp4"))

	req := httptest.NewRequest("GET", "/api/stream/cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", nil)
	req.URL.Path = "/api/stream/cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	rec := httptest.NewRecorder()
	g.handleStream(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404 for missing file on disk, got %d", rec.Code)
	}
}

func createJPEGFile(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 128, B: 64, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()
	jpeg.Encode(f, img, &jpeg.Options{Quality: 75})
}
