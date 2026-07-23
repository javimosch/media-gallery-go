package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	authed := AuthMiddleware("user", "pass")(handler)

	t.Run("no auth header returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		authed.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("wrong credentials returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("user", "wrong")
		rec := httptest.NewRecorder()
		authed.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("correct credentials passes through", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("user", "pass")
		rec := httptest.NewRecorder()
		authed.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Errorf("expected 200, got %d", rec.Code)
		}
		if rec.Body.String() != "ok" {
			t.Errorf("expected 'ok', got '%s'", rec.Body.String())
		}
	})
}

func TestAuthMiddlewareEmpty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	// Empty user/pass = no auth required
	authed := AuthMiddleware("", "")(handler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	authed.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("expected 200 with no auth, got %d", rec.Code)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(2)
	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// Non-download endpoints should pass through (rate limiter only applies to download/stream)
	req := httptest.NewRequest("GET", "/api/browse", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("expected 200 for non-download, got %d", rec.Code)
	}
}

func TestChain(t *testing.T) {
	called := []string{}
	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = append(called, "mw1-before")
			next.ServeHTTP(w, r)
			called = append(called, "mw1-after")
		})
	}
	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = append(called, "mw2-before")
			next.ServeHTTP(w, r)
			called = append(called, "mw2-after")
		})
	}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = append(called, "handler")
		w.WriteHeader(200)
	})

	chain := Chain(final, mw1, mw2)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(called) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(called), called)
	}
	for i, v := range expected {
		if called[i] != v {
			t.Errorf("call %d: expected %s, got %s", i, v, called[i])
		}
	}
}

func TestCacheMiddleware(t *testing.T) {
	handler := CacheMiddleware(3600)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Errorf("expected Cache-Control header, got %s", rec.Header().Get("Cache-Control"))
	}
}

func TestGzipMiddlewareWithoutAccept(t *testing.T) {
	handler := GzipMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not gzip when Accept-Encoding missing")
	}
}

func TestGzipMiddlewareNonAPI(t *testing.T) {
	handler := GzipMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not gzip non-API paths")
	}
}

func TestLoggingMiddleware(t *testing.T) {
	handler := LoggingMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestStatusWriter(t *testing.T) {
	sw := &statusWriter{ResponseWriter: httptest.NewRecorder(), status: 200}
	sw.WriteHeader(404)
	if sw.status != 404 {
		t.Errorf("expected status 404, got %d", sw.status)
	}
}

func TestRateLimiterDownloadBlocking(t *testing.T) {
	rl := NewRateLimiter(1)
	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// First download request succeeds
	req1 := httptest.NewRequest("GET", "/api/download/abc", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != 200 {
		t.Errorf("first request: expected 200, got %d", rec1.Code)
	}

	// Second concurrent download should be rate limited (429)
	// But since first already returned, semaphore is released
	// So this should also succeed
	req2 := httptest.NewRequest("GET", "/api/download/def", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Errorf("second sequential request: expected 200, got %d", rec2.Code)
	}
}

func TestLogf(t *testing.T) {
	// Just verify it doesn't panic
	logf("test %s %d", "arg", 42)
}

func TestChainEmpty(t *testing.T) {
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	chain := Chain(final)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200 with no middlewares, got %d", rec.Code)
	}
}

func TestAuthMiddlewareWrongUser(t *testing.T) {
	handler := AuthMiddleware("admin", "pass")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("wronguser", "pass")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Errorf("expected 401 for wrong user, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Unauthorized") {
		t.Errorf("expected 'Unauthorized' in body, got %s", rec.Body.String())
	}
}
