package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/opengfx1030/rdnactl/dashboard"
)

func init() {
	cmd := &Command{
		Name:    "dash",
		Summary: "Start the web dashboard",
		Usage:   "[--listen ADDR] [--token T]",
	}
	cmd.Run = func(c *Context, args []string) error {
		fs := c.Flags(cmd)
		listen := fs.String("listen", dashboard.DefaultListen, "listen address; a non-loopback address requires the token")
		token := fs.String("token", "", "access token (default $RDNACTL_DASH_TOKEN; random when the address is not loopback)")
		pos, err := Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) > 0 {
			return Usagef("unexpected argument %q", pos[0])
		}
		if *token == "" {
			*token = os.Getenv("RDNACTL_DASH_TOKEN")
		}
		if *token == "" && !dashboard.IsLoopback(*listen) {
			*token = dashboard.NewToken()
		}
		return dashboard.Serve(c.Ctx, dashboard.Options{
			Listen: *listen,
			Token:  *token,
			Log:    c.Err,
			Ready: func(url string) {
				_ = c.Emit(map[string]any{"url": url, "listen": *listen, "token_required": *token != ""}, func(w io.Writer) {
					fmt.Fprintf(w, "rdnactl dashboard: %s\n", url)
					if *token != "" {
						fmt.Fprintln(w, "API clients: send the header \"Authorization: Bearer <token>\".")
					}
					fmt.Fprintln(w, "Press Ctrl-C to stop.")
				})
			},
		})
	}
	Register(cmd)
}
