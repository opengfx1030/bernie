package serving

import (
	"fmt"
	"io"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
)

func init() {
	cmd := &cli.Command{Name: "attach",
		Summary: "Register an already running vLLM server as a run",
		Usage:   "<url|port> [--name N] [--recipe ID]"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		name := fs.String("name", "attached", "name of the run")
		recipe := fs.String("recipe", "", "recipe the server serves (optional)")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return cli.Usagef("attach needs one URL or port")
		}
		res, err := app.Attach(c.Ctx, app.AttachOptions{URL: pos[0], Name: *name, Recipe: *recipe})
		if err != nil {
			return err
		}
		return c.Emit(res, func(w io.Writer) {
			fmt.Fprintf(w, "%s\n", res.Note)
			fmt.Fprintf(w, "  stop the server yourself; bernie stop %s only forgets it\n", res.Run.Name)
		})
	}
	cli.Register(cmd)
}
