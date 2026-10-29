package dashcmd

import (
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/opengfx1030/bernie/dashboard"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
)

func init() {
	cmd := &cli.Command{
		Name:    "dash",
		Summary: "Start the web dashboard",
		Usage:   "[--listen ADDR] [--token T] [--demo]",
	}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		listen := fs.String("listen", dashboard.DefaultListen, "listen address; a non-loopback address requires the token")
		token := fs.String("token", "", "access token (default $BERNIE_DASH_TOKEN; random when the address is not loopback)")
		demo := fs.Bool("demo", false, "serve the demo: a captured host and a fake run, with no GPU")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) > 0 {
			return cli.Usagef("unexpected argument %q", pos[0])
		}
		if *token == "" {
			*token = os.Getenv("BERNIE_DASH_TOKEN")
		}
		if *token == "" && !dashboard.IsLoopback(*listen) {
			*token = dashboard.NewToken()
		}
		if *demo {
			// The demo serves a captured host from a sandbox under the user
			// cache. The host root is read from the environment before
			// anything probes the host, so the demo runs again with that
			// environment.
			if os.Getenv(app.DemoEnv) == "" {
				env, err := app.PrepareDemo()
				if err != nil {
					return err
				}
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				fmt.Fprintln(c.Err, "bernie demo: the captured host is unpacked under "+app.DemoDir()+".")
				return syscall.Exec(exe, os.Args, append(os.Environ(), env...))
			}
			d, err := app.StartDemo(c.Ctx)
			if err != nil {
				return err
			}
			defer d.Stop()
		}
		return dashboard.Serve(c.Ctx, dashboard.Options{
			Listen:  *listen,
			Token:   *token,
			Log:     c.Err,
			Version: cli.Version,
			Ready: func(url string) {
				_ = c.Emit(map[string]any{"url": url, "listen": *listen, "token_required": *token != "", "demo": *demo}, func(w io.Writer) {
					fmt.Fprintf(w, "bernie dashboard: %s\n", url)
					if *token != "" {
						fmt.Fprintln(w, "API clients: send the header \"Authorization: Bearer <token>\".")
					}
					if *demo {
						fmt.Fprintln(w, "Demo: nothing here is real.")
					}
					fmt.Fprintln(w, "Press Ctrl-C to stop.")
				})
			},
		})
	}
	cli.Register(cmd)
}
