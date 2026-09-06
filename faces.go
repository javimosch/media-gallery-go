package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type FaceDB struct {
	conn *sql.DB
}

type FaceRecord struct {
	ID        int     `json:"id"`
	SHA256    string  `json:"sha256"`
	RelPath   string  `json:"relpath"`
	FaceIdx   int     `json:"face_idx"`
	BBox      [4]int  `json:"bbox"`
	Gender    int     `json:"gender,omitempty"`
	Age       int     `json:"age,omitempty"`
	ThumbPath string  `json:"thumb_path,omitempty"`
	Similarity float64 `json:"similarity,omitempty"`
}

type FaceScanStatus struct {
	TotalImages int     `json:"total_images"`
	Scanned     int     `json:"scanned"`
	Pending     int     `json:"pending"`
	FacesFound  int     `json:"faces_found"`
	ProgressPct float64 `json:"progress_pct"`
}

func OpenFaceDB(path string) (*FaceDB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("faces db not found: %s", path)
	}
	conn, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open faces db: %w", err)
	}
	conn.SetMaxOpenConns(2)
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping faces db: %w", err)
	}
	return &FaceDB{conn: conn}, nil
}

func (f *FaceDB) Close() error { return f.conn.Close() }

// faceSHASet returns a comma-separated, quoted list of distinct SHA256 values
// that have at least one detected face. Used for junk filtering in Browse.
// Returns empty string if no faces or error.
func (f *FaceDB) faceSHASet() string {
	rows, err := f.conn.Query("SELECT DISTINCT sha256 FROM faces")
	if err != nil {
		return ""
	}
	defer rows.Close()
	var shas []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		shas = append(shas, "'"+s+"'")
	}
	if len(shas) == 0 {
		return ""
	}
	return strings.Join(shas, ",")
}

func (f *FaceDB) Status() (*FaceScanStatus, error) {
	s := &FaceScanStatus{}
	f.conn.QueryRow("SELECT COUNT(*) FROM scan_progress WHERE status='done'").Scan(&s.Scanned)
	f.conn.QueryRow("SELECT COUNT(*) FROM faces").Scan(&s.FacesFound)
	// total images from faces db scan_progress + pending
	totalRow := f.conn.QueryRow("SELECT COUNT(*) FROM scan_progress")
	totalRow.Scan(&s.TotalImages)
	s.Pending = s.TotalImages - s.Scanned
	if s.TotalImages > 0 {
		s.ProgressPct = math.Round(float64(s.Scanned)/float64(s.TotalImages)*1000) / 10
	}
	return s, nil
}

