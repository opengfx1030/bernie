package setup

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gitBlobID is the name of a small file in the cache of the Hub: the sha1
// of "blob <size>\0" and the content.
func gitBlobID(b []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// hubServer is a local stand-in for the Hub: the model info and the files.
type hubServer struct {
	files   map[string][]byte
	lfs     map[string]bool
	corrupt map[string]bool // the server sends other bytes than it announces
	sha     string
	ranges  []string
	noRange bool
}

// serve starts the server and returns it with its endpoint.
func (s *hubServer) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/", func(w http.ResponseWriter, r *http.Request) {
		repo := strings.TrimPrefix(r.URL.Path, "/api/models/")
		siblings := []map[string]any{}
		for path, content := range s.files {
			if s.lfs[path] {
				siblings = append(siblings, map[string]any{
					"rfilename": path, "size": len(content),
					"lfs": map[string]any{"sha256": sha256hex(content), "size": len(content)},
				})
				continue
			}
			siblings = append(siblings, map[string]any{
				"rfilename": path, "blobId": gitBlobID(content), "size": len(content),
			})
		}
		json.NewEncoder(w).Encode(map[string]any{"id": repo, "sha": s.sha, "siblings": siblings})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		i := strings.Index(r.URL.Path, "/resolve/")
		if i < 0 {
			http.NotFound(w, r)
			return
		}
		rest := r.URL.Path[i+len("/resolve/"):]
		j := strings.Index(rest, "/")
		if j < 0 {
			http.NotFound(w, r)
			return
		}
		path := rest[j+1:]
		content, ok := s.files[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if s.corrupt[path] {
			content = append([]byte{}, content...)
			content[len(content)-1] = 'x'
		}
		rng := r.Header.Get("Range")
		s.ranges = append(s.ranges, rng)
		if rng != "" && !s.noRange {
			var start int
			fmt.Sscanf(rng, "bytes=%d-", &start)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(content)-1, len(content)))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(content[start:])
			return
		}
		w.Write(content)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// testRepo is a repo with one small file and one LFS file.
func testRepo() *hubServer {
	return &hubServer{
		files: map[string][]byte{
			"config.json":       []byte(`{"model_type":"llama"}`),
			"model.safetensors": bytes.Repeat([]byte("abcdefgh"), 4096),
		},
		lfs: map[string]bool{"model.safetensors": true},
		sha: "0123456789abcdef0123456789abcdef01234567",
	}
}

// A fresh download fills the cache the way the Hub expects it: the blobs,
// the snapshot with its symlinks and the ref.
func TestDownloadModel(t *testing.T) {
	s := testRepo()
	srv := s.serve(t)
	t.Setenv("HF_ENDPOINT", srv.URL)
	hub := t.TempDir()
	var log bytes.Buffer
	if err := downloadModel(context.Background(), newHubClient(),
		&Download{Repo: "org/model", HubCache: hub}, &log); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(hub, "models--org--model")
	for name, want := range s.files {
		blob := filepath.Join(dir, "blobs", blobNameOf(s, name))
		b, err := os.ReadFile(blob)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(b, want) {
			t.Errorf("%s: content differs", name)
		}
		link := filepath.Join(dir, "snapshots", s.sha, name)
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if target != filepath.Join("..", "..", "blobs", blobNameOf(s, name)) {
			t.Errorf("%s: link = %q", name, target)
		}
	}
	ref, err := os.ReadFile(filepath.Join(dir, "refs", "main"))
	if err != nil || string(ref) != s.sha {
		t.Errorf("refs/main = %q (%v)", ref, err)
	}
	if !strings.Contains(log.String(), "model.download: done") {
		t.Errorf("no summary in the log:\n%s", log.String())
	}
}

// blobNameOf is the name of a file in the blobs directory.
func blobNameOf(s *hubServer, path string) string {
	content := s.files[path]
	if s.lfs[path] {
		return sha256hex(content)
	}
	return gitBlobID(content)
}

// An interrupted download continues: the server gets a Range request and
// the file is complete afterwards.
func TestDownloadModelResumes(t *testing.T) {
	s := testRepo()
	srv := s.serve(t)
	t.Setenv("HF_ENDPOINT", srv.URL)
	hub := t.TempDir()
	dir := filepath.Join(hub, "models--org--model", "blobs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(dir, sha256hex(s.files["model.safetensors"])+".incomplete")
	half := s.files["model.safetensors"][:1000]
	if err := os.WriteFile(part, half, 0o644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err := downloadModel(context.Background(), newHubClient(),
		&Download{Repo: "org/model", HubCache: hub}, &log); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range s.ranges {
		if r == "bytes=1000-" {
			found = true
		}
	}
	if !found {
		t.Errorf("no Range request: %v", s.ranges)
	}
	blob := filepath.Join(dir, sha256hex(s.files["model.safetensors"]))
	b, err := os.ReadFile(blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, s.files["model.safetensors"]) {
		t.Error("the file is not complete")
	}
	if _, err := os.Stat(part); err == nil {
		t.Error("the partial file is still there")
	}
	if !strings.Contains(log.String(), "continues") {
		t.Errorf("no resume in the log:\n%s", log.String())
	}
}

// A server that ignores the Range request starts the file over.
func TestDownloadModelRestarts(t *testing.T) {
	s := testRepo()
	s.noRange = true
	srv := s.serve(t)
	t.Setenv("HF_ENDPOINT", srv.URL)
	hub := t.TempDir()
	dir := filepath.Join(hub, "models--org--model", "blobs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(dir, sha256hex(s.files["model.safetensors"])+".incomplete")
	if err := os.WriteFile(part, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := downloadModel(context.Background(), newHubClient(),
		&Download{Repo: "org/model", HubCache: hub}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, sha256hex(s.files["model.safetensors"])))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, s.files["model.safetensors"]) {
		t.Error("the file is not complete")
	}
}

// A file that does not match its checksum is refused.
func TestDownloadModelChecksum(t *testing.T) {
	s := testRepo()
	s.corrupt = map[string]bool{"config.json": true}
	srv := s.serve(t)
	t.Setenv("HF_ENDPOINT", srv.URL)
	hub := t.TempDir()
	err := downloadModel(context.Background(), newHubClient(),
		&Download{Repo: "org/model", HubCache: hub}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("err = %v, want a checksum error", err)
	}
}

// The partial bytes of the cache tell the operator that the download
// continues.
func TestPartialBytes(t *testing.T) {
	hub := t.TempDir()
	dir := filepath.Join(hub, "models--org--model", "blobs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "abc.incomplete"), make([]byte, 700), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := partialBytes(hub, "org/model"); got != 700 {
		t.Errorf("partialBytes = %d, want 700", got)
	}
	if got := partialBytes(hub, "org/other"); got != 0 {
		t.Errorf("partialBytes of another repo = %d", got)
	}
}

// The size of a download is short.
func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "0 B", 999: "999 B", 1000: "1.0 kB", 1500000: "1.5 MB", 2500000000: "2.5 GB"}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
