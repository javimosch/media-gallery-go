package main

import (
	"strings"
	"testing"
)

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

func TestNameFilter(t *testing.T) {
	g, _ := setupTestGallery(t)
	all, err := g.db.Browse("", "", "", 10, false, false, nil)
	if err != nil || len(all.Files) == 0 {
		t.Fatal("no files", err)
	}
	rel := all.Files[0].RelPath
	folder, base := rel[:strings.Index(rel, "/")], rel[strings.LastIndex(rel, "/")+1:]
	find := func(name string, hideJunk, junkOnly bool) int {
		r, err := g.db.Browse("", "", "", 10, hideJunk, false, nil, MediaFilter{Name: name, JunkOnly: junkOnly})
		if err != nil {
			t.Fatal(err)
		}
		return r.Total
	}
	n := find(strings.ToUpper(base), false, false)
	if n == 0 {
		t.Fatalf("%s not found ignoring case", base)
	}
	if find(base, true, false) != n || find(base, false, true) != n {
		t.Error("junk settings must not change name search results")
	}
	if find(folder, false, false) != 0 {
		t.Error("folder names must not match")
	}
	if find("%", false, false) != 0 {
		t.Error("%% must be literal")
	}
}
