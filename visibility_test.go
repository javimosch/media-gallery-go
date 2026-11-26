package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestHiddenWhenNoHealthyReplica(t *testing.T) {
	defer hiddenPaths.Store(nil)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "i.db")
	c, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER)",
		"INSERT INTO files VALUES ('here/2026-01/a.mp4',1,0,'aa',0), ('gone/2025-01/b.mp4',1,0,'bb',0), ('here/2026-01/c.mp4',1,0,'cc',0)",
	} {
		if _, err := c.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	c.Close()
	os.MkdirAll(filepath.Join(dir, "here/2026-01"), 0o755)
	os.WriteFile(filepath.Join(dir, "here/2026-01/a.mp4"), []byte("x"), 0o644)

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	g := NewGalleryHandler(db, nil, dir, nil)
	g.sources.Refresh(db, dir)

	check := func(want int, cats int) {
		t.Helper()
		res, err := db.Browse("", "", "", 1, false, false, nil)
		if err != nil || res.Total != want {
			t.Fatalf("total=%v err=%v want %d", res, err, want)
		}
		page2, _ := db.Browse("", "", res.NextCursor, 1, false, false, nil)
		if want > 1 && page2.Total != want {
			t.Fatalf("page 2 total=%d want %d", page2.Total, want)
		}
		got, _ := db.Categories()
		st, _ := db.GetStats()
		if len(got) != cats || st.TotalFiles != want {
			t.Fatalf("cats=%v stats=%d", got, st.TotalFiles)
		}
	}

	if n := g.syncVisibility(); n != 2 { // b (absent dir) and c (absent file)
		t.Fatalf("hidden=%d", n)
	}
	check(1, 1)
	g.replicas.Register("http://127.0.0.1:1", "replica-a", 3)
	g.syncVisibility()
	check(3, 2)
	g.replicas.Deregister("replica-a")
	g.syncVisibility()
	check(1, 1)

	// category is a bound parameter, not spliced into SQL
	res, _ := db.Browse("x' OR 1=1 --", "", "", 10, false, false, nil)
	if res.Total != 0 {
		t.Fatalf("injection total=%d", res.Total)
	}
}
