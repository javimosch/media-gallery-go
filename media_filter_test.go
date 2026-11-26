package main

import "testing"

func TestMediaFilterCondition(t *testing.T) {
	if c := (MediaFilter{}).condition(); c != "" {
		t.Fatalf("default must not filter, got %q", c)
	}
	if c := (MediaFilter{HidePhotos: true, HideVideos: true}).condition(); c != "1=0" {
		t.Fatalf("both hidden must match nothing, got %q", c)
	}
}

// "Show junk" must list exactly what the default view hides.
func TestJunkOnlyIsComplement(t *testing.T) {
	g, _ := setupTestGallery(t)
	db := g.db
	total := func(hide, only bool) int {
		r, err := db.Browse("", "", "", 10, hide, false, nil, MediaFilter{JunkOnly: only})
		if err != nil {
			t.Fatal(err)
		}
		return r.Total
	}
	if all, kept, junk := total(false, false), total(true, false), total(false, true); kept+junk != all {
		t.Fatalf("kept %d + junk %d != all %d", kept, junk, all)
	}
}
