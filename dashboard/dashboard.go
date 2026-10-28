// Package dashboard serves the bernie web UI and its JSON API (spec §11).
// It is a view over the domain packages: every endpoint calls the same code
// as the matching CLI command and returns the same JSON.
package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/opengfx1030/bernie/internal/mcp"
)

//go:embed web
var webFS embed.FS

// Options configures Serve.
type Options struct {
	// Listen is the TCP address, default "127.0.0.1:7860".
	Listen string
	// Token gates every request when non-empty. Serve generates one when the
	// address is not loopback and Token is empty.
	Token string
	// Log receives the start line and request errors. Nil discards.
	Log io.Writer
	// MCPWrite exposes the mutating MCP tools on /mcp (spec §13.5). It takes
	// effect only with a token. Serve also sets it when BERNIE_MCP_WRITE=1.
	MCPWrite bool
	// Version is the bernie version reported to MCP clients.
	Version string
	// Ready, when set, is called once the socket listens, with the URL to
	// open (including ?token= in token mode).
	Ready func(url string)
}

// DefaultListen is the loopback address of spec §11.
const DefaultListen = "127.0.0.1:7860"

// Serve listens and serves until ctx is done, then shuts down gracefully.
func Serve(ctx context.Context, o Options) error {
	if o.Listen == "" {
		o.Listen = DefaultListen
	}
	if o.Log == nil {
		o.Log = io.Discard
	}
	if o.Token == "" && !IsLoopback(o.Listen) {
		o.Token = NewToken()
	}
	o.MCPWrite = o.MCPWrite || os.Getenv("BERNIE_MCP_WRITE") == "1"
	if o.MCPWrite && o.Token == "" {
		fmt.Fprintln(o.Log, "bernie dash: /mcp stays read-only: the write tools need a dashboard token")
	}
	ln, err := net.Listen("tcp", o.Listen)
	if err != nil {
		return err
	}
	s := newServer(o)
	srv := &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: SSE streams and benchmarks run for minutes.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	url := "http://" + displayAddr(ln.Addr(), o.Listen) + "/"
	if o.Token != "" {
		url += "?token=" + o.Token
	}
	if o.Ready != nil {
		o.Ready(url)
	} else {
		fmt.Fprintf(o.Log, "bernie dash: listening on %s\n", url)
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = srv.Shutdown(shCtx)
	if errors.Is(err, context.DeadlineExceeded) {
		err = srv.Close() // open SSE streams; their contexts end with ctx anyway
	}
	if e := <-errc; e != nil && !errors.Is(e, http.ErrServerClosed) {
		return e
	}
	return err
}

// displayAddr keeps the host the operator typed (a name stays a name) and
// takes the real port from the listener (":0" picks one).
func displayAddr(a net.Addr, listen string) string {
	h, _, err := net.SplitHostPort(listen)
	_, p, perr := net.SplitHostPort(a.String())
	if err != nil || perr != nil {
		return a.String()
	}
	if h == "" || h == "0.0.0.0" || h == "::" {
		h = "localhost"
		if !IsLoopback(listen) {
			if name, err := os.Hostname(); err == nil && name != "" {
				h = name
			}
		}
	}
	return net.JoinHostPort(h, p)
}

type server struct {
	o       Options
	tele    *hub
	sampler sampler
	mcp     http.Handler // nil unless the dashboard listens on loopback
}

func newServer(o Options) *server {
	s := &server{o: o}
	if o.Listen == "" || IsLoopback(o.Listen) {
		s.mcp = mcp.New(mcp.Options{Write: o.MCPWrite && o.Token != "", Transport: "http", Version: o.Version, Log: o.Log}).Handler(o.Token)
	}
	s.tele = newHub(time.Second, s.sampleTelemetry)
	return s
}

// handler returns the full handler chain: guard (auth, CSRF) → routes, and /mcp.
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	web, _ := fs.Sub(webFS, "web")
	static := http.FileServerFS(web)
	mux.Handle("GET /{$}", withCSP(static))
	mux.Handle("GET /app.js", withCSP(static))
	mux.Handle("GET /app.css", withCSP(static))

	mux.HandleFunc("GET /api/hw", s.getHW)
	mux.HandleFunc("GET /api/topo", s.getTopo)
	mux.HandleFunc("GET /api/doctor", s.getDoctor)
	mux.HandleFunc("GET /api/recipes", s.getRecipes)
	mux.HandleFunc("GET /api/recipes/{id}/plan", s.getPlan)
	mux.HandleFunc("GET /api/runs", s.getRuns)
	mux.HandleFunc("POST /api/runs", s.postRun)
	mux.HandleFunc("DELETE /api/runs/{name}", s.deleteRun)
	mux.HandleFunc("GET /api/runs/{name}/logs", s.getLogs)
	mux.HandleFunc("GET /api/telemetry", s.getTelemetry)
	mux.HandleFunc("GET /api/bench", s.getBench)
	mux.HandleFunc("POST /api/bench", s.postBench)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpError(w, http.StatusNotFound, "no such endpoint: "+r.Method+" "+r.URL.Path)
	})
	g := newGuard(s.o.Token, mux)
	// /mcp has its own guard (spec §13.10): the dashboard cookie and the
	// X-Bernie rule do not apply to it.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			g.ServeHTTP(w, r)
		} else if s.mcp == nil {
			httpError(w, http.StatusNotFound, "/mcp is served on loopback addresses only")
		} else {
			s.mcp.ServeHTTP(w, r)
		}
	})
}

// withCSP adds the CSP to the embedded UI (the guard sets Cache-Control).
func withCSP(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// httpError writes the CLI's --json failure shape: {"error": "...", "exit_code": N}.
func httpError(w http.ResponseWriter, code int, msg string) {
	exit := 1
	if code == http.StatusBadRequest {
		exit = 2
	}
	writeJSON(w, code, map[string]any{"error": msg, "exit_code": exit})
}
