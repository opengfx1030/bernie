package bench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/machine/gpu"
	"github.com/opengfx1030/rdnactl/internal/support/paths"
)

// Dir is where results are stored: <state>/bench.
func Dir() string { return filepath.Join(paths.State(), "bench") }

// Save writes r to <state>/bench/<id>.json and returns the path. An empty ID
// is assigned.
func Save(r *Result) (string, error) {
	if r.ID == "" {
		r.ID = newID(r.Time.UTC(), r.Options, 0)
	}
	dir, err := paths.Ensure(Dir())
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, r.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// List returns all saved results, oldest first. Unreadable files are skipped.
func List() ([]Result, error) {
	files, err := filepath.Glob(filepath.Join(Dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(files))
	for _, f := range files {
		r, err := load(f)
		if err != nil {
			continue
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.Before(out[j].Time)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Get returns the result with this id, or the only one whose id starts with
// it.
func Get(id string) (*Result, error) {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return nil, fmt.Errorf("invalid result id %q", id)
	}
	r, err := load(filepath.Join(Dir(), id+".json"))
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	files, _ := filepath.Glob(filepath.Join(Dir(), id+"*.json"))
	switch len(files) {
	case 0:
		return nil, fmt.Errorf("no bench result %q", id)
	case 1:
		return load(files[0])
	default:
		return nil, fmt.Errorf("bench result id %q is ambiguous (%d matches)", id, len(files))
	}
}

func load(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &r, nil
}

// runRecord holds the fields of <state>/runs/<name>/run.json (spec §8.3)
// that bench needs.
type runRecord struct {
	Name   string          `json:"name"`
	Recipe string          `json:"recipe"`
	Host   string          `json:"host"`
	Port   int             `json:"port"`
	Plan   json.RawMessage `json:"plan"`
}

func runRecords(stateDir string) []runRecord {
	files, _ := filepath.Glob(filepath.Join(stateDir, "runs", "*", "run.json"))
	var out []runRecord
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var r runRecord
		if json.Unmarshal(data, &r) == nil && r.Port > 0 {
			out = append(out, r)
		}
	}
	return out
}

// DefaultURL is the URL of the only run record, else FallbackURL.
func DefaultURL() string { return defaultURL(paths.State()) }

func defaultURL(stateDir string) string {
	recs := runRecords(stateDir)
	if len(recs) != 1 {
		return FallbackURL
	}
	return "http://" + net.JoinHostPort(dialHost(recs[0].Host), strconv.Itoa(recs[0].Port))
}

// dialHost maps a listen address to an address a local client can dial.
func dialHost(h string) string {
	switch h {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	}
	return h
}

func isLoopback(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// sameEndpoint reports whether a client URL reaches the run's listen address.
func sameEndpoint(u *url.URL, r runRecord) bool {
	if u.Port() != strconv.Itoa(r.Port) {
		return false
	}
	h, rh := u.Hostname(), dialHost(r.Host)
	return h == rh || (isLoopback(h) && isLoopback(rh))
}

// gpuList is gpu.List; tests replace it.
var gpuList = gpu.List

// fingerprint records where the result came from: host name, GPUs and, when
// the URL belongs to a run, the run name, recipe and plan hash.
func fingerprint(stateDir, rawURL string) map[string]string {
	fp := map[string]string{}
	if h, err := os.Hostname(); err == nil {
		fp["host"] = h
	}
	if gpus, err := gpuList("/"); err == nil && len(gpus) > 0 {
		fp["gpus"] = gpuSummary(gpus)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fp
	}
	for _, r := range runRecords(stateDir) {
		if !sameEndpoint(u, r) {
			continue
		}
		fp["run"] = r.Name
		if r.Recipe != "" {
			fp["recipe"] = r.Recipe
		}
		if len(r.Plan) > 0 && string(r.Plan) != "null" {
			var buf bytes.Buffer
			if json.Compact(&buf, r.Plan) == nil {
				sum := sha256.Sum256(buf.Bytes())
				fp["plan_hash"] = hex.EncodeToString(sum[:])[:12]
			}
		}
		break
	}
	return fp
}

// gpuSummary groups GPUs by name and gfx target in HIP order, e.g.
// "2x AMD Radeon PRO W7800 (gfx1100), 8x AMD Radeon PRO V620 (gfx1030)".
func gpuSummary(gpus []gpu.GPU) string {
	type group struct {
		label string
		n     int
	}
	var groups []*group
	idx := map[string]*group{}
	for _, g := range gpus {
		label := g.Name
		if label == "" {
			label = g.DeviceID
		}
		if g.GFX != "" {
			label += " (" + g.GFX + ")"
		}
		gr, ok := idx[label]
		if !ok {
			gr = &group{label: label}
			idx[label] = gr
			groups = append(groups, gr)
		}
		gr.n++
	}
	parts := make([]string, len(groups))
	for i, g := range groups {
		parts[i] = fmt.Sprintf("%dx %s", g.n, g.label)
	}
	return strings.Join(parts, ", ")
}
