package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed ui/*
var uiFiles embed.FS

var (
	serverInstance *http.Server
	serverMu       sync.Mutex
)

type ServerConfig struct {
	Port      int
	MediaRoot string
	DBPath    string
	ThumbDir  string
	AuthUser  string
	AuthPass  string
	MaxConcurrent int
	FacesDB   string
}

type StatusResponse struct {
	Status    string    `json:"status"`
	Port      int       `json:"port"`
	Uptime    string    `json:"uptime"`
	Version   string    `json:"version"`
	StartTime time.Time `json:"start_time"`
	MediaRoot string    `json:"media_root"`
	Files     int       `json:"files_indexed"`
}

var serverStatus StatusResponse

func startServer(cfg ServerConfig) {
	db, err := OpenDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	thumbs := NewThumbCache(cfg.ThumbDir)

	// Face recognition (opt-in: only if faces-db flag is set and file exists)
	var faceHandler *FaceHandler
	var faceDB *FaceDB
	if cfg.FacesDB != "" {
		if fdb, err := OpenFaceDB(cfg.FacesDB); err == nil {
			faceDB = fdb
			faceHandler = NewFaceHandler(fdb, cfg.MediaRoot)
			log.Printf("Face recognition: enabled (%s)", cfg.FacesDB)
		} else {
			log.Printf("Face recognition: disabled (%v)", err)
		}
	}

	gallery := NewGalleryHandler(db, thumbs, cfg.MediaRoot, faceDB)

	mux := http.NewServeMux()

	// API endpoints
	gallery.RegisterRoutes(mux)
	mux.HandleFunc("/api/status", handleStatusAPI)
	mux.HandleFunc("/api/health", handleHealthAPI)

	// Face recognition routes
	if faceHandler != nil {
		faceHandler.RegisterRoutes(mux)
	}

	// Static UI
	uiSub, _ := fs.Sub(uiFiles, "ui")
	fileServer := http.FileServer(http.FS(uiSub))
	mux.Handle("/", fileServer)

	// middleware chain
	handler := Chain(mux,
		LoggingMiddleware(),
		AuthMiddleware(cfg.AuthUser, cfg.AuthPass),
		NewRateLimiter(cfg.MaxConcurrent).Middleware(),
		CacheMiddleware(3600),
	)

	serverStatus = StatusResponse{
		Status:    "running",
		Port:      cfg.Port,
		Version:   Version,
		StartTime: time.Now(),
		MediaRoot: cfg.MediaRoot,
	}

	count, _ := db.UniqueSHA256Count()
	serverStatus.Files = count

	serverInstance = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // long for video streaming
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Media Gallery starting on :%d", cfg.Port)
	log.Printf("Media root: %s", cfg.MediaRoot)
	log.Printf("Database: %s", cfg.DBPath)
	log.Printf("Thumbnails: %s", cfg.ThumbDir)
	log.Printf("Files indexed: %d", count)

	if err := serverInstance.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func handleStatusAPI(w http.ResponseWriter, r *http.Request) {
	serverMu.Lock()
	serverStatus.Uptime = time.Since(serverStatus.StartTime).String()
	serverMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(serverStatus)
}

func handleHealthAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"version": Version,
	})
}

// resolvePath safely joins mediaRoot with relpath, preventing directory traversal
func resolvePath(mediaRoot, relpath string) (string, bool) {
	clean := filepath.Clean("/" + relpath)
	if strings.Contains(clean, "..") {
		return "", false
	}
	full := filepath.Join(mediaRoot, clean)
	absRoot, _ := filepath.Abs(mediaRoot)
	absFull, _ := filepath.Abs(full)
	if !strings.HasPrefix(absFull, absRoot) {
		return "", false
	}
	return full, true
}

func init() {
	// suppress embed errors at init
	_ = os.Stderr
}
