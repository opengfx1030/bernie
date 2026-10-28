package dashboard

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
)

// The web tree is embedded in the binary and served from memory. Every file
// is prepared once at start-up: the body, the compressed body and the ETag.
// A file is served with `Cache-Control: no-cache` (the browser revalidates
// it and gets a 304), the fonts with `immutable` (their name holds the
// content), and the page itself with `no-store` (it names the token flow).
// The API answers `no-store` in the guard (spec §11).

// asset is one file of the web tree.
type asset struct {
	body      []byte
	gz        []byte
	etag      string
	kind      string
	immutable bool
}

// assets holds every file of the embedded web tree.
type assets struct {
	files map[string]*asset
}

// newAssets reads the tree and prepares the answers.
func newAssets(web fs.FS) *assets {
	a := &assets{files: map[string]*asset{}}
	_ = fs.WalkDir(web, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(web, p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(body)
		file := &asset{
			body: body,
			etag: `"` + hex.EncodeToString(sum[:8]) + `"`,
			kind: contentType(p),
		}
		if isText(p) {
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			_, _ = zw.Write(body)
			_ = zw.Close()
			file.gz = buf.Bytes()
		}
		// A font never changes under its name: the browser keeps it.
		file.immutable = strings.HasPrefix(p, "fonts/")
		a.files[p] = file
		return nil
	})
	return a
}

// ServeHTTP answers one file of the web tree.
func (a *assets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	file, ok := a.files[name]
	if !ok {
		httpjson.Error(w, http.StatusNotFound, "no such file: "+r.URL.Path)
		return
	}
	w.Header().Set("ETag", file.etag)
	w.Header().Set("Content-Type", file.kind)
	switch {
	case file.immutable:
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case name == "index.html":
		w.Header().Set("Cache-Control", "no-store")
	default:
		w.Header().Set("Cache-Control", "no-cache")
	}
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, file.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := file.body
	if file.gz != nil && acceptsGzip(r) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		body = file.gz
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

// acceptsGzip reports whether the client takes a compressed answer.
func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// isText reports whether a file is worth compressing.
func isText(name string) bool {
	switch {
	case strings.HasSuffix(name, ".html"), strings.HasSuffix(name, ".css"),
		strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".json"),
		strings.HasSuffix(name, ".svg"), strings.HasSuffix(name, ".txt"):
		return true
	}
	return false
}

// contentType names the type of a file.
func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".json"):
		return "application/json"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".txt"):
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

// guardWrites refuses a write that comes from another site. The page always
// carries the X-Bernie header (the CSRF guard of spec §11); this guard adds
// the browser's own word about the site: `Sec-Fetch-Site` first, the `Origin`
// header as the fallback.
func guardWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
				httpjson.Error(w, http.StatusForbidden, "the request comes from another site")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, r.Host) {
				httpjson.Error(w, http.StatusForbidden, "the request comes from another origin")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin reports whether the Origin header names the host of the request.
func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}
