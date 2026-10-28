package dashboard

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

// cookieName holds the dashboard token after the operator visits /?token=….
const cookieName = "bernie_token"

// NewToken returns a random 128-bit hex token.
func NewToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("dashboard: crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// IsLoopback reports whether the listen address binds only to loopback.
// An empty host ("" or ":7860") binds all interfaces and is not loopback.
func IsLoopback(listen string) bool {
	h, _, err := net.SplitHostPort(listen)
	if err != nil {
		h = listen
	}
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

// guard enforces the access rules of spec §11:
//   - token mode (token != ""): every request carries the token as
//     `Authorization: Bearer <t>` or the cookie; `/?token=<t>` sets the cookie
//     and redirects to `/`. That is the only place the URL carries a token:
//     /api paths refuse one, and nothing logs a query string.
//   - every response is `Cache-Control: no-store`.
//   - loopback mode (token == ""): the Host header must name a loopback host,
//     which blocks DNS-rebinding pages from reaching the API.
//   - POST and DELETE need `X-Bernie: 1` in both modes (CSRF guard).
type guard struct {
	tokenSum [32]byte
	token    bool
	next     http.Handler
}

func newGuard(token string, next http.Handler) *guard {
	g := &guard{next: next, token: token != ""}
	if g.token {
		g.tokenSum = sha256.Sum256([]byte(token))
	}
	return g
}

// match compares in constant time; hashing first hides the token length.
func (g *guard) match(t string) bool {
	if t == "" {
		return false
	}
	s := sha256.Sum256([]byte(t))
	return subtle.ConstantTimeCompare(s[:], g.tokenSum[:]) == 1
}

func (g *guard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Query().Has("token") {
		httpError(w, http.StatusBadRequest, "the token is not accepted in the URL on /api: send Authorization: Bearer <token>")
		return
	}
	if g.token {
		if q := r.URL.Query(); r.URL.Path == "/" && q.Has("token") {
			if !g.match(q.Get("token")) {
				httpError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name: cookieName, Value: q.Get("token"), Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil,
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if !g.authorized(r) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("WWW-Authenticate", `Bearer realm="bernie"`)
			}
			httpError(w, http.StatusUnauthorized, "token required: open the URL printed by `bernie dash` (…/?token=…) or send Authorization: Bearer <token>")
			return
		}
	} else if !loopbackHost(r.Host) {
		httpError(w, http.StatusForbidden, "host header is not a loopback name")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Bernie") != "1" {
		httpError(w, http.StatusForbidden, "missing header X-Bernie: 1")
		return
	}
	g.next.ServeHTTP(w, r)
}

func (g *guard) authorized(r *http.Request) bool {
	if a := r.Header.Get("Authorization"); a != "" {
		const p = "Bearer "
		return len(a) > len(p) && strings.EqualFold(a[:len(p)], p) && g.match(a[len(p):])
	}
	c, err := r.Cookie(cookieName)
	return err == nil && g.match(c.Value)
}

func loopbackHost(hostport string) bool {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		h = hostport
	}
	return IsLoopback(net.JoinHostPort(h, "0"))
}
