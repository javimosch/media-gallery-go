package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type GalleryHandler struct {
	db        *DB
	thumbs    *ThumbCache
	mediaRoot string
	faceDB    *FaceDB
}

func NewGalleryHandler(db *DB, thumbs *ThumbCache, mediaRoot string, faceDB *FaceDB) *GalleryHandler {
	return &GalleryHandler{db: db, thumbs: thumbs, mediaRoot: mediaRoot, faceDB: faceDB}
}

func (g *GalleryHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/browse", g.handleBrowse)
	mux.HandleFunc("/api/search", g.handleSearch)
	mux.HandleFunc("/api/dupes", g.handleDupes)
	mux.HandleFunc("/api/dupes/", g.handleDupeFiles)
	mux.HandleFunc("/api/stats", g.handleStats)
	mux.HandleFunc("/api/categories", g.handleCategories)
	mux.HandleFunc("/api/yearmonths", g.handleYearMonths)
	mux.HandleFunc("/api/file/", g.handleFile)
	mux.HandleFunc("/api/thumb/", g.handleThumb)
	mux.HandleFunc("/api/download/", g.handleDownload)
	mux.HandleFunc("/api/stream/", g.handleStream)
}

func sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": msg,
			"type":    "http_error",
		},
	})
}

func (g *GalleryHandler) handleBrowse(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	yearMonth := r.URL.Query().Get("ym")
	cursor := r.URL.Query().Get("cursor")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	hideJunk := r.URL.Query().Get("hide_junk") == "1"
	hasFace := r.URL.Query().Get("has_face") == "1"

	result, err := g.db.Browse(category, yearMonth, cursor, limit, hideJunk, hasFace, g.faceDB)
	if err != nil {
		sendError(w, err.Error(), 500)
		return
	}
	sendJSON(w, result)
}

func (g *GalleryHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		sendError(w, "missing query parameter 'q'", 400)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	files, err := g.db.Search(q, limit)
	if err != nil {
		sendError(w, err.Error(), 500)
		return
	}
	sendJSON(w, map[string]interface{}{
		"query":  q,
		"count":  len(files),
		"files":  files,
	})
}

func (g *GalleryHandler) handleDupes(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	groups, err := g.db.DupeGroups(limit, offset)
	if err != nil {
		sendError(w, err.Error(), 500)
		return
	}
	sendJSON(w, map[string]interface{}{
		"groups": groups,
		"count":  len(groups),
	})
}

func (g *GalleryHandler) handleDupeFiles(w http.ResponseWriter, r *http.Request) {
	sha := strings.TrimPrefix(r.URL.Path, "/api/dupes/")
	if sha == "" {
		sendError(w, "missing sha256", 400)
		return
	}
	files, err := g.db.DupeFiles(sha)
	if err != nil {
		sendError(w, err.Error(), 500)
		return
	}
	sendJSON(w, map[string]interface{}{
		"sha256": sha,
		"count":  len(files),
		"files":  files,
	})
}

func (g *GalleryHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := g.db.GetStats()
	if err != nil {
		sendError(w, err.Error(), 500)
		return
	}
	sendJSON(w, stats)
}

func (g *GalleryHandler) handleCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := g.db.Categories()
	if err != nil {
		sendError(w, err.Error(), 500)
		return
	}
	sendJSON(w, map[string]interface{}{"categories": cats})
}

func (g *GalleryHandler) handleYearMonths(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	yms, err := g.db.YearMonths(category)
	if err != nil {
		sendError(w, err.Error(), 500)
		return
	}
	sendJSON(w, map[string]interface{}{"year_months": yms})
}

func (g *GalleryHandler) handleFile(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimPrefix(r.URL.Path, "/api/file/")
	var file *FileRecord
	var err error

	if len(query) == 64 {
		// SHA256
		file, err = g.db.GetBySHA(query)
	} else {
		// relpath (URL-encoded)
		file, err = g.db.GetByRelPath(query)
	}
	if err != nil {
		sendError(w, "file not found", 404)
		return
	}
	sendJSON(w, file)
}

func (g *GalleryHandler) handleThumb(w http.ResponseWriter, r *http.Request) {
	sha := strings.TrimPrefix(r.URL.Path, "/api/thumb/")
	if sha == "" || len(sha) != 64 {
		sendError(w, "invalid sha256", 400)
		return
	}

	file, err := g.db.GetBySHA(sha)
	if err != nil {
		sendError(w, "file not found", 404)
		return
	}

	fullPath := filepath.Join(g.mediaRoot, file.RelPath)
	data, err := g.thumbs.Get(sha, fullPath)
	if err != nil {
		// Don't cache errors — browser will retry on next view
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

func (g *GalleryHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	sha := strings.TrimPrefix(r.URL.Path, "/api/download/")
	if sha == "" || len(sha) != 64 {
		sendError(w, "invalid sha256", 400)
		return
	}

	file, err := g.db.GetBySHA(sha)
	if err != nil {
		sendError(w, "file not found", 404)
		return
	}

	fullPath := filepath.Join(g.mediaRoot, file.RelPath)
	stat, err := os.Stat(fullPath)
	if err != nil {
		sendError(w, "file not on disk", 404)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(file.RelPath)))
	http.ServeContent(w, r, filepath.Base(file.RelPath), stat.ModTime(), openFile(fullPath))
}

func (g *GalleryHandler) handleStream(w http.ResponseWriter, r *http.Request) {
	sha := strings.TrimPrefix(r.URL.Path, "/api/stream/")
	if sha == "" || len(sha) != 64 {
		sendError(w, "invalid sha256", 400)
		return
	}

	file, err := g.db.GetBySHA(sha)
	if err != nil {
		sendError(w, "file not found", 404)
		return
	}

	fullPath := filepath.Join(g.mediaRoot, file.RelPath)
	stat, err := os.Stat(fullPath)
	if err != nil {
		sendError(w, "file not on disk", 404)
		return
	}

	ext := strings.ToLower(filepath.Ext(file.RelPath))
	mime := "video/mp4"
	switch ext {
	case ".mov":
		mime = "video/quicktime"
	case ".avi":
		mime = "video/x-msvideo"
	case ".mkv":
		mime = "video/x-matroska"
	case ".wmv":
		mime = "video/x-ms-wmv"
	}
	w.Header().Set("Content-Type", mime)
	http.ServeContent(w, r, filepath.Base(file.RelPath), stat.ModTime(), openFile(fullPath))
}

func openFile(path string) *os.File {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	return f
}
