package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/opengfx1030/rdnactl/internal/serving/recipe"
	"github.com/opengfx1030/rdnactl/internal/serving/tuning"
	"github.com/opengfx1030/rdnactl/internal/support/paths"
)

// Run states (spec §8.3).
const (
	StateRunning  = "running"
	StateStarting = "starting"
	StateExited   = "exited"
)

// StopTimeout is the SIGTERM grace period before SIGKILL.
const StopTimeout = 20 * time.Second

// Record is <state>/runs/<name>/run.json.
type Record struct {
	Name        string       `json:"name"`
	Recipe      string       `json:"recipe"`
	Mode        string       `json:"mode"`
	PID         int          `json:"pid"`
	Container   string       `json:"container"`
	Engine      string       `json:"engine"`
	Host        string       `json:"host"`
	Port        int          `json:"port"`
	StartedAt   time.Time    `json:"started_at"`
	Log         string       `json:"log"`
	Image       string       `json:"image"`
	ImageDigest string       `json:"image_digest"`
	Plan        *recipe.Plan `json:"plan"`
}

// Status is a record with its live state.
type Status struct {
	Record
	State string `json:"state"`
}

// URL is the base URL of the run's server (loopback for wildcard hosts).
func (r Record) URL() string {
	h := r.Host
	switch h {
	case "", "0.0.0.0", "::", "[::]":
		h = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(h, strconv.Itoa(r.Port))
}

func recordPath(name string) string { return filepath.Join(RunDir(name), "run.json") }

func readRecord(name string) (*Record, error) {
	b, err := os.ReadFile(recordPath(name))
	if err != nil {
		return nil, err
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", recordPath(name), err)
	}
	return &r, nil
}

func writeRecord(r *Record) error {
	if _, err := paths.Ensure(RunDir(r.Name)); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := recordPath(r.Name) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, recordPath(r.Name))
}

// Start starts a plan: a detached host process in a new session, or
// `<engine> run -d`. It refuses a name that is running. With Foreground it
// waits for the process to exit (Ctrl-C stops it).
func Start(ctx context.Context, p *recipe.Plan, o StartOptions) (*Record, error) {
	if prev, err := readRecord(p.Name); err == nil {
		if st := state(ctx, prev); st != StateExited {
			return nil, fmt.Errorf("run %s is %s (pid %d %s); stop it first (rdnactl stop %s) or use --name", p.Name, st, prev.PID, prev.Container, p.Name)
		}
	}
	dir, err := paths.Ensure(RunDir(p.Name))
	if err != nil {
		return nil, err
	}
	rec := &Record{Name: p.Name, Recipe: p.Recipe, Mode: p.Mode, Host: p.Host, Port: p.Port, Plan: p,
		StartedAt: time.Now().UTC().Truncate(time.Second)}
	if p.Mode == recipe.ModeContainer {
		return startContainer(ctx, p, o, rec)
	}
	return startHost(ctx, p, o, rec, dir)
}

func startHost(ctx context.Context, p *recipe.Plan, o StartOptions, rec *Record, dir string) (*Record, error) {
	ex, err := Command(p, o)
	if err != nil {
		return nil, err
	}
	for _, c := range cacheEnv {
		if _, err := paths.Ensure(filepath.Join(CacheDir(p.Name), c[1])); err != nil {
			return nil, err
		}
	}
	if f := p.Env["PYTORCH_TUNABLEOP_FILENAME"]; f == tuning.PerUserFile() {
		if _, err := paths.Ensure(filepath.Dir(f)); err != nil {
			return nil, err
		}
	}
	rec.Log = filepath.Join(dir, "serve.log")
	logf, err := os.OpenFile(rec.Log, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	fmt.Fprintf(logf, "# rdnactl %s: %s\n", rec.StartedAt.Format(time.RFC3339), shellJoin(ex.Argv))

	cmd := exec.Command(ex.Argv[0], ex.Argv[1:]...)
	cmd.Dir = ex.Dir
	cmd.Env = mergeEnv(os.Environ(), ex.Env, ex.Unset)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out io.Writer = logf
	if o.Foreground && o.Progress != nil {
		out = io.MultiWriter(logf, o.Progress)
	}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", ex.Argv[0], err)
	}
	rec.PID = cmd.Process.Pid
	if err := writeRecord(rec); err != nil {
		_ = syscall.Kill(-rec.PID, syscall.SIGTERM)
		return nil, err
	}
	if !o.Foreground {
		return rec, cmd.Process.Release()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return rec, exitErr(err)
	case <-ctx.Done():
		_ = stopGroup(rec.PID, StopTimeout)
		<-done
		return rec, nil
	}
}

func exitErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("vLLM exited: %w", err)
}

// mergeEnv removes unset and overridden keys from base and appends add.
func mergeEnv(base, add, unset []string) []string {
	drop := map[string]bool{}
	for _, k := range unset {
		drop[k] = true
	}
	for _, kv := range add {
		k, _, _ := strings.Cut(kv, "=")
		drop[k] = true
	}
	out := make([]string, 0, len(base)+len(add))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			out = append(out, kv)
		}
	}
	return append(out, add...)
}
