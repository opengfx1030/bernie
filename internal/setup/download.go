package setup

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// downloadFailure says why a step failed in words, with the command that
// runs it again. A network failure is named as such: the files are not
// broken, the connection is.
func downloadFailure(err error, step string) string {
	if err == nil {
		return ""
	}
	if isNetworkError(err) {
		return fmt.Sprintf("no network: the host did not answer (%s). Check the connection, then run: bernie setup apply --step %s", err, step)
	}
	return err.Error()
}

// isNetworkError reports whether the failure is the network.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	var opErr *net.OpError
	if errors.As(err, &dnsErr) || errors.As(err, &opErr) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"no such host", "connection refused", "connection reset",
		"i/o timeout", "network is unreachable", "no route to host", "tls handshake",
		"temporary failure in name resolution"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// Download is the model download of the model.download step.
type Download struct {
	Repo     string `json:"repo"`
	HubCache string `json:"hub_cache"`
	// ResumeBytes is the size of the partial download in the cache: the
	// download continues it instead of starting over.
	ResumeBytes int64 `json:"resume_bytes"`
}

// downloadModel puts the files of a model repo in the Hub cache. A file
// that is complete is skipped. A partial file continues with a Range
// request. The checksum of every file is checked before the file is moved
// in place.
func downloadModel(ctx context.Context, c *hubClient, d *Download, log io.Writer) error {
	repo, err := c.repo(ctx, d.Repo)
	if err != nil {
		return err
	}
	dir := repoDir(d.HubCache, d.Repo)
	snap := filepath.Join(dir, "snapshots", repo.SHA)
	for _, p := range []string{filepath.Join(dir, "blobs"), snap} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}
	var total int64
	for _, f := range repo.Files {
		total += f.Size
	}
	pr := &progress{log: log, total: total, start: time.Now(), last: time.Now()}
	for _, f := range repo.Files {
		if err := downloadFile(ctx, c, dir, snap, repo, f, pr); err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
	}
	pr.print(true)
	ref := filepath.Join(dir, "refs", "main")
	if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ref, []byte(repo.SHA), 0o644)
}

// downloadFile puts one file of a repo in the cache: the content in
// blobs/<name> and a symlink in snapshots/<commit>/<path>.
func downloadFile(ctx context.Context, c *hubClient, dir, snap string, repo hubRepo, f hubFile, pr *progress) error {
	blob := filepath.Join(dir, "blobs", f.blobName())
	if st, err := os.Stat(blob); err == nil && (f.Size == 0 || st.Size() == f.Size) {
		pr.add(f.Size)
		return linkSnapshot(snap, f, blob)
	}
	part := blob + ".incomplete"
	if _, err := c.fetch(ctx, c.fileURL(repo, f.Path), part, f, pr); err != nil {
		return err
	}
	if err := os.Rename(part, blob); err != nil {
		return err
	}
	return linkSnapshot(snap, f, blob)
}

// fetch downloads a file into part. A partial file continues with a Range
// request; a server that ignores the Range starts the file over. The
// checksum of the content is checked before the file is complete.
func (c *hubClient) fetch(ctx context.Context, url, part string, f hubFile, pr *progress) (int64, error) {
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	if f.Size > 0 && have == f.Size {
		return have, verify(part, f)
	}
	req, err := c.newRequest(ctx, url)
	if err != nil {
		return 0, err
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
		fmt.Fprintf(pr.log, "model.download: %s continues at %s of %s\n",
			f.Path, humanBytes(have), humanBytes(f.Size))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusOK:
		flags |= os.O_TRUNC
		have = 0
	case http.StatusPartialContent:
		flags |= os.O_APPEND
	default:
		return 0, fmt.Errorf("%s: %s", url, resp.Status)
	}
	out, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	sum := newChecksum(f)
	if have > 0 {
		// the checksum covers the bytes that are already there
		old, err := os.Open(part)
		if err != nil {
			return 0, err
		}
		_, err = io.Copy(sum, old)
		old.Close()
		if err != nil {
			return 0, err
		}
		// they are part of the download too
		pr.add(have)
	}
	written, err := io.Copy(io.MultiWriter(out, sum, pr), resp.Body)
	if err != nil {
		return 0, err
	}
	total := have + written
	if f.Size > 0 && total != f.Size {
		return 0, fmt.Errorf("got %d bytes, want %d", total, f.Size)
	}
	if got := sum.sum(); got != f.blobName() {
		return 0, fmt.Errorf("checksum %s does not match %s", got, f.blobName())
	}
	return total, nil
}

