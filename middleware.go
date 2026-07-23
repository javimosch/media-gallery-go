package main

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Middleware func(http.Handler) http.Handler

func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

func AuthMiddleware(user, pass string) Middleware {
	if user == "" && pass == "" {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(user)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(pass)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Media Gallery"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimiter limits concurrent heavy requests (downloads/streams)
// to avoid eating all bandwidth on the fibre link
type RateLimiter struct {
	sem chan struct{}
}

func NewRateLimiter(maxConcurrent int) *RateLimiter {
	return &RateLimiter{sem: make(chan struct{}, maxConcurrent)}
}

func (rl *RateLimiter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// only rate limit heavy endpoints
			path := r.URL.Path
			if !strings.HasPrefix(path, "/api/download/") &&
				!strings.HasPrefix(path, "/api/stream/") {
				next.ServeHTTP(w, r)
				return
			}
			select {
			case rl.sem <- struct{}{}:
				defer func() { <-rl.sem }()
				next.ServeHTTP(w, r)
			default:
				http.Error(w, `{"error":{"code":429,"message":"Too many concurrent downloads"}}`, http.StatusTooManyRequests)
			}
		})
	}
}

func CacheMiddleware(maxAge int) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
			next.ServeHTTP(w, r)
		})
	}
}

// GzipMiddleware for API responses (lightweight, only for JSON)
func GzipMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}
			path := r.URL.Path
			// only gzip API JSON, not thumbnails or streams
			if !strings.HasPrefix(path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Encoding", "gzip")
			next.ServeHTTP(w, r)
		})
	}
}

// LoggingMiddleware logs requests to stderr
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func LoggingMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(sw, r)
			logf("%s %s %d %v", r.Method, r.URL.Path, sw.status, time.Since(start))
		})
	}
}

var logMu sync.Mutex

func logf(format string, args ...interface{}) {
	logMu.Lock()
	defer logMu.Unlock()
	// stderr only
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
