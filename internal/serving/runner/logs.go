package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/support/execx"
)

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

// Get returns the record of the named run. The error wraps os.ErrNotExist when
// there is no such run.
func Get(name string) (*Record, error) { return readRecord(name) }

// OpenLog opens the whole log of a run for reading. A host run's log is the
// file named by the record (serve.log in the run directory when the record
// names none or the file is gone). A container run's log is the output of
// `<engine> logs`, run with the execx timeout. Reads from a host log fail
// with ctx.Err() once ctx is done.
func OpenLog(ctx context.Context, r Record, timeout time.Duration) (io.ReadCloser, error) {
	if r.Engine != "" && r.Container != "" {
		res, err := execx.Run(ctx, timeout, r.Engine, "logs", r.Container)
		if err != nil {
			return nil, err
		}
		if !res.OK() {
			return nil, fmt.Errorf("%s logs %s failed: %s", r.Engine, r.Container, execx.FirstLine(res.Stderr))
		}
		return io.NopCloser(io.MultiReader(strings.NewReader(res.Stdout), strings.NewReader(res.Stderr))), nil
	}
	path := r.Log
	if _, err := os.Stat(path); path == "" || err != nil {
		path = filepath.Join(RunDir(r.Name), "serve.log")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &ctxFile{f: f, ctx: ctx}, nil
}

// ctxFile is a file whose reads stop once ctx is done.
type ctxFile struct {
	f   *os.File
	ctx context.Context
}

func (c *ctxFile) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.f.Read(p)
}

func (c *ctxFile) Close() error { return c.f.Close() }
