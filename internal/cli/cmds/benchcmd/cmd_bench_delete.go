package benchcmd

import (
	"fmt"
	"io"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
)

// benchDeleteResult is the document of `bernie bench delete --json`: the
// id that was removed and the path of the saved result file.
type benchDeleteResult struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Note string `json:"note,omitempty"`
}

// runBenchDelete removes a saved benchmark result. A saved result is the
// evidence of a measurement, so the tool asks for --confirm before it
// removes anything (R30).
func runBenchDelete(c *cli.Context, cmd *cli.Command, args []string) error {
	fs := c.Flags(cmd)
	confirm := fs.Bool("confirm", false, "delete the saved result (without this the command prints the document and exits 0)")
	pos, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return cli.Usagef("bench delete needs exactly one result id")
	}
	if !*confirm {
		// The dry form prints the document the operator is about to
		// remove, so a stray invocation never deletes anything.
		return c.Emit(benchDeleteResult{ID: pos[0], Note: "pass --confirm to remove this result"},
			func(w io.Writer) { fmt.Fprintf(w, "result %s\n  pass --confirm to remove it\n", pos[0]) })
	}
	id, err := app.BenchDelete(pos[0])
	if err != nil {
		return err
	}
	return c.Emit(benchDeleteResult{ID: id, Path: "state/bench/" + id + ".json", Note: "saved result removed"},
		func(w io.Writer) { fmt.Fprintf(w, "removed %s\n", id) })
}

// benchDeleteCommand is the `bench delete` subcommand.
func benchDeleteCommand() *cli.Command {
	deleteCmd := &cli.Command{Name: "delete", Summary: "Delete a saved bench result: pass --confirm to actually remove it.",
		Usage: "<id> [--confirm]"}
	deleteCmd.Run = func(c *cli.Context, args []string) error { return runBenchDelete(c, deleteCmd, args) }
	return deleteCmd
}
