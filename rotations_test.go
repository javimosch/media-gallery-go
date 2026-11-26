package main

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotations(t *testing.T) {
	g, _ := setupTestGallery(t)
	sha := strings.Repeat("a", 64)
	call := func(method, body string, headers ...string) (int, string) {
		r := httptest.NewRequest(method, "/api/rotations", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		for i := 0; i+1 < len(headers); i += 2 {
			r.Header.Set(headers[i], headers[i+1])
		}
		w := httptest.NewRecorder()
		g.handleRotations(w, r)
		return w.Code, w.Body.String()
	}
	if _, body := call("GET", ""); !strings.Contains(body, `"enabled":false`) {
		t.Error("off without an edits db:", body)
	}
	path := filepath.Join(t.TempDir(), "edits.db")
	edits, err := OpenEditsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	g.edits = edits
	if code, _ := call("POST", `{"sha256":"`+strings.Repeat("9", 64)+`","degrees":90}`); code != 404 {
		t.Error("unknown file", code)
	}
	known := `{"sha256":"` + sha + `","degrees":180}`
	if code, body := call("POST", known); code != 200 {
		t.Error("known file", code, body)
	}
	if _, body := call("GET", ""); !strings.Contains(body, `"`+sha+`":180`) {
		t.Error("listed", body)
	}
	for _, bad := range []string{`{}`, `{"sha256":"x","degrees":90}`, `{"sha256":"` + sha + `","degrees":45}`, `{"sha256":"` + sha + `"}`} {
		if code, _ := call("POST", bad); code != 400 {
			t.Error(bad, code)
		}
	}
	if code, _ := call("POST", `{}`, "Origin", "https://evil.test"); code != 403 {
		t.Error("cross-site", code)
	}
	if code, _ := call("POST", `{}`, "Content-Type", "text/plain"); code != 403 {
		t.Error("form post", code)
	}
	for _, d := range []int{90, 270} {
		if err := edits.SetRotation(sha, d); err != nil {
			t.Fatal(err)
		}
	}
	edits.Close()
	again, err := OpenEditsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if m, _ := again.Rotations(); len(m) != 1 || m[sha] != 270 {
		t.Error("must persist the latest value", m)
	}
	again.SetRotation(sha, 0)
	if m, _ := again.Rotations(); len(m) != 0 {
		t.Error("0 removes it", m)
	}
}
