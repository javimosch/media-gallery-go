package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProfileFor(t *testing.T) {
	t.Cleanup(func() { configureHDThumbs("", "") })
	check := func(path string, want thumbProfile) {
		t.Helper()
		if got := profileFor(path); got != want {
			t.Errorf("%s: got %+v", path, got)
		}
	}
	check("/m/camera-a/2030-06/a.jpg", legacyThumb) // off by default

	if err := configureHDThumbs("2030-06", "camera-a, camera-b"); err != nil {
		t.Fatal(err)
	}
	check("/m/photos/2030-06/a.jpg", hdThumb)
	check("/m/photos/2031-01/a.mp4", hdThumb)
	check("/m/photos/2030-05/a.jpg", legacyThumb)
	check("/m/camera-b/2020-01/a.jpg", hdThumb)
	check("/m/unsorted/a.mp4", legacyThumb)

	if configureHDThumbs("june", "") == nil {
		t.Error("expected an error for a bad month")
	}
}

func TestThumbSourcePrefersHDCopy(t *testing.T) {
	t.Cleanup(func() { configureHDThumbs("", "") })
	g, _ := setupTestGallery(t)
	files, err := g.db.Browse("", "", "", 1, false, false, nil)
	if err != nil || len(files.Files) == 0 {
		t.Fatal("no files", err)
	}
	f := files.Files[0]
	full := filepath.Join(g.mediaRoot, f.RelPath)
	os.MkdirAll(filepath.Dir(full), 0755)
	os.WriteFile(full, []byte("x"), 0644)
	// the only HD copy is f; the fallback is some other, non-HD path
	configureHDThumbs("", filepath.Base(filepath.Dir(filepath.Dir(full))))
	if profileFor(full) != hdThumb {
		t.Skip("fixture is not folder/YYYY-MM/file")
	}
	if got := g.thumbSource(f.SHA256, "/elsewhere/a.jpg"); got != full {
		t.Fatalf("got %s want %s", got, full)
	}
}

func TestWantsHDByMediaDate(t *testing.T) {
	t.Cleanup(func() { configureHDThumbs("", "") })
	g, _ := setupTestGallery(t)
	r, err := g.db.Browse("", "2024-02", "", 1, false, false, nil)
	if err != nil || len(r.Files) == 0 {
		t.Fatal("no files", err)
	}
	sha := r.Files[0].SHA256
	month := g.db.GalleryMonth(sha)
	if month < "2024-02" {
		t.Fatalf("gallery month %q", month)
	}
	if g.wantsHD(sha) {
		t.Error("off by default")
	}
	configureHDThumbs(month, "")
	if !g.wantsHD(sha) {
		t.Error("same month")
	}
	configureHDThumbs("2031-01", "")
	if g.wantsHD(sha) || g.wantsHD("unknown") {
		t.Error("older or unknown content")
	}
}

func TestHDAll(t *testing.T) {
	t.Cleanup(func() { configureHDThumbs("", "") })
	g, _ := setupTestGallery(t)
	if err := configureHDThumbs("all", ""); err != nil {
		t.Fatal(err)
	}
	if !g.wantsHD("unknown") {
		t.Error("all must cover undated content")
	}
}

func TestShortVideoThumb(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "short.mp4")
	if out, err := exec.Command("ffmpeg", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=25", "-t", "0.5", "-pix_fmt", "yuv420p", clip).CombinedOutput(); err != nil {
		t.Skipf("cannot make a test clip: %v %s", err, out)
	}
	data, err := NewThumbCache(filepath.Join(dir, "t")).makeVideoThumb(clip, "short-clip-test", hdThumb)
	if err != nil || len(data) == 0 {
		t.Fatalf("no thumbnail for a 0.5s clip: %v", err)
	}
}
