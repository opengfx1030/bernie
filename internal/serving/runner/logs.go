package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
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
