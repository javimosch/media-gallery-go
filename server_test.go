package main

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
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
