package main

import "testing"

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
