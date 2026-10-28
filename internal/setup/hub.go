package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The Hugging Face Hub. bernie puts the files of a model repo in the Hub
// cache with the layout the hf tool uses, so vLLM finds the model in the
// cache without asking the Hub again.
//
// The download runs on the host: the container image of bernie has no
// HTTPS client and no Python, and it never downloads anything.

const (
	// hubEndpoint is the base URL of the Hub. HF_ENDPOINT moves it (a
	// mirror, a test server).
	hubEndpoint = "https://huggingface.co"
	// hubTimeout bounds one request. A big file takes a while.
	hubTimeout = 2 * time.Hour
)

// hubRepo is a model repo of the Hub.
type hubRepo struct {
	ID    string
	SHA   string // the commit the files belong to
	Files []hubFile
}

// hubFile is one file of a repo.
type hubFile struct {
	Path   string // the path inside the repo
	Size   int64
	SHA256 string // the checksum of an LFS file
	BlobID string // the git blob id of a small file
}

// blobName is the name of the file in the blobs directory of the cache.
func (f hubFile) blobName() string {
	if f.SHA256 != "" {
		return f.SHA256
	}
	return f.BlobID
}

// hubClient reads a model repo from the Hub.
type hubClient struct {
	endpoint string
	http     *http.Client
	token    string
}

// newHubClient is the client for the Hub. HF_ENDPOINT moves the endpoint,
// HF_TOKEN authenticates (a private repo).
func newHubClient() *hubClient {
	endpoint := os.Getenv("HF_ENDPOINT")
	if endpoint == "" {
		endpoint = hubEndpoint
	}
	return &hubClient{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		http:     &http.Client{Timeout: hubTimeout},
		token:    os.Getenv("HF_TOKEN"),
	}
}

// repo asks the Hub for the files of a model repo.
func (c *hubClient) repo(ctx context.Context, id string) (hubRepo, error) {
	u := fmt.Sprintf("%s/api/models/%s?blobs=true", c.endpoint, id)
	body, err := c.get(ctx, u)
	if err != nil {
		return hubRepo{}, err
	}
	var api struct {
		ID       string `json:"id"`
		SHA      string `json:"sha"`
		Siblings []struct {
			Name   string `json:"rfilename"`
			BlobID string `json:"blobId"`
			Size   int64  `json:"size"`
			LFS    struct {
				SHA256 string `json:"sha256"`
				Size   int64  `json:"size"`
			} `json:"lfs"`
		} `json:"siblings"`
	}
	if err := json.Unmarshal(body, &api); err != nil {
		return hubRepo{}, fmt.Errorf("%s: %w", u, err)
	}
	r := hubRepo{ID: api.ID, SHA: api.SHA}
	for _, s := range api.Siblings {
		f := hubFile{Path: s.Name, Size: s.Size, SHA256: s.LFS.SHA256, BlobID: s.BlobID}
		if s.LFS.SHA256 != "" {
			f.Size = s.LFS.Size
		}
		r.Files = append(r.Files, f)
	}
	return r, nil
}

// fileURL is the URL of a file of a repo at a commit.
func (c *hubClient) fileURL(repo hubRepo, path string) string {
	return fmt.Sprintf("%s/%s/resolve/%s/%s", c.endpoint, repo.ID, repo.SHA, path)
}

// get is one GET request.
func (c *hubClient) get(ctx context.Context, u string) ([]byte, error) {
	req, err := c.newRequest(ctx, u)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", u, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// newRequest is a GET request with the token and the user agent.
func (c *hubClient) newRequest(ctx context.Context, u string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "bernie")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// hubCache is the cache directory of the Hub: <HF_HOME>/hub.
func hubCache(home string) string {
	return filepath.Join(home, "hub")
}

// repoDir is the cache directory of a repo: models--<org>--<name>.
func repoDir(hub, id string) string {
	return filepath.Join(hub, "models--"+strings.ReplaceAll(id, "/", "--"))
}
