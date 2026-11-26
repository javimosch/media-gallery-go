package main

import (
	"database/sql/driver"
	"net/http"
	"sync/atomic"

	"modernc.org/sqlite"
)

// hiddenPaths holds the relpaths that cannot currently be served: absent from
// the primary root with no healthy replica to proxy them. nil = nothing hidden.
var hiddenPaths atomic.Pointer[map[string]bool]

func isVisible(relpath string) bool {
	m := hiddenPaths.Load()
	return m == nil || !(*m)[relpath]
}

// visible(relpath) lets queries filter in SQL, so paging and totals stay exact.
// Not deterministic: the result changes with replica health.
func init() {
	sqlite.MustRegisterScalarFunction("visible", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			p, _ := args[0].(string)
			if isVisible(p) {
				return int64(1), nil
			}
			return int64(0), nil
		})
}

// syncVisibility applies current replica health to the hidden set and returns
// the number of hidden files.
func (g *GalleryHandler) syncVisibility() int {
	if len(g.replicas.Healthy()) > 0 {
		hiddenPaths.Store(nil)
		return 0
	}
	m := g.sources.Missing()
	hiddenPaths.Store(&m)
	return len(m)
}

// WithVisibility refreshes visibility before every request.
func (g *GalleryHandler) WithVisibility(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.syncVisibility()
		next.ServeHTTP(w, r)
	})
}
