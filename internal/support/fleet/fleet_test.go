package fleet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/support/config"
)

// TestAllowedPaths guards the fixed list: a path that is not on the
// allowlist is refused by get.
func TestAllowedPaths(t *testing.T) {
	good := []string{"/api/hw", "/api/runs", "/api/telemetry", "/api/recipes"}
	for _, p := range good {
		if !allowedPath(p) {
			t.Errorf("%q: not allowed", p)
		}
	}
	bad := []string{"/api/doctor", "/api/jobs", "/api/bench/run", "/metrics", "/", "/anything"}
	for _, p := range bad {
		if allowedPath(p) {
			t.Errorf("%q: allowed (not on allowlist)", p)
		}
	}
}

// TestFetchOneRefusesRedirect spins up a peer that 302s to another host.
// The fetcher must answer with "redirects refused" and not follow it.
func TestFetchOneRefusesRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/", http.StatusFound)
	}))
	defer target.Close()
	row := fetchOne(context.Background(), &http.Client{Timeout: Timeout},
		config.FleetEntry{Name: "p", URL: target.URL})
	if row.Reachable {
		t.Errorf("got reachable, want unreachable")
	}
	if !strings.Contains(row.Error, "redirect") {
		t.Errorf("error %q does not mention redirect", row.Error)
	}
}

// TestFetchOneRefusesOversizeBody returns more bytes than the cap; the
// fetcher must refuse the body and report the oversize reason.
func TestFetchOneRefusesOversizeBody(t *testing.T) {
	// /api/hw on the test server returns a body larger than MaxBody.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A single big field so the body is large enough to exceed MaxBody.
		w.Header().Set("Content-Type", "application/json")
		buf := make([]byte, MaxBody+4096)
		for i := range buf {
			buf[i] = 'x'
		}
		fmt.Fprintf(w, `{"big":"%s"}`, buf)
	}))
	defer server.Close()
	row := fetchOne(context.Background(), &http.Client{Timeout: Timeout},
		config.FleetEntry{Name: "p", URL: server.URL})
	if row.Reachable {
		t.Errorf("got reachable, want unreachable")
	}
	if !strings.Contains(row.Error, "larger than") {
		t.Errorf("error %q does not mention size", row.Error)
	}
}

// TestFetchOneTimesOut spins up a server that never answers; the fetcher
// must surface a timeout error and mark the peer unreachable.
func TestFetchOneTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the per-request timeout.
		time.Sleep(Timeout + 500*time.Millisecond)
	}))
	defer server.Close()
	row := fetchOne(context.Background(), &http.Client{Timeout: Timeout},
		config.FleetEntry{Name: "p", URL: server.URL})
	if row.Reachable {
		t.Errorf("got reachable, want unreachable")
	}
	if row.Error == "" {
		t.Errorf("no error message")
	}
}

// TestFetchOneRefusesBadScheme: a configured URL with an ftp scheme is
// refused at the URL parse stage; the peer never sees the request.
func TestFetchOneRefusesBadScheme(t *testing.T) {
	row := fetchOne(context.Background(), &http.Client{Timeout: Timeout},
		config.FleetEntry{Name: "p", URL: "ftp://host"})
	if row.Reachable {
		t.Errorf("got reachable, want unreachable")
	}
	if !strings.Contains(row.Error, "scheme") {
		t.Errorf("error %q does not mention scheme", row.Error)
	}
}

// TestFetchOneSendsBearer: when the entry carries a token, the request to
// the peer carries an Authorization: Bearer header. The peer reads the
// header and answers 200.
func TestFetchOneSendsBearer(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"host":{"hostname":"peer"},"gpus":[]}`)
	}))
	defer server.Close()
	row := fetchOne(context.Background(), &http.Client{Timeout: Timeout},
		config.FleetEntry{Name: "p", URL: server.URL, Token: "secret-token"})
	if !row.Reachable {
		t.Fatalf("got unreachable: %s", row.Error)
	}
	if got != "Bearer secret-token" {
		t.Errorf("header = %q, want Bearer secret-token", got)
	}
}

// TestFetchReachableWithHWAndRuns: the happy path — the peer answers hw
// and runs, the row carries the host fields.
func TestFetchReachableWithHWAndRuns(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/hw", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"host":{"hostname":"peer"},"gpus":[{"hip_index":0,"name":"V620","vram_total":32212254720,"power_cap_w":175}]}`)
	})
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"name":"flashnext","state":"serving","recipe":"flashnext","stats":{"gen_tok_s":42.3}}]`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	rows := Fetch(context.Background(), Source{
		Entries: []config.FleetEntry{{Name: "p", URL: server.URL}},
		Self:    func(ctx context.Context) Summary { return Summary{Name: "self", Self: true} },
	})
	if len(rows) != 2 {
		t.Fatalf("len = %d, want 2", len(rows))
	}
	if !rows[0].Self {
		t.Errorf("rows[0]: not self")
	}
	if !rows[1].Reachable {
		t.Fatalf("peer: %s", rows[1].Error)
	}
	if rows[1].Recipe != "flashnext" {
		t.Errorf("recipe = %q", rows[1].Recipe)
	}
	if rows[1].State != "serving" {
		t.Errorf("state = %q", rows[1].State)
	}
	if rows[1].GenTokS == nil || *rows[1].GenTokS != 42.3 {
		t.Errorf("gen_tok_s = %v", rows[1].GenTokS)
	}
	if len(rows[1].GPU) != 1 || rows[1].GPU[0].Name != "V620" {
		t.Errorf("gpu = %+v", rows[1].GPU)
	}
	if rows[1].GPU[0].VRAMGiB == nil || *rows[1].GPU[0].VRAMGiB < 29.9 || *rows[1].GPU[0].VRAMGiB > 30.1 {
		t.Errorf("vram = %v GiB", rows[1].GPU[0].VRAMGiB)
	}
}

// TestValidate checks the host-allowlist guard.
func TestValidate(t *testing.T) {
	hosts := []string{"box-a", "box-b"}
	cases := []struct {
		url     string
		wantErr error
	}{
		{"http://box-a:7860", nil},
		{"https://box-b/", nil},
		{"http://other:7860", ErrNotConfigured},
		{"ftp://box-a", errors.New("scheme")},
	}
	for _, c := range cases {
		err := Validate(hosts, c.url)
		if c.wantErr == nil && err != nil {
			t.Errorf("%s: %v", c.url, err)
			continue
		}
		if c.wantErr == nil {
			continue
		}
		if !errors.Is(err, c.wantErr) && !strings.Contains(fmt.Sprint(err), c.wantErr.Error()) {
			t.Errorf("%s: err = %v, want %v", c.url, err, c.wantErr)
		}
	}
}