// verify checks the checksum of a file that is already complete.
func verify(path string, f hubFile) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := newChecksum(f)
	if _, err := sum.Write(b); err != nil {
		return err
	}
	if got := sum.sum(); got != f.blobName() {
		return fmt.Errorf("checksum %s does not match %s", got, f.blobName())
	}
	return nil
}

// linkSnapshot points the entry of a file in the snapshot at its blob.
func linkSnapshot(snap string, f hubFile, blob string) error {
	target := filepath.Join(snap, f.Path)
	if _, err := os.Lstat(target); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rel, err := filepath.Rel(filepath.Dir(target), blob)
	if err != nil {
		return err
	}
	return os.Symlink(rel, target)
}

// checksum hashes the content of a file: the sha256 of an LFS file, the
// git blob id of a small file.
type checksum struct {
	sha256 hash.Hash
	sha1   hash.Hash
	lfs    bool
}

func newChecksum(f hubFile) *checksum {
	c := &checksum{sha256: sha256.New(), sha1: sha1.New(), lfs: f.SHA256 != ""}
	fmt.Fprintf(c.sha1, "blob %d\x00", f.Size)
	return c
}

func (c *checksum) Write(p []byte) (int, error) {
	c.sha256.Write(p)
	c.sha1.Write(p)
	return len(p), nil
}

// sum is the checksum of the content.
func (c *checksum) sum() string {
	if c.lfs {
		return hex.EncodeToString(c.sha256.Sum(nil))
	}
	return hex.EncodeToString(c.sha1.Sum(nil))
}

// progress counts the bytes of a download and prints one line every two
// seconds, and one at the end.
type progress struct {
	log   io.Writer
	total int64
	done  int64
	start time.Time
	last  time.Time
}

// Write counts the bytes of the download.
func (p *progress) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	p.print(false)
	return len(b), nil
}

// add counts bytes that the download did not fetch now: the bytes of a
// file that is already in the cache, or the bytes of a file that continue
// an earlier download.
func (p *progress) add(n int64) {
	p.done += n
	p.print(false)
}

// print writes the progress line when it is time.
func (p *progress) print(final bool) {
	now := time.Now()
	if !final && now.Sub(p.last) < 2*time.Second {
		return
	}
	p.last = now
	elapsed := now.Sub(p.start).Seconds()
	var speed int64
	if elapsed > 0 {
		speed = int64(float64(p.done) / elapsed)
	}
	if final {
		fmt.Fprintf(p.log, "model.download: done, %s in %s (%s/s)\n",
			humanBytes(p.done), now.Sub(p.start).Round(time.Second), humanBytes(speed))
		return
	}
	pct := 0
	if p.total > 0 {
		pct = int(p.done * 100 / p.total)
	}
	fmt.Fprintf(p.log, "model.download: %s of %s (%d%%) at %s/s\n",
		humanBytes(p.done), humanBytes(p.total), pct, humanBytes(speed))
}

// partialBytes is the size of the partial download in the cache: the
// .incomplete files of the repo.
func partialBytes(hub, id string) int64 {
	var total int64
	filepath.WalkDir(repoDir(hub, id), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".incomplete") {
			return nil
		}
		if st, err := d.Info(); err == nil {
			total += st.Size()
		}
		return nil
	})
	return total
}

// humanBytes is a size in bytes as a short string (1.8 MB).
func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}
