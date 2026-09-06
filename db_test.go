package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenDB(t *testing.T) {
	// Create a minimal test database
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	// Create schema and insert test data
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER);
		INSERT INTO files VALUES
			('family-media-sorted/2024-01/IMG_001.jpg', 1048576, 1700000000, 'aaa111', 1700000001),
			('family-media-sorted/2024-01/IMG_002.jpg', 2097152, 1700000001, 'bbb222', 1700000002),
			('family-media-sorted/2024-02/IMG_003.jpg', 3145728, 1700000002, 'aaa111', 1700000003),
			('drone-media-sorted/2024-01/DJI_001.jpg', 524288, 1700000003, 'ccc333', 1700000004),
			('videos-unsorted/2024-01/VID_001.mp4', 10485760, 1700000004, 'ddd444', 1700000005);
	`)
	conn.Close()
	if err != nil {
		t.Fatalf("seed test db: %v", err)
	}

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	t.Run("Browse all", func(t *testing.T) {
		r, err := db.Browse("", "", "", 10, false, nil)
		if err != nil {
			t.Fatalf("Browse: %v", err)
		}
		if len(r.Files) != 5 {
			t.Errorf("expected 5 files, got %d", len(r.Files))
		}
		if r.Total != 5 {
			t.Errorf("expected total 5, got %d", r.Total)
		}
	})

	t.Run("Browse with category", func(t *testing.T) {
		r, err := db.Browse("family-media-sorted", "", "", 10, false, nil)
		if err != nil {
			t.Fatalf("Browse: %v", err)
		}
		if len(r.Files) != 3 {
			t.Errorf("expected 3 files in family-media-sorted, got %d", len(r.Files))
		}
	})

	t.Run("Browse with yearMonth", func(t *testing.T) {
		r, err := db.Browse("", "2024-01", "", 10, false, nil)
		if err != nil {
			t.Fatalf("Browse: %v", err)
		}
		// 4 files have 2024-01 in path across all categories
		if len(r.Files) != 4 {
			t.Errorf("expected 4 files in 2024-01, got %d", len(r.Files))
		}
	})

	t.Run("Browse with category + yearMonth", func(t *testing.T) {
		r, err := db.Browse("family-media-sorted", "2024-01", "", 10, false, nil)
		if err != nil {
			t.Fatalf("Browse: %v", err)
		}
		if len(r.Files) != 2 {
			t.Errorf("expected 2 files, got %d", len(r.Files))
		}
	})

	t.Run("Browse pagination", func(t *testing.T) {
		r, err := db.Browse("", "", "", 2, false, nil)
		if err != nil {
			t.Fatalf("Browse: %v", err)
		}
		if len(r.Files) != 2 {
			t.Errorf("expected 2 files (limit), got %d", len(r.Files))
		}
		if r.NextCursor == "" {
			t.Error("expected next_cursor for pagination")
		}
	})

	t.Run("Search", func(t *testing.T) {
		files, err := db.Search("IMG_", 10)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(files) != 3 {
			t.Errorf("expected 3 IMG_ files, got %d", len(files))
		}
	})

	t.Run("Search no results", func(t *testing.T) {
		files, err := db.Search("NONEXISTENT", 10)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(files) != 0 {
			t.Errorf("expected 0 results, got %d", len(files))
		}
	})

	t.Run("GetBySHA", func(t *testing.T) {
		f, err := db.GetBySHA("aaa111")
		if err != nil {
			t.Fatalf("GetBySHA: %v", err)
		}
		if f.SHA256 != "aaa111" {
			t.Errorf("expected sha256 aaa111, got %s", f.SHA256)
		}
	})

	t.Run("GetBySHA not found", func(t *testing.T) {
		_, err := db.GetBySHA("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent SHA")
		}
	})

	t.Run("GetByRelPath", func(t *testing.T) {
		f, err := db.GetByRelPath("drone-media-sorted/2024-01/DJI_001.jpg")
		if err != nil {
			t.Fatalf("GetByRelPath: %v", err)
		}
		if f.RelPath != "drone-media-sorted/2024-01/DJI_001.jpg" {
			t.Errorf("wrong relpath: %s", f.RelPath)
		}
	})

	t.Run("Categories", func(t *testing.T) {
		cats, err := db.Categories()
		if err != nil {
			t.Fatalf("Categories: %v", err)
		}
		if len(cats) != 3 {
			t.Errorf("expected 3 categories, got %d: %v", len(cats), cats)
		}
		for _, c := range cats {
			if c == "" {
				t.Error("empty category should be filtered out")
			}
		}
	})

	t.Run("YearMonths all", func(t *testing.T) {
		yms, err := db.YearMonths("")
		if err != nil {
			t.Fatalf("YearMonths: %v", err)
		}
		if len(yms) != 2 {
			t.Errorf("expected 2 year-months, got %d: %v", len(yms), yms)
		}
		// verify no garbage entries
		for _, ym := range yms {
			if len(ym) != 7 || ym[:4] != "2024" {
				t.Errorf("invalid year-month: %s", ym)
			}
		}
	})

	t.Run("YearMonths by category", func(t *testing.T) {
		yms, err := db.YearMonths("family-media-sorted")
		if err != nil {
			t.Fatalf("YearMonths: %v", err)
		}
		if len(yms) != 2 {
			t.Errorf("expected 2 year-months, got %d", len(yms))
		}
	})

	t.Run("DupeGroups", func(t *testing.T) {
		groups, err := db.DupeGroups(10, 0)
		if err != nil {
			t.Fatalf("DupeGroups: %v", err)
		}
		if len(groups) != 1 {
			t.Errorf("expected 1 dupe group (aaa111), got %d", len(groups))
		}
		if groups[0].Count != 2 {
			t.Errorf("expected 2 copies, got %d", groups[0].Count)
		}
	})

	t.Run("DupeFiles", func(t *testing.T) {
		files, err := db.DupeFiles("aaa111")
		if err != nil {
			t.Fatalf("DupeFiles: %v", err)
		}
		if len(files) != 2 {
			t.Errorf("expected 2 files, got %d", len(files))
		}
	})

	t.Run("GetStats", func(t *testing.T) {
		s, err := db.GetStats()
		if err != nil {
			t.Fatalf("GetStats: %v", err)
		}
		if s.TotalFiles != 5 {
			t.Errorf("expected 5 total files, got %d", s.TotalFiles)
		}
		if s.DupeGroups != 1 {
			t.Errorf("expected 1 dupe group, got %d", s.DupeGroups)
		}
		if len(s.Categories) != 3 {
			t.Errorf("expected 3 categories, got %d", len(s.Categories))
		}
	})

	t.Run("UniqueSHA256Count", func(t *testing.T) {
		count, err := db.UniqueSHA256Count()
		if err != nil {
			t.Fatalf("UniqueSHA256Count: %v", err)
		}
		if count != 4 {
			t.Errorf("expected 4 unique SHAs, got %d", count)
		}
	})

	t.Run("ScanProgress", func(t *testing.T) {
		done, total := db.ScanProgress()
		if total != 5 {
			t.Errorf("expected total 5, got %d", total)
		}
		if done != 5 {
			t.Errorf("expected done 5, got %d", done)
		}
	})

	t.Run("Close", func(t *testing.T) {
		// Test that close works without panic
		tmpDir := t.TempDir()
		dbPath := tmpDir + "/close.db"
		conn, _ := sql.Open("sqlite", dbPath)
		conn.Exec("CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER)")
		conn.Close()
		db2, _ := OpenDB(dbPath)
		if err := db2.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}

func TestTs(t *testing.T) {
	result := ts()
	if result == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestBrowseLimitClamping(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	conn, _ := sql.Open("sqlite", dbPath)
	conn.Exec("CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER)")
	conn.Close()

	db, _ := OpenDB(dbPath)
	defer db.Close()

	// limit 0 should default to 50
	r, err := db.Browse("", "", "", 0, false, nil)
	if err != nil {
		t.Fatalf("Browse with limit 0: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil result")
	}
}
