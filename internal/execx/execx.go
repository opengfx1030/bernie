// Package execx runs external tools (rocminfo, amd-smi, docker, python...)
// with a timeout and captured output. Probes must never hang the CLI.
package execx

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// Result is the outcome of one command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	// Missing is true when the binary is not on PATH.
	Missing bool
}

// OK reports a zero exit with the binary present.
func (r Result) OK() bool { return !r.Missing && r.ExitCode == 0 }

// DefaultTimeout bounds probe commands.
const DefaultTimeout = 15 * time.Second

// Run executes name with args, bounded by timeout (DefaultTimeout if 0).
// A non-zero exit is not an error; err is only set when the process could
// not run or timed out.
func Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error) {
	return RunEnv(ctx, timeout, nil, name, args...)
}

// RunEnv is Run with extra KEY=VALUE environment entries appended.
func RunEnv(ctx context.Context, timeout time.Duration, env []string, name string, args ...string) (Result, error) {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return Result{Missing: true, ExitCode: -1}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	if env != nil {
		cmd.Env = append(cmd.Environ(), env...)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	res := Result{Stdout: out.String(), Stderr: errb.String()}
	if ctx.Err() == context.DeadlineExceeded {
		res.ExitCode = -1
		return res, errors.New(name + ": timed out after " + timeout.String())
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	if err != nil {
		res.ExitCode = -1
		return res, err
	}
	return res, nil
}

// Which returns the resolved path of name, or "".
func Which(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// FirstLine returns the first non-empty trimmed line of s.
func FirstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}
