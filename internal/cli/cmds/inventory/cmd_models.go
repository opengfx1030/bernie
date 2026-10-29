package inventory

import (
	"fmt"
	"io"
	"strings"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
)

func init() {
	// `bernie models` lists the ids the running server serves and the repos
	// the Hugging Face cache on this host holds (ia.md R31, spec §11).
	cmd := &cli.Command{Name: "models", Summary: "List the models this host serves and the repos its Hugging Face cache holds"}
	cmd.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(cmd), args); err != nil {
			return err
		}
		rep, err := app.Models(c.Ctx)
		if err != nil {
			return err
		}
		return c.Emit(rep, func(w io.Writer) { printModels(w, rep) })
	}
	cli.Register(cmd)
}

// printModels writes the human view: a section per live run and one per
// cached repo, in alphabetical order. The exact JSON is what `--json` emits.
func printModels(w io.Writer, rep app.ModelsReport) {
	fmt.Fprintf(w, "hub cache %s (from %s)\n", rep.HubCache, rep.HubSource)
	if rep.LiveRun != "" {
		fmt.Fprintf(w, "\n%s (%s) serves:\n", rep.LiveRun, rep.RunningURL)
	} else {
		fmt.Fprintln(w, "\nno run is serving on this host")
	}
	if rep.LiveError != "" {
		fmt.Fprintf(w, "  live: %s\n", rep.LiveError)
	}
	if len(rep.Running) == 0 {
		fmt.Fprintln(w, "  (no model ids)")
	} else {
		for _, id := range rep.Running {
			fmt.Fprintf(w, "  %s\n", id)
		}
	}
	fmt.Fprintln(w, "\ncached on this host:")
	if len(rep.Cached) == 0 {
		fmt.Fprintln(w, "  (no cached repo)")
		return
	}
	for _, m := range rep.Cached {
		sha := shortSHA(m.SnapshotSHA)
		fmt.Fprintf(w, "  %s  @%s\n", m.Repo, sha)
	}
}

// shortSHA shows the first 12 chars of a commit (the shape Hugging Face
// uses in its own URLs) so the line fits a terminal. The full commit is in
// the JSON.
func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return strings.TrimSpace(s)
}