func (f *FaceDB) RecentFaces(limit, offset int) ([]FaceRecord, error) {
	if limit <= 0 || limit > 200 { limit = 50 }
	rows, err := f.conn.Query(
		`SELECT id, sha256, relpath, face_idx, bbox, COALESCE(gender,-1), COALESCE(age,-1), COALESCE(thumb_path,'')
		 FROM faces ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil { return nil, err }
	defer rows.Close()
	return scanFaces(rows)
}

func (f *FaceDB) FaceByID(id int) (*FaceRecord, error) {
	r := &FaceRecord{}
	var bboxStr string
	var gender, age int
	err := f.conn.QueryRow(
		`SELECT id, sha256, relpath, face_idx, bbox, COALESCE(gender,-1), COALESCE(age,-1), COALESCE(thumb_path,'')
		 FROM faces WHERE id = ?`, id).Scan(&r.ID, &r.SHA256, &r.RelPath, &r.FaceIdx, &bboxStr, &gender, &age, &r.ThumbPath)
	if err != nil { return nil, err }
	json.Unmarshal([]byte(bboxStr), &r.BBox)
	r.Gender = gender
	r.Age = age
	return r, nil
}

func (f *FaceDB) Embedding(id int) ([]float32, error) {
	var blob []byte
	err := f.conn.QueryRow("SELECT embedding FROM faces WHERE id = ?", id).Scan(&blob)
	if err != nil { return nil, err }
	// blob is packed float32
	n := len(blob) / 4
	emb := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := uint32(blob[i*4]) | uint32(blob[i*4+1])<<8 | uint32(blob[i*4+2])<<16 | uint32(blob[i*4+3])<<24
		emb[i] = math.Float32frombits(bits)
	}
	return emb, nil
}

func (f *FaceDB) AllEmbeddings() ([]FaceRecord, [][]float32, error) {
	rows, err := f.conn.Query(
		`SELECT id, sha256, relpath, face_idx, bbox, COALESCE(gender,-1), COALESCE(age,-1), COALESCE(thumb_path,''), embedding
		 FROM faces ORDER BY id`)
	if err != nil { return nil, nil, err }
	defer rows.Close()

	var faces []FaceRecord
	var embeddings [][]float32
	for rows.Next() {
		var r FaceRecord
		var bboxStr string
		var gender, age int
		var blob []byte
		rows.Scan(&r.ID, &r.SHA256, &r.RelPath, &r.FaceIdx, &bboxStr, &gender, &age, &r.ThumbPath, &blob)
		json.Unmarshal([]byte(bboxStr), &r.BBox)
		r.Gender = gender
		r.Age = age
		n := len(blob) / 4
		emb := make([]float32, n)
		for i := 0; i < n; i++ {
			bits := uint32(blob[i*4]) | uint32(blob[i*4+1])<<8 | uint32(blob[i*4+2])<<16 | uint32(blob[i*4+3])<<24
			emb[i] = math.Float32frombits(bits)
		}
		faces = append(faces, r)
		embeddings = append(embeddings, emb)
	}
	return faces, embeddings, nil
}

func (f *FaceDB) SearchSimilar(queryID int, threshold float64, limit int) ([]FaceRecord, error) {
	queryEmb, err := f.Embedding(queryID)
	if err != nil { return nil, fmt.Errorf("get embedding: %w", err) }

	faces, embeddings, err := f.AllEmbeddings()
	if err != nil { return nil, err }

	type scored struct {
		face FaceRecord
		score float64
	}
	var results []scored
	for i, emb := range embeddings {
		if faces[i].ID == queryID { continue }
		sim := cosineSim(queryEmb, emb)
		if sim >= threshold {
			faces[i].Similarity = math.Round(sim*1000) / 10
			results = append(results, scored{faces[i], sim})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	out := make([]FaceRecord, len(results))
	for i, r := range results { out[i] = r.face }
	return out, nil
}

func cosineSim(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 { return 0 }
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func scanFaces(rows *sql.Rows) ([]FaceRecord, error) {
	var faces []FaceRecord
	for rows.Next() {
		var r FaceRecord
		var bboxStr string
		var gender, age int
		rows.Scan(&r.ID, &r.SHA256, &r.RelPath, &r.FaceIdx, &bboxStr, &gender, &age, &r.ThumbPath)
		json.Unmarshal([]byte(bboxStr), &r.BBox)
		r.Gender = gender
		r.Age = age
		faces = append(faces, r)
	}
	return faces, nil
}

// FaceHandler wraps face API endpoints
type FaceHandler struct {
	fdb       *FaceDB
	mediaRoot string
}

func NewFaceHandler(fdb *FaceDB, mediaRoot string) *FaceHandler {
	return &FaceHandler{fdb: fdb, mediaRoot: mediaRoot}
}

func (fh *FaceHandler) Available() bool { return fh != nil && fh.fdb != nil }

func (fh *FaceHandler) RegisterRoutes(mux *http.ServeMux) {
	if !fh.Available() { return }
	mux.HandleFunc("/api/faces", fh.handleFaces)
	mux.HandleFunc("/api/faces/", fh.handleFaceRoute)
	mux.HandleFunc("/api/face-thumb/", fh.handleFaceThumb)
}

func (fh *FaceHandler) handleFaces(w http.ResponseWriter, r *http.Request) {
	if !fh.Available() {
		sendError(w, "face recognition not enabled", 503)
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
	faces, err := fh.fdb.RecentFaces(limit, offset)
	if err != nil {
		sendError(w, err.Error(), 500)
		return
	}
	total := 0
	fh.fdb.conn.QueryRow("SELECT COUNT(*) FROM faces").Scan(&total)
	sendJSON(w, map[string]interface{}{"faces": faces, "total": total})
}

func (fh *FaceHandler) handleFaceRoute(w http.ResponseWriter, r *http.Request) {
	if !fh.Available() {
		sendError(w, "face recognition not enabled", 503)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/faces/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 1 { sendError(w, "invalid path", 400); return }

	if parts[0] == "status" {
		s, _ := fh.fdb.Status()
		sendJSON(w, s)
		return
	}

	// /api/faces/{id}/similar
	id := parseIntDefault(parts[0], 0)
	if id == 0 { sendError(w, "invalid face id", 400); return }

	if len(parts) == 2 && parts[1] == "similar" {
		threshold := 0.5
		if t := r.URL.Query().Get("threshold"); t != "" {
			if v := parseFloatDefault(t, 0.5); v > 0 { threshold = v }
		}
		limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
		results, err := fh.fdb.SearchSimilar(id, threshold, limit)
		if err != nil {
			sendError(w, err.Error(), 500)
			return
		}
		sendJSON(w, map[string]interface{}{"faces": results, "query_id": id, "threshold": threshold})
		return
	}

	// single face
	face, err := fh.fdb.FaceByID(id)
	if err != nil { sendError(w, "face not found", 404); return }
	sendJSON(w, face)
}

func (fh *FaceHandler) handleFaceThumb(w http.ResponseWriter, r *http.Request) {
	if !fh.Available() {
		sendError(w, "face recognition not enabled", 503)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/face-thumb/")
	id := parseIntDefault(idStr, 0)
	if id == 0 { sendError(w, "invalid face id", 400); return }

	face, err := fh.fdb.FaceByID(id)
	if err != nil { sendError(w, "face not found", 404); return }

	if face.ThumbPath != "" && fileExists(face.ThumbPath) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, face.ThumbPath)
		return
	}
	// fallback: serve the full image thumbnail
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/api/thumb/"+face.SHA256, 302)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func parseIntDefault(s string, def int) int {
	if s == "" { return def }
	var v int
	fmt.Sscanf(s, "%d", &v)
	if v == 0 { return def }
	return v
}

func parseFloatDefault(s string, def float64) float64 {
	if s == "" { return def }
	var v float64
	fmt.Sscanf(s, "%f", &v)
	if v == 0 { return def }
	return v
}

// unused but kept for future cluster feature
var _ = filepath.Join
