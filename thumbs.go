package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

const (
	ThumbSize    = 150
	ThumbQuality = 60
	VideoThumbW  = 320
	VideoThumbH  = 180
	MaxThumbWorkers = 4 // limit concurrent thumbnail generation to avoid OOM
)

type ThumbCache struct {
	dir    string
	mu     sync.Mutex
	busy   map[string]bool
	sem    chan struct{}
}

func NewThumbCache(dir string) *ThumbCache {
	os.MkdirAll(dir, 0755)
	return &ThumbCache{
		dir:  dir,
		busy: make(map[string]bool),
		sem:  make(chan struct{}, MaxThumbWorkers),
	}
}

func (tc *ThumbCache) Path(sha string) string {
	if len(sha) < 2 {
		return filepath.Join(tc.dir, "unknown.jpg")
	}
	subdir := sha[:2]
	os.MkdirAll(filepath.Join(tc.dir, subdir), 0755)
	return filepath.Join(tc.dir, subdir, sha+".jpg")
}

func (tc *ThumbCache) Exists(sha string) bool {
	if sha == "" {
		return false
	}
	_, err := os.Stat(tc.Path(sha))
	return err == nil
}

func (tc *ThumbCache) Get(sha, fullPath string) ([]byte, error) {
	if sha == "" {
		return nil, fmt.Errorf("empty sha")
	}

	p := tc.Path(sha)

	// cache hit
	if data, err := os.ReadFile(p); err == nil {
		return data, nil
	}

	// dedup concurrent requests for same sha
	tc.mu.Lock()
	if tc.busy[sha] {
		tc.mu.Unlock()
		// wait for other goroutine, then read cache
		for i := 0; i < 100; i++ {
			if data, err := os.ReadFile(p); err == nil {
				return data, nil
			}
			busyWait(50)
		}
		return nil, fmt.Errorf("thumbnail timeout")
	}
	tc.busy[sha] = true
	tc.mu.Unlock()

	defer func() {
		tc.mu.Lock()
		delete(tc.busy, sha)
		tc.mu.Unlock()
	}()

	// limit concurrent generation to avoid OOM
	tc.sem <- struct{}{}
	defer func() { <-tc.sem }()

	// generate
	ext := strings.ToLower(filepath.Ext(fullPath))
	var data []byte
	var err error

	switch ext {
	case ".jpg", ".jpeg":
		data, err = tc.makeImageThumb(fullPath, true)
	case ".png":
		data, err = tc.makeImageThumb(fullPath, false)
	case ".bmp", ".tif", ".tiff":
		data, err = tc.makeImageThumb(fullPath, true)
	case ".mp4", ".mov", ".avi", ".mkv", ".wmv":
		data, err = tc.makeVideoThumb(fullPath, sha)
	case ".gif":
		data, err = tc.makeImageThumb(fullPath, true)
	default:
		return nil, fmt.Errorf("unsupported type: %s", ext)
	}

	if err != nil {
		return nil, err
	}

	// write cache
	os.WriteFile(p, data, 0644)
	return data, nil
}

func (tc *ThumbCache) makeImageThumb(fullPath string, isJPEG bool) ([]byte, error) {
	// Use ffmpeg for all image types — it's memory-efficient (streams, doesn't load full image into Go heap)
	// Unique temp file per call to avoid collisions between concurrent ffmpeg processes
	rnd := make([]byte, 4)
	rand.Read(rnd)
	tmpFile := filepath.Join(os.TempDir(), "thumb_"+hex.EncodeToString(rnd)+".jpg")
	defer os.Remove(tmpFile)

	cmd := exec.Command("ffmpeg",
		"-i", fullPath,
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", ThumbSize, ThumbSize, ThumbSize, ThumbSize),
		"-frames:v", "1",
		"-q:v", fmt.Sprintf("%d", ThumbQuality),
		"-y",
		tmpFile)
	cmd.Stderr = io.Discard
	cmd.Stdout = io.Discard

	if err := cmd.Run(); err != nil {
		// fallback to Go image decoding for non-video formats
		return tc.makeImageThumbGo(fullPath, isJPEG)
	}
	return os.ReadFile(tmpFile)
}

func (tc *ThumbCache) makeImageThumbGo(fullPath string, isJPEG bool) ([]byte, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var img image.Image
	// decode with pixel limit to avoid OOM on huge images
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	f.Seek(0, 0)

	// if image is very large, skip Go decode and return placeholder
	if cfg.Width > 8000 || cfg.Height > 8000 {
		return nil, fmt.Errorf("image too large for Go decode: %dx%d", cfg.Width, cfg.Height)
	}

	if isJPEG {
		img, err = jpeg.Decode(f)
	} else {
		img, err = png.Decode(f)
	}
	if err != nil {
		f.Seek(0, 0)
		img, _, err = image.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decode image: %w", err)
		}
	}

	// use fast approximation scaler instead of CatmullRom (much less memory)
	thumb := resizeImageFast(img, ThumbSize, ThumbSize)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: ThumbQuality}); err != nil {
		return nil, fmt.Errorf("encode thumb: %w", err)
	}
	return buf.Bytes(), nil
}

func (tc *ThumbCache) makeVideoThumb(fullPath, sha string) ([]byte, error) {
	tmpFile := filepath.Join(os.TempDir(), "vthumb_"+sha+".jpg")
	defer os.Remove(tmpFile)

	cmd := exec.Command("ffmpeg",
		"-ss", "1",
		"-i", fullPath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:%d", VideoThumbW, VideoThumbH),
		"-q:v", "5",
		"-y",
		tmpFile)
	cmd.Stderr = io.Discard
	cmd.Stdout = io.Discard

	if err := cmd.Run(); err != nil {
		cmd = exec.Command("ffmpeg",
			"-ss", "0",
			"-i", fullPath,
			"-frames:v", "1",
			"-vf", fmt.Sprintf("scale=%d:%d", VideoThumbW, VideoThumbH),
			"-q:v", "5",
			"-y",
			tmpFile)
		cmd.Stderr = io.Discard
		cmd.Stdout = io.Discard
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("ffmpeg: %w", err)
		}
	}
	return os.ReadFile(tmpFile)
}

func resizeImageFast(src image.Image, maxW, maxH int) image.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w > h {
		h = h * maxW / w
		w = maxW
	} else {
		w = w * maxH / h
		h = maxH
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// Nearest-Neighbor is fastest and uses least memory
	// thumbnails are small so quality is acceptable
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

func busyWait(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
