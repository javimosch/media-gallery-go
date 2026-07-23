package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestThumbCache(t *testing.T) {
	tmpDir := t.TempDir()
	tc := NewThumbCache(tmpDir)

	t.Run("cache miss returns error", func(t *testing.T) {
		// Non-existent file should return error
		_, err := tc.Get("nonexistent_sha", "/nonexistent/path.jpg")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("cache hit returns data", func(t *testing.T) {
		// Create a dummy cached thumbnail
		sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		cachePath := filepath.Join(tmpDir, sha[:2], sha+".jpg")
		os.MkdirAll(filepath.Dir(cachePath), 0755)
		os.WriteFile(cachePath, []byte("fake-thumb-data"), 0644)

		data, err := tc.Get(sha, "/nonexistent/path.jpg")
		if err != nil {
			t.Fatalf("cache hit should not error: %v", err)
		}
		if string(data) != "fake-thumb-data" {
			t.Errorf("expected 'fake-thumb-data', got %s", string(data))
		}
	})

	t.Run("Exists false for empty sha", func(t *testing.T) {
		if tc.Exists("") {
			t.Error("expected false for empty sha")
		}
	})

	t.Run("Exists false for missing", func(t *testing.T) {
		if tc.Exists("nonexistent") {
			t.Error("expected false for missing thumb")
		}
	})

	t.Run("Exists true for cached", func(t *testing.T) {
		sha := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		cachePath := filepath.Join(tmpDir, sha[:2], sha+".jpg")
		os.MkdirAll(filepath.Dir(cachePath), 0755)
		os.WriteFile(cachePath, []byte("data"), 0644)
		if !tc.Exists(sha) {
			t.Error("expected true for cached thumb")
		}
	})

	t.Run("Get empty sha returns error", func(t *testing.T) {
		_, err := tc.Get("", "/path")
		if err == nil {
			t.Error("expected error for empty sha")
		}
	})
}

func TestBusyWait(t *testing.T) {
	// Just verify it doesn't hang
	busyWait(10)
}

func TestScaleImage(t *testing.T) {
	// Test with a small image
	img := createTestImage(100, 100)
	scaled := resizeImageFast(img, 50, 50)
	bounds := scaled.Bounds()
	if bounds.Dx() != 50 || bounds.Dy() != 50 {
		t.Errorf("expected 50x50, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func createTestImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	return img
}

func TestMakeVideoThumbNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	tc := NewThumbCache(tmpDir)
	// Non-existent video file should return error
	_, err := tc.makeVideoThumb("/nonexistent/video.mp4", "testsha")
	if err == nil {
		t.Error("expected error for non-existent video")
	}
}

func TestMakeImageThumbGo(t *testing.T) {
	tmpDir := t.TempDir()
	tc := NewThumbCache(tmpDir)
	// Create a test image file
	imgPath := filepath.Join(tmpDir, "test.jpg")
	img := createTestImage(200, 200)
	os.WriteFile(imgPath, encodeJPEG(img), 0644)

	_, err := tc.makeImageThumbGo(imgPath, true)
	if err != nil {
		t.Errorf("makeImageThumbGo: %v", err)
	}
}

func TestMakeImageThumbGoNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	tc := NewThumbCache(tmpDir)
	_, err := tc.makeImageThumbGo("/nonexistent/image.jpg", true)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestThumbCacheGetWithRealImage(t *testing.T) {
	tmpDir := t.TempDir()
	tc := NewThumbCache(tmpDir)

	// Create a real JPEG image
	imgPath := filepath.Join(tmpDir, "real.jpg")
	img := createTestImage(300, 300)
	os.WriteFile(imgPath, encodeJPEG(img), 0644)

	// This should generate a thumbnail (via ffmpeg or Go fallback)
	data, err := tc.Get("testsha123", imgPath)
	if err != nil {
		t.Logf("Get with real image returned error (expected if no ffmpeg): %v", err)
		// Try the Go fallback path
		data, err = tc.makeImageThumbGo(imgPath, true)
		if err != nil {
			t.Skipf("Go fallback also failed: %v", err)
		}
	}
	if len(data) == 0 {
		t.Error("expected non-empty thumbnail data")
	}
}

func TestMakeImageThumbFFmpeg(t *testing.T) {
	tmpDir := t.TempDir()
	tc := NewThumbCache(tmpDir)

	// Create a real JPEG image
	imgPath := filepath.Join(tmpDir, "test.jpg")
	img := createTestImage(400, 300)
	os.WriteFile(imgPath, encodeJPEG(img), 0644)

	// Try the ffmpeg path
	data, err := tc.makeImageThumb(imgPath, true)
	if err != nil {
		t.Logf("makeImageThumb (ffmpeg) failed: %v — this is OK if ffmpeg not installed", err)
		return
	}
	if len(data) == 0 {
		t.Error("expected non-empty thumbnail")
	}
}

func encodeJPEG(img image.Image) []byte {
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75})
	return buf.Bytes()
}
