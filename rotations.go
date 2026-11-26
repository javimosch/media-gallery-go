package main

// Image rotations are what people change in the viewer. They live in their own
// small database (-edits-db), so the media inventory stays read-only and can be
// rebuilt without losing them. Originals are never modified: the UI rotates what
// it displays. Rotations are keyed by content, so identical copies share one.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type EditsDB struct{ conn *sql.DB }

func OpenEditsDB(path string) (*EditsDB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS rotations (
		sha256 TEXT PRIMARY KEY, degrees INTEGER NOT NULL, updated_at INTEGER NOT NULL)`); err != nil {
		conn.Close()
		return nil, err
	}
	return &EditsDB{conn}, nil
}

func (e *EditsDB) Close() error { return e.conn.Close() }

// Rotations returns the clockwise degrees of every rotated file.
func (e *EditsDB) Rotations() (map[string]int, error) {
	rows, err := e.conn.Query("SELECT sha256, degrees FROM rotations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var d int
		if err := rows.Scan(&s, &d); err != nil {
			return nil, err
		}
		out[s] = d
	}
	return out, rows.Err()
}

// SetRotation stores 90, 180 or 270; 0 removes the rotation.
func (e *EditsDB) SetRotation(sha string, degrees int) error {
	if degrees == 0 {
		_, err := e.conn.Exec("DELETE FROM rotations WHERE sha256 = ?", sha)
		return err
	}
	_, err := e.conn.Exec(`INSERT INTO rotations VALUES (?, ?, ?)
		ON CONFLICT(sha256) DO UPDATE SET degrees = excluded.degrees, updated_at = excluded.updated_at`,
		sha, degrees, time.Now().Unix())
	return err
}

var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// handleRotations: GET lists them; POST {"sha256", "degrees"} saves one.
func (g *GalleryHandler) handleRotations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		if g.edits == nil {
			sendJSON(w, map[string]interface{}{"enabled": false, "rotations": map[string]int{}})
			return
		}
		list, err := g.edits.Rotations()
		if err != nil {
			sendError(w, err.Error(), 500)
			return
		}
		sendJSON(w, map[string]interface{}{"enabled": true, "rotations": list})
	case http.MethodPost:
		if g.edits == nil {
			sendError(w, "rotation is not enabled", 404)
			return
		}
		// a cross-site page cannot send JSON without the browser asking first
		if !sameOrigin(r) || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			sendError(w, "request refused", 403)
			return
		}
		var in struct {
			SHA256  string `json:"sha256"`
			Degrees *int   `json:"degrees"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil ||
			!shaPattern.MatchString(in.SHA256) || in.Degrees == nil ||
			(*in.Degrees != 0 && *in.Degrees != 90 && *in.Degrees != 180 && *in.Degrees != 270) {
			sendError(w, "need sha256 and degrees of 0, 90, 180 or 270", 400)
			return
		}
		if _, err := g.db.GetBySHA(in.SHA256); err != nil {
			sendError(w, "file not found", 404)
			return
		}
		if err := g.edits.SetRotation(in.SHA256, *in.Degrees); err != nil {
			sendError(w, err.Error(), 500)
			return
		}
		sendJSON(w, map[string]interface{}{"sha256": in.SHA256, "degrees": *in.Degrees})
	default:
		sendError(w, "method not allowed", 405)
	}
}
