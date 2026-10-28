package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/opengfx1030/rdnactl/internal/config"
	"github.com/opengfx1030/rdnactl/internal/paths"
	"github.com/opengfx1030/rdnactl/internal/recipe"
	"github.com/opengfx1030/rdnactl/internal/tuning"
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

func startContainer(ctx context.Context, p *recipe.Plan, o StartOptions, rec *Record) (*Record, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	eng, err := resolveEngine(ctx, o.Engine, cfg)
	if err != nil {
		return nil, err
	}
	progress := o.Progress
	if progress == nil {
		progress = io.Discard
	}
	if p.Pull && !o.NoPull {
		fmt.Fprintf(progress, "%s pull %s\n", eng.Name, p.Image)
		pull := exec.CommandContext(ctx, eng.Name, "pull", p.Image)
		pull.Stdout, pull.Stderr = progress, progress
		if err := pull.Run(); err != nil {
			return nil, fmt.Errorf("%s pull %s: %w (use --no-pull to start the local image)", eng.Name, p.Image, err)
		}
	}
	digest, err := engineOut(ctx, eng.Name, "image", "inspect", "--format", "{{.Id}}", p.Image)
	if err != nil {
		return nil, fmt.Errorf("image %s is not available locally: %w", p.Image, err)
	}
	name := ContainerName(p.Name)
	// A stopped container of an earlier run keeps the name: remove it.
	_, _ = engineOut(ctx, eng.Name, "rm", "-f", name)

	argv := ContainerCommand(p, eng, rowsMount(p), cfg.ResolveHFHome())
	rec.Container, rec.Engine, rec.Image, rec.ImageDigest = name, eng.Name, p.Image, digest
	rec.Log = eng.Name + " logs " + name
	if o.Foreground {
		argv = append([]string{argv[0], "run", "--rm"}, argv[3:]...) // drop -d
		if err := writeRecord(rec); err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Cancel = func() error {
			_, err := engineOut(context.Background(), eng.Name, "stop", "-t", "20", name)
			return err
		}
		cmd.Stdout, cmd.Stderr = progress, progress
		return rec, exitErr(cmd.Run())
	}
	if _, err := engineOut(ctx, argv[0], argv[1:]...); err != nil {
		return nil, err
	}
	return rec, writeRecord(rec)
}

func engineOut(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", name, args[0], msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// List returns every recorded run with its state, oldest first.
func List(ctx context.Context) ([]Status, error) {
	dirs, err := filepath.Glob(filepath.Join(paths.State(), "runs", "*", "run.json"))
	if err != nil {
		return nil, err
	}
	out := []Status{}
	for _, f := range dirs {
		r, err := readRecord(filepath.Base(filepath.Dir(f)))
		if err != nil {
			continue
		}
		out = append(out, Status{Record: *r, State: state(ctx, r)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out, nil
}

// state is running (/health answers), starting (alive, not healthy) or exited.
func state(ctx context.Context, r *Record) string {
	alive := false
	if r.Mode == recipe.ModeContainer {
		s, err := engineOut(ctx, r.Engine, "inspect", "--format", "{{.State.Running}}", r.Container)
		alive = err == nil && s == "true"
	} else {
		alive = processAlive(r.PID)
	}
	if !alive {
		return StateExited
	}
	if healthy(ctx, r.URL()) {
		return StateRunning
	}
	return StateStarting
}

func healthy(ctx context.Context, base string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// processAlive reports a live process; on Linux it also checks that the
// pid still runs vLLM (pids are reused).
func processAlive(pid int) bool {
	if pid <= 0 || syscall.Kill(pid, 0) != nil && !errors.Is(syscall.Kill(pid, 0), syscall.EPERM) {
		return false
	}
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		return bytes.Contains(b, []byte("vllm"))
	}
	if st, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil && bytes.Contains(st, []byte(") Z ")) {
		return false
	}
	return true
}

// resolveName returns name, or the only run that is not exited.
func resolveName(ctx context.Context, name string) (*Record, error) {
	if name != "" {
		r, err := readRecord(name)
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no run named %s", name)
		}
		return r, err
	}
	runs, err := List(ctx)
	if err != nil {
		return nil, err
	}
	var live []Status
	for _, s := range runs {
		if s.State != StateExited {
			live = append(live, s)
		}
	}
	switch len(live) {
	case 1:
		return &live[0].Record, nil
	case 0:
		if len(runs) == 1 {
			return &runs[0].Record, nil
		}
		return nil, fmt.Errorf("no running run; give a NAME (rdnactl ps)")
	}
	names := make([]string, len(live))
	for i, s := range live {
		names[i] = s.Name
	}
	return nil, fmt.Errorf("several runs are running (%s); give a NAME", strings.Join(names, ", "))
}

// Stop stops a run (the only running one when name is ""): SIGTERM to the
// process group, 20 s, then SIGKILL; or `<engine> stop` and `rm`.
func Stop(ctx context.Context, name string) error {
	r, err := resolveName(ctx, name)
	if err != nil {
		return err
	}
	if r.Mode == recipe.ModeContainer {
		if _, err := engineOut(ctx, r.Engine, "stop", "-t", strconv.Itoa(int(StopTimeout/time.Second)), r.Container); err != nil {
			if !strings.Contains(err.Error(), "no such container") {
				return err
			}
		}
		_, _ = engineOut(ctx, r.Engine, "rm", "-f", r.Container)
		return nil
	}
	if !processAlive(r.PID) {
		return nil
	}
	return stopGroup(r.PID, StopTimeout)
}

func stopGroup(pid int, grace time.Duration) error {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("SIGTERM process group %d: %w", pid, err)
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pid, 0) != nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("SIGKILL process group %d: %w", pid, err)
	}
	return nil
}

// Logs writes the last tail lines of a run's log (all when tail <= 0) and
// with follow keeps writing new output until ctx ends.
func Logs(ctx context.Context, name string, follow bool, tail int, w io.Writer) error {
	r, err := resolveName(ctx, name)
	if err != nil {
		return err
	}
	if r.Mode == recipe.ModeContainer {
		args := []string{"logs"}
		if tail > 0 {
			args = append(args, "--tail", strconv.Itoa(tail))
		}
		if follow {
			args = append(args, "-f")
		}
		cmd := exec.CommandContext(ctx, r.Engine, append(args, r.Container)...)
		cmd.Stdout, cmd.Stderr = w, w
		if err := cmd.Run(); err != nil && ctx.Err() == nil {
			return fmt.Errorf("%s logs %s: %w", r.Engine, r.Container, err)
		}
		return nil
	}
	f, err := os.Open(r.Log)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := tailLines(f, tail, w); err != nil {
		return err
	}
	if !follow {
		return nil
	}
	buf := make([]byte, 32<<10)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			continue
		}
		if err != nil && err != io.EOF {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// tailLines writes the last n lines of r (all when n <= 0), leaving r at EOF.
func tailLines(r io.Reader, n int, w io.Writer) error {
	if n <= 0 {
		_, err := io.Copy(w, r)
		return err
	}
	ring := make([]string, 0, n)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		if len(ring) == n {
			ring = ring[1:]
		}
		ring = append(ring, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return err
	}
	for _, l := range ring {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return nil
}

// shellJoin quotes argv for display and copy-paste into a POSIX shell.
func shellJoin(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = ShellQuote(a)
	}
	return strings.Join(q, " ")
}

// ShellQuote quotes s for a POSIX shell (unchanged when it needs no quotes).
func ShellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=,+@%", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
