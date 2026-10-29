package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
	"github.com/opengfx1030/bernie/internal/support/fixture"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// The demo mode of the dashboard: a captured host, a sandbox under the user
// cache and a fake serving run, so that the whole page can be tried on a
// machine with no GPU. Nothing outside the sandbox is touched: the host is
// the captured tree and every write stays in the sandbox.

// DemoEnv names the demo mode in the environment. The dashboard server and
// the page read it: the page says "Demo: nothing here is real".
const DemoEnv = "BERNIE_DEMO"

// DemoSnapshot is the captured host the demo serves.
const DemoSnapshot = "par1-cs25"

// DemoRun is the name of the fake run of the demo.
const DemoRun = "demo"

// Demo is a running demo: the sandbox, the captured host and the fake
// server the page talks to.
type Demo struct {
	Dir  string // the sandbox dir, under the user cache
	Root string // the root of the captured host
	Port int    // the port of the fake server
	stop func()
}

// Stop ends the fake server. The sandbox stays for the next demo.
func (d *Demo) Stop() {
	if d == nil || d.stop == nil {
		return
	}
	d.stop()
}

// DemoDir is the sandbox of the demo: it lives under the user cache, which
// survives a reboot (the temporary directory does not).
func DemoDir() string { return filepath.Join(paths.Cache(), "demo") }

// PrepareDemo makes the sandbox of the demo: the captured host and the
// dirs every write of the demo goes to. It returns the environment of the
// demo; the caller runs the dashboard with it, so that the host root is
// read from the environment before anything probes the host.
func PrepareDemo() ([]string, error) {
	dir := DemoDir()
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	for _, sub := range []string{"host", "state", "config", "cache", "data"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	root, err := fixture.Extract(DemoSnapshot, filepath.Join(dir, "host"))
	if err != nil {
		return nil, err
	}
	return []string{
		DemoEnv + "=1",
		"BERNIE_ROOT=" + root,
		"BERNIE_STATE_DIR=" + filepath.Join(dir, "state"),
		"BERNIE_CONFIG_DIR=" + filepath.Join(dir, "config"),
		"BERNIE_CACHE_DIR=" + filepath.Join(dir, "cache"),
		"BERNIE_DATA_DIR=" + filepath.Join(dir, "data"),
	}, nil
}

// StartDemo starts the fake serving run of the demo: the same fake server
// the tests use, attached like an outside server, so that bernie never
// stops it and the page sees a run that answers.
func StartDemo(ctx context.Context) (*Demo, error) {
	srv := vllmtest.New(vllmtest.Config{Model: "demo-model"})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	go hs.Serve(ln)
	port := ln.Addr().(*net.TCPAddr).Port
	rec, err := runner.Attach(ctx, runner.AttachOptions{
		Name:   DemoRun,
		URL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		Recipe: DemoRun,
	})
	if err != nil {
		hs.Close()
		return nil, err
	}
	// The log of the run: the fake server writes none, so the demo writes
	// the lines the Logs tab shows.
	logPath := filepath.Join(runner.RunDir(rec.Name), "serve.log")
	if err := os.WriteFile(logPath, []byte(demoLog), 0o644); err != nil {
		hs.Close()
		return nil, err
	}
	rec.Log = logPath
	if err := runner.WriteRecord(rec); err != nil {
		hs.Close()
		return nil, err
	}
	return &Demo{Dir: DemoDir(), Port: port, stop: func() { hs.Close() }}, nil
}

// demoLog is the log of the fake run of the demo.
const demoLog = `INFO demo: the demo run answers on 127.0.0.1. Nothing here is real.
INFO demo: the model is fake; the tokens are counted, not generated.
INFO demo: press Ctrl-C to stop the demo. The sandbox stays under the user cache.
`
