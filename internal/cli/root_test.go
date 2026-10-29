package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/support/hostroot"
)

// runRoot runs a small command tree whose leaf reports the root it got.
func runRoot(t *testing.T, args ...string) (string, string) {
	t.Helper()
	var leaf *Command
	leaf = &Command{Name: "leaf", Run: func(c *Context, args []string) error {
		fs := c.Flags(leaf)
		pos, err := Parse(fs, args)
		if err != nil {
			return err
		}
		return c.Emit(map[string]any{"root": c.Root, "pos": pos}, func(w io.Writer) {
			fmt.Fprintf(w, "root=%q pos=%v\n", c.Root, pos)
		})
	}}
	root := &Command{Name: "bernie", Subs: []*Command{{Name: "grp", Subs: []*Command{leaf}}}}
	var out, errb bytes.Buffer
	run(context.Background(), &out, &errb, root, args)
	return out.String(), errb.String()
}

func TestRootFlagBeforeTheCommand(t *testing.T) {
	t.Setenv(hostroot.EnvVar, "")
	out, _ := runRoot(t, "--root", "/srv/host", "grp", "leaf")
	if !strings.Contains(out, "/srv/host") {
		t.Errorf("the leaf got %q, want the root /srv/host", out)
	}
	if hostroot.Root() != "/srv/host" {
		t.Errorf("the host root is %q, want /srv/host", hostroot.Root())
	}
}

func TestRootFlagInsideTheGroup(t *testing.T) {
	t.Setenv(hostroot.EnvVar, "")
	out, _ := runRoot(t, "grp", "--root=/srv/other", "leaf")
	if !strings.Contains(out, "/srv/other") {
		t.Errorf("the leaf got %q, want the root /srv/other", out)
	}
	if hostroot.Root() != "/srv/other" {
		t.Errorf("the host root is %q, want /srv/other", hostroot.Root())
	}
}

func TestRootFlagAfterTheCommand(t *testing.T) {
	t.Setenv(hostroot.EnvVar, "")
	out, _ := runRoot(t, "grp", "leaf", "--root", "/srv/third")
	if !strings.Contains(out, "/srv/third") {
		t.Errorf("the leaf got %q, want the root /srv/third", out)
	}
}

func TestRootFlagNeedsADirectory(t *testing.T) {
	t.Setenv(hostroot.EnvVar, "")
	_, err := runRoot(t, "--root")
	if !strings.Contains(err, "--root needs a directory") {
		t.Errorf("the error is %q, want the usage error", err)
	}
}
