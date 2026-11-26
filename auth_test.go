package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testAuth(t *testing.T) (*Auth, string, http.Handler) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	for _, u := range []string{"alice", "bob"} {
		if err := setPassword(path, u, "correct horse"); err != nil {
			t.Fatal(err)
		}
	}
	a, err := NewAuth(path, "10.9.0.1", "10.9.0.2")
	if err != nil {
		t.Fatal(err)
	}
	return a, path, a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Write([]byte("SECRET"))
	}))
}

func req(h http.Handler, method, path, peer, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = peer + ":5000"
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func signIn(h http.Handler, peer, user, pass string, extra ...string) *httptest.ResponseRecorder {
	headers := map[string]string{"Content-Type": "application/json"}
	for i := 0; i+1 < len(extra); i += 2 {
		headers[extra[i]] = extra[i+1]
	}
	return req(h, "POST", "/auth/login", peer, `{"username":"`+user+`","password":"`+pass+`"}`, headers)
}

func TestAuthGate(t *testing.T) {
	_, _, h := testAuth(t)
	for _, p := range []string{"/api/browse", "/api/thumb/x", "/api/download/x", "/api/stream/x", "/api/faces", "/api/stats",
		"/js/app.js", "/index.html", "/auth/me", "/login/../api/browse", "/api/replicas/register"} {
		w := req(h, "GET", p, "8.8.8.8", "", map[string]string{"Authorization": "Basic YWRtaW46eA=="})
		if w.Code != 401 || strings.Contains(w.Body.String(), "SECRET") {
			t.Errorf("%s: %d %s", p, w.Code, w.Body.String())
		}
	}
	if w := req(h, "GET", "/", "8.8.8.8", "", nil); w.Code != 303 || w.Header().Get("Location") != "/login" {
		t.Error("root must redirect", w.Code)
	}
	if w := req(h, "GET", "/login", "8.8.8.8", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "Show password") {
		t.Error("login page", w.Code)
	}
	// loopback and replica exceptions apply to direct requests only
	if w := req(h, "GET", "/api/health", "127.0.0.1", "", nil); w.Code != 200 {
		t.Error("loopback", w.Code)
	}
	if w := req(h, "GET", "/api/browse", "127.0.0.1", "", map[string]string{"X-Forwarded-For": "8.8.8.8"}); w.Code != 401 {
		t.Error("proxied loopback must sign in", w.Code)
	}
	if w := req(h, "POST", "/api/replicas/register", "10.9.0.2", "{}", nil); w.Code != 200 {
		t.Error("replica register", w.Code)
	}
	for _, c := range []*httptest.ResponseRecorder{
		req(h, "GET", "/api/browse", "10.9.0.2", "", nil),
		req(h, "POST", "/api/replicas/register", "10.9.0.2", "{}", map[string]string{"X-Forwarded-For": "1.2.3.4"}),
		req(h, "POST", "/api/replicas/register", "10.9.0.3", "{}", nil),
		req(h, "GET", "/api/browse", "10.9.0.1", "", nil),
	} {
		if c.Code != 401 {
			t.Error("exception too wide", c.Code)
		}
	}
}

func TestAuthSession(t *testing.T) {
	a, path, h := testAuth(t)
	if w := signIn(h, "8.8.8.8", "alice", "wrong password"); w.Code != 401 || len(w.Result().Cookies()) != 0 {
		t.Fatal("wrong password", w.Code)
	}
	if w := signIn(h, "8.8.8.8", "nobody", "correct horse"); w.Code != 401 {
		t.Fatal("unknown user", w.Code)
	}
	if w := signIn(h, "8.8.8.8", "alice", "correct horse", "Origin", "https://evil.test"); w.Code != 403 {
		t.Fatal("cross-site", w.Code)
	}
	w := signIn(h, "8.8.8.8", " Alice ", "correct horse", "X-Forwarded-Proto", "https")
	if w.Code != 200 {
		t.Fatal("sign in", w.Code, w.Body.String())
	}
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 30*24*3600 {
		t.Errorf("cookie flags %+v", c)
	}
	cookie := map[string]string{"Cookie": c.Name + "=" + c.Value}
	got := req(h, "GET", "/api/browse", "8.8.8.8", "", cookie)
	if got.Code != 200 || got.Header().Get("Cache-Control") != "private, max-age=60" {
		t.Error("signed in", got.Code, got.Header().Get("Cache-Control"))
	}
	if me := req(h, "GET", "/auth/me", "8.8.8.8", "", cookie); !strings.Contains(me.Body.String(), `"alice"`) {
		t.Error("me", me.Body.String())
	}
	out := req(h, "POST", "/auth/logout", "8.8.8.8", "", cookie)
	if out.Code != 200 || out.Result().Cookies()[0].MaxAge >= 0 {
		t.Error("logout", out.Code)
	}
	// tampered, expired and wrong-user tokens
	parts := strings.Split(c.Value, ".")
	for name, v := range map[string]string{
		"tampered": c.Value[:len(c.Value)-2] + "AA",
		"swapped":  "Ym9i." + parts[1] + "." + parts[2],
		"expired":  a.token("alice", time.Now().Add(-time.Second)),
	} {
		if w := req(h, "GET", "/api/browse", "8.8.8.8", "", map[string]string{"Cookie": sessionCookie + "=" + v}); w.Code != 401 {
			t.Error(name, w.Code)
		}
	}
	// a password change ends that user's sessions only
	bob := a.token("bob", time.Now().Add(time.Hour))
	if err := setPassword(path, "alice", "another password"); err != nil {
		t.Fatal(err)
	}
	b, _ := NewAuth(path, "", "")
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Cookie", sessionCookie+"="+c.Value)
	if b.sessionUser(r) != "" {
		t.Error("old session survived a password change")
	}
	r.Header.Set("Cookie", sessionCookie+"="+bob)
	if b.sessionUser(r) != "bob" {
		t.Error("other user was signed out")
	}
	if _, err := NewAuth(path+".missing", "", ""); err == nil {
		t.Error("a missing auth file must be an error")
	}
}

func TestLoginRateLimit(t *testing.T) {
	var l attemptLimiter
	now := time.Now()
	for i := 0; i < 10; i++ {
		if ok, _ := l.allow("a", now.Add(time.Duration(i)*time.Second)); !ok {
			t.Fatal("attempt", i+1)
		}
	}
	if ok, wait := l.allow("a", now.Add(10*time.Second)); ok || wait != 50*time.Second {
		t.Error("11th attempt", ok, wait)
	}
	if ok, _ := l.allow("b", now.Add(10*time.Second)); !ok {
		t.Error("another visitor")
	}
	if ok, _ := l.allow("a", now.Add(60*time.Second)); !ok {
		t.Error("window must slide")
	}
	var all attemptLimiter
	for i := 0; i < 60; i++ {
		all.allow(string(rune('A'+i)), now)
	}
	if ok, _ := all.allow("new", now); ok {
		t.Error("overall limit")
	}

	_, _, h := testAuth(t)
	for i := 0; i < 10; i++ {
		// through the trusted proxy, forged entries before the proxy's own are ignored
		if w := signIn(h, "10.9.0.1", "alice", "wrong password", "X-Forwarded-For", "1.1.1."+string(rune('0'+i))+", 9.9.9.9"); w.Code != 401 {
			t.Fatal(i, w.Code)
		}
	}
	w := signIn(h, "10.9.0.1", "alice", "correct horse", "X-Forwarded-For", "9.9.9.9")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Error("limited even with the right password", w.Code)
	}
	if w := signIn(h, "10.9.0.1", "alice", "correct horse", "X-Forwarded-For", "9.9.9.8"); w.Code != 200 {
		t.Error("other visitor behind the proxy", w.Code)
	}
	// an untrusted peer cannot pick its own identity
	for i := 0; i < 10; i++ {
		signIn(h, "7.7.7.7", "alice", "wrong password", "X-Forwarded-For", "2.2.2."+string(rune('0'+i)))
	}
	if w := signIn(h, "7.7.7.7", "alice", "correct horse", "X-Forwarded-For", "3.3.3.3"); w.Code != 429 {
		t.Error("spoofed header must not reset the limit", w.Code)
	}
}
