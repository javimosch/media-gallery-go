package main

import (
	"database/sql"
	"strings"
	"testing"
)

func TestAssignments(t *testing.T) {
	t.Cleanup(func() { configureGroups("") })
	path := t.TempDir() + "/a.db"
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(`
		CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER);
		CREATE TABLE assignments (sha256 TEXT PRIMARY KEY, folder TEXT NOT NULL, month TEXT NOT NULL, rule TEXT, batch TEXT, assigned_at INTEGER);
		INSERT INTO files VALUES
			('photos/2020-01/a.jpg', 2000000, 0, 's1', 0),
			('photos/2020-01/b.jpg', 2000000, 0, 's2', 0),
			('other/2020-02/a_copy.jpg', 2000000, 0, 's1', 0),
			('photos/2020-03/tiny.jpg', 10, 0, 's3', 0),
			('cam-new/2019-12/n.jpg', 2000000, 0, 's4', 0);
		INSERT INTO assignments VALUES ('s1','cam-old','2021-05','r','p',0), ('s3','cam-old','2018-01','r','p',0);`)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := configureGroups("Cam=cam-*;Rest=*"); err != nil {
		t.Fatal(err)
	}
	list := func(category, ym, cursor string, limit int, hideJunk bool, m MediaFilter) (string, *BrowseResult) {
		t.Helper()
		r, err := db.Browse(category, ym, cursor, limit, hideJunk, false, nil, m)
		if err != nil {
			t.Fatal(err)
		}
		var p []string
		for _, f := range r.Files {
			p = append(p, f.RelPath)
		}
		return strings.Join(p, " "), r
	}
	want := func(label, got, exp string) {
		t.Helper()
		if got != exp {
			t.Errorf("%s:\n got %q\nwant %q", label, got, exp)
		}
	}
	// one copy only, ordered by virtual path (2018-01 tiny, then 2021-05), real paths returned
	got, r := list("cam-old", "", "", 10, false, MediaFilter{})
	want("folder", got, "other/2020-02/a_copy.jpg photos/2020-03/tiny.jpg")
	if r.Total != 2 {
		t.Error("total", r.Total)
	}
	got, r = list("", "", "", 2, false, MediaFilter{Group: "Cam"})
	want("page 1", got, "other/2020-02/a_copy.jpg cam-new/2019-12/n.jpg")
	got, _ = list("", "", r.NextCursor, 2, false, MediaFilter{Group: "Cam"})
	want("page 2", got, "photos/2020-03/tiny.jpg")
	got, _ = list("", "", "", 10, false, MediaFilter{Group: "Rest"})
	want("assigned files leave their old group", got, "photos/2020-01/b.jpg")
	got, _ = list("", "2021-05", "", 10, false, MediaFilter{})
	want("assigned month", got, "other/2020-02/a_copy.jpg")
	got, _ = list("", "", "", 10, false, MediaFilter{Name: "a", Group: "Cam"})
	want("name search lists every copy", got, "photos/2020-01/a.jpg other/2020-02/a_copy.jpg")
	got, _ = list("", "", "", 10, true, MediaFilter{AssignedOnly: true})
	want("assigned only includes junk", got, "other/2020-02/a_copy.jpg photos/2020-03/tiny.jpg")

	counts, err := db.FolderCounts()
	if err != nil {
		t.Fatal(err)
	}
	g := groupFolders(counts)
	if len(g) != 2 || g[0].Count != 3 || g[1].Count != 1 {
		t.Errorf("counts %+v", g)
	}
	yms, _ := db.YearMonths("cam-old")
	want("months", strings.Join(yms, " "), "2021-05 2018-01")
	cats, _ := db.Categories()
	want("categories", strings.Join(cats, " "), "cam-new cam-old photos")
	if !db.IsAssigned("s1") || db.IsAssigned("s2") {
		t.Error("IsAssigned")
	}
}
