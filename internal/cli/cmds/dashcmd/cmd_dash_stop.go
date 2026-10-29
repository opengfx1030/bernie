package dashcmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/opengfx1030/bernie/internal/app/dashsvc"
	"github.com/opengfx1030/bernie/internal/cli"
)

// runStop implements `bernie dash stop`: SIGTERM the running dashboard
// service, then SIGKILL if it has not exited in five seconds. Idempotent:
// a stop with no running service is not an error.
func runStop(c *cli.Context, args []string) error {
	fs := c.Flags(cmdDash)
	_, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if len(args) > 0 && !isFlag(args[0]) {
		return cli.Usagef("dash stop takes no positional argument (got %q)", args[0])
	}
	if err := dashsvc.Stop(c.Ctx); err != nil {
		if errors.Is(err, dashsvc.ErrNotRunning) {
			return c.Emit(map[string]any{"state": "stopped"}, func(w io.Writer) {
				fmt.Fprintln(w, "bernie dash: not running")
			})
		}
		return err
	}
	return c.Emit(map[string]any{"state": "stopped"}, func(w io.Writer) {
		fmt.Fprintln(w, "bernie dash: stopped")
	})
}
