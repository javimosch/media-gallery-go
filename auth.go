package main

// Sign-in with a login page and a signed session cookie, enabled by -auth-file.
//
// The file (keep it private, outside any repository) holds bcrypt hashes and the
// cookie signing key:
//
//	{"secret": "<base64>", "users": {"name": "<bcrypt hash>"}}
//
// Create or change a user with: media-gallery-go passwd FILE USER  (password on stdin)

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

//go:embed login.html
var loginPage []byte

const (
	sessionCookie = "mg_session"
	sessionTTL    = 30 * 24 * time.Hour
	loginWindow   = time.Minute
	loginPerIP    = 10 // sign-in attempts per visitor per minute
	loginOverall  = 60 // and from everyone together
)

type authFile struct {
	Secret string            `json:"secret"`
	Users  map[string]string `json:"users"`
}

func readAuthFile(path string) (*authFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f authFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("auth file: %w", err)
	}
	return &f, nil
}

// setPassword adds or updates a user, creating the file and key if needed.
func setPassword(path, user, password string) error {
	if user == "" || strings.ContainsAny(user, "|\n") || len(password) < 8 || len(password) > 72 {
		return errors.New("need a user name without '|' and a password of 8 to 72 bytes")
	}
	f, err := readAuthFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		f = &authFile{Secret: base64.StdEncoding.EncodeToString(key), Users: map[string]string{}}
	} else if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	f.Users[user] = string(hash)
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func handlePasswd(args []string, stdin io.Reader) error {
	if len(args) != 2 {
		return errors.New("usage: media-gallery-go passwd AUTH_FILE USER  (password on stdin)")
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	return setPassword(args[0], args[1], strings.TrimRight(line, "\r\n"))
}

type attemptLimiter struct {
	mu  sync.Mutex
	all []time.Time
	ip  map[string][]time.Time
}

func recent(list []time.Time, now time.Time) []time.Time {
	i := 0
	for i < len(list) && now.Sub(list[i]) >= loginWindow {
		i++
	}
	return list[i:]
}

// allow records an attempt in a sliding one-minute window, or returns how long
// to wait. Refused attempts are not recorded, so waiting always works.
func (l *attemptLimiter) allow(ip string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ip == nil {
		l.ip = map[string][]time.Time{}
	}
	l.all = recent(l.all, now)
	for k, v := range l.ip {
		if v = recent(v, now); len(v) == 0 {
			delete(l.ip, k)
		} else {
			l.ip[k] = v
		}
	}
	if mine := l.ip[ip]; len(mine) >= loginPerIP {
		return false, loginWindow - now.Sub(mine[0])
	}
	if len(l.all) >= loginOverall {
		return false, loginWindow - now.Sub(l.all[0])
	}
	l.all = append(l.all, now)
	l.ip[ip] = append(l.ip[ip], now)
	return true, 0
}

type Auth struct {
	key          []byte
	users        map[string]string
	decoy        []byte // compared for unknown users, so both take as long
	trusted      []*net.IPNet
	registerFrom []*net.IPNet
	limiter      attemptLimiter
}

func parseNets(list string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, s := range strings.Split(list, ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			if strings.Contains(s, ":") {
				s += "/128"
			} else {
				s += "/32"
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func inNets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if ip != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// NewAuth loads the auth file. trustedProxies may name the visitor in
// X-Forwarded-For; registerFrom may register replicas without signing in.
func NewAuth(path, trustedProxies, registerFrom string) (*Auth, error) {
	f, err := readAuthFile(path)
	if err != nil {
		return nil, err
	}
	a := &Auth{users: f.Users}
	if a.key, err = base64.StdEncoding.DecodeString(f.Secret); err != nil || len(a.key) < 32 {
		return nil, errors.New("auth file: missing or short secret")
	}
	if len(a.users) == 0 {
		return nil, errors.New("auth file: no users")
	}
	if a.decoy, err = bcrypt.GenerateFromPassword([]byte("decoy-password"), bcrypt.DefaultCost); err != nil {
		return nil, err
	}
	if a.trusted, err = parseNets(trustedProxies); err != nil {
		return nil, fmt.Errorf("-trusted-proxies: %w", err)
	}
	if a.registerFrom, err = parseNets(registerFrom); err != nil {
		return nil, fmt.Errorf("-replica-from: %w", err)
	}
	return a, nil
}

func peerIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

func forwarded(r *http.Request) bool {
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip", "Forwarded"} {
		if r.Header.Get(h) != "" {
			return true
		}
	}
	return false
}

// clientIP is the last X-Forwarded-For entry (the one our proxy added) when the
// peer is a trusted proxy, else the peer itself.
func (a *Auth) clientIP(r *http.Request) string {
	peer := peerIP(r)
	if inNets(peer, a.trusted) {
		parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
		if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
			return ip.String()
		}
	}
	return peer.String()
}

// sign covers the user's password hash, so changing a password ends sessions.
func (a *Auth) sign(user string, expires int64) string {
	m := hmac.New(sha256.New, a.key)
	fmt.Fprintf(m, "%s|%d|%s", user, expires, a.users[user])
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (a *Auth) token(user string, expires time.Time) string {
	return base64.RawURLEncoding.EncodeToString([]byte(user)) + "." +
		strconv.FormatInt(expires.Unix(), 10) + "." + a.sign(user, expires.Unix())
}

func (a *Auth) sessionUser(r *http.Request) string {
	for _, c := range r.Cookies() {
		if c.Name != sessionCookie {
			continue
		}
		p := strings.Split(c.Value, ".")
		if len(p) != 3 {
			continue
		}
		name, err1 := base64.RawURLEncoding.DecodeString(p[0])
		expires, err2 := strconv.ParseInt(p[1], 10, 64)
		if err1 != nil || err2 != nil || time.Now().Unix() >= expires || a.users[string(name)] == "" {
			continue
		}
		if hmac.Equal([]byte(p[2]), []byte(a.sign(string(name), expires))) {
			return string(name)
		}
	}
	return ""
}

func setCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}

// sameOrigin rejects cross-site form posts. Browsers send Origin on POST.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

func authJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func authError(w http.ResponseWriter, status int, msg string) {
	authJSON(w, status, map[string]string{"error": msg})
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		authError(w, http.StatusForbidden, "request refused")
		return
	}
	ip := a.clientIP(r)
	if ok, wait := a.limiter.allow(ip, time.Now()); !ok {
		seconds := int(wait/time.Second) + 1
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		authJSON(w, http.StatusTooManyRequests, map[string]interface{}{"error": "too many attempts", "retry_after": seconds})
		return
	}
	var in struct{ Username, Password string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		authError(w, http.StatusBadRequest, "bad request")
		return
	}
	// phone keyboards like to capitalise and pad user names
	user := strings.ToLower(strings.TrimSpace(in.Username))
	hash, known := a.users[user]
	if !known {
		hash = string(a.decoy)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil || !known {
		logf("auth: failed sign-in from %s", ip)
		authError(w, http.StatusUnauthorized, "wrong username or password")
		return
	}
	logf("auth: sign-in %q from %s", user, ip)
	setCookie(w, r, a.token(user, time.Now().Add(sessionTTL)), int(sessionTTL/time.Second))
	authJSON(w, http.StatusOK, map[string]string{"user": user})
}

// privateCache keeps shared caches from storing anything served after sign-in.
type privateCache struct {
	http.ResponseWriter
	done bool
}

func (p *privateCache) WriteHeader(code int) {
	if !p.done {
		p.done = true
		h := p.Header()
		if v := h.Get("Cache-Control"); strings.HasPrefix(v, "public") {
			h.Set("Cache-Control", "private"+strings.TrimPrefix(v, "public"))
		}
	}
	p.ResponseWriter.WriteHeader(code)
}

func (p *privateCache) Write(b []byte) (int, error) {
	if !p.done {
		p.WriteHeader(http.StatusOK)
	}
	return p.ResponseWriter.Write(b)
}

func (p *privateCache) Flush() {
	if f, ok := p.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Wrap protects everything except the login page and the sign-in request.
func (a *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		switch {
		case r.URL.Path == "/login" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			if a.sessionUser(r) != "" {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Write(loginPage)
			return
		case r.URL.Path == "/auth/login" && r.Method == http.MethodPost:
			a.login(w, r)
			return
		}
		peer := peerIP(r)
		// Direct requests only: anything that came through a proxy must sign in.
		if !forwarded(r) && (peer.IsLoopback() ||
			(r.Method == http.MethodPost && r.URL.Path == "/api/replicas/register" && inNets(peer, a.registerFrom))) {
			next.ServeHTTP(w, r)
			return
		}
		user := a.sessionUser(r)
		if user == "" {
			if r.URL.Path == "/" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				w.Header().Set("Cache-Control", "no-store")
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			authError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		switch r.URL.Path {
		case "/auth/me":
			authJSON(w, http.StatusOK, map[string]string{"user": user})
		case "/auth/logout":
			if r.Method != http.MethodPost || !sameOrigin(r) {
				authError(w, http.StatusForbidden, "request refused")
				return
			}
			setCookie(w, r, "", -1)
			authJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			next.ServeHTTP(&privateCache{ResponseWriter: w}, r)
		}
	})
}
