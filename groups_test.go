package main

import (
	"strings"
	"testing"
)

func TestGroups(t *testing.T) {
	t.Cleanup(func() { configureGroups("") })
	for _, bad := range []string{"A", "=x", "A=", "A=[", "A= , "} {
		if configureGroups(bad) == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
	g, _ := setupTestGallery(t)
	all, err := g.db.Browse("", "", "", 100, false, false, nil)
	if err != nil || all.Total == 0 {
		t.Fatal("no files", err)
	}
	first := all.Files[0].RelPath
	folder := first[:strings.Index(first, "/")]
	if err := configureGroups("A=" + folder + "; B=*"); err != nil {
		t.Fatal(err)
	}
	counts, err := g.db.FolderCounts()
	if err != nil {
		t.Fatal(err)
	}
	groups := groupFolders(counts)
	if len(groups) != 2 || groups[0].Name != "A" || len(groups[0].Folders) != 1 {
		t.Fatalf("first rule must win: %+v", groups)
	}
	for _, gr := range groups {
		r, err := g.db.Browse("", "", "", 100, false, false, nil, MediaFilter{Group: gr.Name})
		if err != nil || r.Total != gr.Count {
			t.Errorf("%s: browse %d, count %d, %v", gr.Name, r.Total, gr.Count, err)
		}
		yms, err := g.db.YearMonths("", gr.Name)
		if err != nil || len(yms) == 0 {
			t.Errorf("%s: no months %v", gr.Name, err)
		}
	}
	r, _ := g.db.Browse("", "", "", 100, false, false, nil, MediaFilter{Group: "x' OR 1=1 --"})
	if r.Total != 0 {
		t.Error("unknown group must match nothing")
	}
}
