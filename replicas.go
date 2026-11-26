package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Replica is a registered fallback node that can serve media files
// the primary media root does not have on disk.
type Replica struct {
	URL      string    `json:"url"`
	Name     string    `json:"name"`
	Files    int       `json:"files"`
	Status   string    `json:"status"` // "healthy" | "stale" | "offline"
	LastSeen time.Time `json:"last_seen"`
}

// ReplicaRegistry tracks registered replica nodes (in-memory, not persisted).
type ReplicaRegistry struct {
	mu        sync.RWMutex
	replicas  map[string]*Replica // keyed by name
	client    *http.Client
	healthTTL time.Duration
}

func NewReplicaRegistry() *ReplicaRegistry {
	return &ReplicaRegistry{
		replicas:  make(map[string]*Replica),
		client:    &http.Client{Timeout: 15 * time.Second},
		healthTTL: 3 * time.Minute, // replica considered offline after this without heartbeat
	}
}

// Register adds or refreshes a replica registration (heartbeat).
func (r *ReplicaRegistry) Register(url, name string, files int) {
	if url == "" || name == "" {
		return
	}
	// normalize: strip trailing slash
	url = strings.TrimRight(url, "/")
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.replicas[name]; ok {
		existing.URL = url
		existing.Files = files
		existing.LastSeen = time.Now()
		existing.Status = "healthy"
	} else {
		r.replicas[name] = &Replica{
			URL:      url,
			Name:     name,
			Files:    files,
			Status:   "healthy",
			LastSeen: time.Now(),
		}
	}
	log.Printf("[replicas] registered %q at %s (%d files)", name, url, files)
}

// Deregister removes a replica by name.
func (r *ReplicaRegistry) Deregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.replicas, name)
	log.Printf("[replicas] deregistered %q", name)
}

// List returns all replicas with health refreshed.
func (r *ReplicaRegistry) List() []Replica {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Replica, 0, len(r.replicas))
	for _, rep := range r.replicas {
		if time.Since(rep.LastSeen) > r.healthTTL {
			rep.Status = "offline"
		}
		out = append(out, *rep)
	}
	// stable order by name
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Name < out[j-1].Name; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Healthy returns registered replicas that are currently healthy.
func (r *ReplicaRegistry) Healthy() []*Replica {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Replica
	for _, rep := range r.replicas {
		if time.Since(rep.LastSeen) <= r.healthTTL {
			out = append(out, rep)
		}
	}
	return out
}

// ProxyFile forwards a media request (thumb/download/stream) to this replica.
// It returns the response so the caller can stream it to the client.
func (r *ReplicaRegistry) ProxyFile(rep *Replica, kind, sha string, rangeHeader string) (*http.Response, error) {
	target := fmt.Sprintf("%s/api/%s/%s", rep.URL, kind, sha)
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return nil, err
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("replica %s: not found", rep.Name)
	}
	if resp.StatusCode >= 400 {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("replica %s: status %d", rep.Name, resp.StatusCode)
	}
	return resp, nil
}

// handleReplicaRegister is the gallery endpoint replicas POST to.
func (g *GalleryHandler) handleReplicaRegister(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		sendError(w, "method not allowed", 405)
		return
	}
	var reg struct {
		URL   string `json:"url"`
		Name  string `json:"name"`
		Files int    `json:"files"`
	}
	if err := json.NewDecoder(req.Body).Decode(&reg); err != nil {
		sendError(w, "invalid JSON", 400)
		return
	}
	if reg.URL == "" || reg.Name == "" {
		sendError(w, "url and name are required", 400)
		return
	}
	g.replicas.Register(reg.URL, reg.Name, reg.Files)
	sendJSON(w, map[string]interface{}{"ok": true})
}

// handleReplicaList lists registered replicas.
func (g *GalleryHandler) handleReplicaList(w http.ResponseWriter, req *http.Request) {
	if req.Method == "DELETE" {
		name := strings.TrimPrefix(req.URL.Path, "/api/replicas/")
		if name == "" {
			sendError(w, "missing replica name", 400)
			return
		}
		g.replicas.Deregister(name)
		sendJSON(w, map[string]interface{}{"ok": true})
		return
	}
	list := g.replicas.List()
	sendJSON(w, map[string]interface{}{
		"replicas": list,
		"count":    len(list),
	})
}

// SourceTracker records which relpaths are missing from the primary media root
// (and so can only be served by a replica). Refreshed by a background walk.
type SourceTracker struct {
	mu      sync.RWMutex
	missing map[string]bool // relpath -> true; replaced wholesale, never mutated
}

func NewSourceTracker() *SourceTracker {
	return &SourceTracker{missing: make(map[string]bool)}
}

func (t *SourceTracker) set(m map[string]bool) {
	t.mu.Lock()
	t.missing = m
	t.mu.Unlock()
}

// Missing returns the current missing set. Callers must not modify it.
func (t *SourceTracker) Missing() map[string]bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.missing
}

// Refresh walks the DB index and stats each relpath on the primary root,
// rebuilding the missing set. Safe to run periodically.
func (t *SourceTracker) Refresh(db *DB, mediaRoot string) {
	paths, err := db.AllRelPaths()
	if err != nil {
		return
	}

	// Quick first pass: a stat per top-level dir hides whole absent trees
	// immediately, instead of after the full walk (~40s for 150k files).
	if len(t.Missing()) == 0 {
		topOK := map[string]bool{}
		quick := map[string]bool{}
		for _, p := range paths {
			top, _, _ := strings.Cut(p, "/")
			ok, seen := topOK[top]
			if !seen {
				_, err := os.Stat(filepath.Join(mediaRoot, top))
				ok = err == nil
				topOK[top] = ok
			}
			if !ok {
				quick[p] = true
			}
		}
		t.set(quick)
	}

	missing := make(map[string]bool)
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(mediaRoot, p)); err != nil {
			missing[p] = true
		}
	}
	t.set(missing)
}

// Source returns "local" or "replica" for a relpath.
func (t *SourceTracker) Source(relpath string) string {
	if t.Missing()[relpath] {
		return "replica"
	}
	return "local"
}
