package main

import (
	"database/sql"
	"encoding/json"
	"math"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFaceDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/faces.db"

	// Create faces schema
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("create faces db: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE faces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sha256 TEXT NOT NULL,
			relpath TEXT NOT NULL,
			face_idx INTEGER NOT NULL,
			bbox TEXT NOT NULL,
			embedding BLOB NOT NULL,
			gender INTEGER,
			age INTEGER,
			thumb_path TEXT,
			scanned_at REAL NOT NULL,
			UNIQUE(sha256, face_idx)
		);
		CREATE TABLE scan_progress (
			sha256 TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			face_count INTEGER DEFAULT 0,
			scanned_at REAL
		);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	// Insert test face data with embeddings
	emb1 := makeEmbedding([]float32{1.0, 0.0, 0.0, 0.0})
	emb2 := makeEmbedding([]float32{0.95, 0.05, 0.0, 0.0}) // similar to emb1
	emb3 := makeEmbedding([]float32{0.0, 1.0, 0.0, 0.0})   // different

	bbox, _ := json.Marshal([4]int{10, 20, 30, 40})

	_, err = conn.Exec(`INSERT INTO faces (sha256, relpath, face_idx, bbox, embedding, gender, age, thumb_path, scanned_at) VALUES
		('sha001', 'cat1/2024-01/IMG_001.jpg', 0, ?, ?, 1, 30, '', 1700000000),
		('sha002', 'cat1/2024-01/IMG_002.jpg', 0, ?, ?, 0, 25, '', 1700000001),
		('sha003', 'cat2/2024-02/IMG_003.jpg', 0, ?, ?, 1, 40, '', 1700000002)`,
		string(bbox), emb1, string(bbox), emb2, string(bbox), emb3)
	if err != nil {
		t.Fatalf("insert faces: %v", err)
	}

	// Insert scan progress
	_, err = conn.Exec(`INSERT INTO scan_progress VALUES
		('sha001', 'done', 1, 1700000000),
		('sha002', 'done', 1, 1700000001),
		('sha003', 'done', 1, 1700000002)`)
	if err != nil {
		t.Fatalf("insert progress: %v", err)
	}
	conn.Close()

	fdb, err := OpenFaceDB(dbPath)
	if err != nil {
		t.Fatalf("OpenFaceDB: %v", err)
	}
	defer fdb.Close()

	t.Run("Status", func(t *testing.T) {
		s, err := fdb.Status()
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if s.Scanned != 3 {
			t.Errorf("expected 3 scanned, got %d", s.Scanned)
		}
		if s.FacesFound != 3 {
			t.Errorf("expected 3 faces, got %d", s.FacesFound)
		}
	})

	t.Run("RecentFaces", func(t *testing.T) {
		faces, err := fdb.RecentFaces(10, 0)
		if err != nil {
			t.Fatalf("RecentFaces: %v", err)
		}
		if len(faces) != 3 {
			t.Errorf("expected 3 faces, got %d", len(faces))
		}
	})

	t.Run("FaceByID", func(t *testing.T) {
		f, err := fdb.FaceByID(1)
		if err != nil {
			t.Fatalf("FaceByID: %v", err)
		}
		if f.SHA256 != "sha001" {
			t.Errorf("expected sha001, got %s", f.SHA256)
		}
		if f.BBox != [4]int{10, 20, 30, 40} {
			t.Errorf("wrong bbox: %v", f.BBox)
		}
	})

	t.Run("Embedding", func(t *testing.T) {
		emb, err := fdb.Embedding(1)
		if err != nil {
			t.Fatalf("Embedding: %v", err)
		}
		if len(emb) != 4 {
			t.Errorf("expected 4D embedding, got %d", len(emb))
		}
		if emb[0] != 1.0 {
			t.Errorf("expected first element 1.0, got %f", emb[0])
		}
	})

	t.Run("SearchSimilar", func(t *testing.T) {
		// Face 1 (emb1) and face 2 (emb2) are similar
		// Face 3 (emb3) is different
		results, err := fdb.SearchSimilar(1, 0.5, 10)
		if err != nil {
			t.Fatalf("SearchSimilar: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 similar face, got %d", len(results))
		}
		if results[0].ID != 2 {
			t.Errorf("expected face #2, got #%d", results[0].ID)
		}
		if results[0].Similarity < 95.0 {
			t.Errorf("expected high similarity, got %.1f%%", results[0].Similarity)
		}
	})

	t.Run("SearchSimilar low threshold", func(t *testing.T) {
		results, err := fdb.SearchSimilar(1, 0.0, 10)
		if err != nil {
			t.Fatalf("SearchSimilar: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results at threshold 0, got %d", len(results))
		}
	})
}

func TestOpenFaceDBNotFound(t *testing.T) {
	_, err := OpenFaceDB("/nonexistent/path/faces.db")
	if err == nil {
		t.Error("expected error for non-existent faces db")
	}
}

func TestCosineSim(t *testing.T) {
	tests := []struct {
		a, b   []float32
		expect float64
	}{
		{[]float32{1, 0}, []float32{1, 0}, 1.0},
		{[]float32{1, 0}, []float32{0, 1}, 0.0},
		{[]float32{1, 0}, []float32{-1, 0}, -1.0},
		{[]float32{0, 0}, []float32{1, 0}, 0.0},
	}
	for _, tt := range tests {
		got := cosineSim(tt.a, tt.b)
		if math.Abs(got-tt.expect) > 0.001 {
			t.Errorf("cosineSim(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.expect)
		}
	}
}

func TestParseIntDefault(t *testing.T) {
	if parseIntDefault("", 42) != 42 {
		t.Error("empty string should return default")
	}
	if parseIntDefault("abc", 42) != 42 {
		t.Error("invalid string should return default")
	}
	if parseIntDefault("10", 42) != 10 {
		t.Error("valid string should parse")
	}
}

func TestParseFloatDefault(t *testing.T) {
	if parseFloatDefault("", 0.5) != 0.5 {
		t.Error("empty string should return default")
	}
	if parseFloatDefault("0.75", 0.5) != 0.75 {
		t.Error("valid string should parse")
	}
}

// makeEmbedding packs float32 slice into bytes (same encoding as face_scan.py)
func makeEmbedding(vals []float32) []byte {
	buf := make([]byte, len(vals)*4)
	for i, v := range vals {
		bits := math.Float32bits(v)
		buf[i*4] = byte(bits)
		buf[i*4+1] = byte(bits >> 8)
		buf[i*4+2] = byte(bits >> 16)
		buf[i*4+3] = byte(bits >> 24)
	}
	return buf
}
