package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestResolvePath(t *testing.T) {
	t.Run("valid path", func(t *testing.T) {
		full, ok := resolvePath("/media", "cat/2024-01/IMG_001.jpg")
		if !ok {
			t.Error("expected ok")
		}
		expected := filepath.Join("/media", "cat/2024-01/IMG_001.jpg")
		if full != expected {
			t.Errorf("expected %s, got %s", expected, full)
		}
	})

	t.Run("directory traversal contained", func(t *testing.T) {
		// resolvePath cleans the path, so ../../../etc/passwd becomes /etc/passwd
		// which when joined with /media becomes /media/etc/passwd — contained under media root
		full, ok := resolvePath("/media", "../../../etc/passwd")
		if !ok {
			t.Error("expected ok (traversal is contained by Clean+Join)")
		}
		// verify result is still under media root
		if !strings.HasPrefix(full, "/media") {
			t.Errorf("path escaped media root: %s", full)
		}
	})

	t.Run("path with .. contained", func(t *testing.T) {
		full, ok := resolvePath("/media", "cat/../../etc/passwd")
		if !ok {
			t.Error("expected ok (traversal is contained)")
		}
		if !strings.HasPrefix(full, "/media") {
			t.Errorf("path escaped media root: %s", full)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		full, ok := resolvePath("/media", "")
		if !ok {
			t.Error("expected ok for empty path")
		}
		if full != "/media" {
			t.Errorf("expected /media, got %s", full)
		}
	})
}

func TestHandleHealthAPI(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/health", nil)
	rec := httptest.NewRecorder()
	handleHealthAPI(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() == "" {
		t.Error("expected non-empty body")
	}
}

func TestHandleStatusAPI(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/status", nil)
	rec := httptest.NewRecorder()
	handleStatusAPI(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestStartServer(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	mediaRoot := tmpDir + "/media"
	os.MkdirAll(mediaRoot, 0755)

	// Create test DB
	conn, _ := sql.Open("sqlite", dbPath)
	conn.Exec("CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER)")
	conn.Exec("INSERT INTO files VALUES ('test/2024-01/IMG.jpg', 1000, 1700000000, 'aaaa', 1700000001)")
	conn.Close()

	cfg := ServerConfig{
		Port:          18090,
		MediaRoot:     mediaRoot,
		DBPath:        dbPath,
		ThumbDir:      tmpDir + "/thumbs",
		MaxConcurrent: 2,
	}

	// Start server in goroutine
	go startServer(cfg)

	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	// Test that it responds
	resp, err := http.Get("http://localhost:18090/api/health")
	if err != nil {
		t.Fatalf("server not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Test status endpoint
	resp2, err := http.Get("http://localhost:18090/api/status")
	if err != nil {
		t.Fatalf("status endpoint failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 for status, got %d", resp2.StatusCode)
	}
}
