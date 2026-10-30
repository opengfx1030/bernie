package serving

import (
	"fmt"
	"io"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
)

func init() {
	cmd := &cli.Command{Name: "smoke", Summary: "Check that the running server answers a request", Usage: "[--url URL] [--model NAME]"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		url := fs.String("url", "", "base URL of the server (default: the running run)")
		model := fs.String("model", "", "model name (default: the first id of /v1/models)")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		res, err := app.Smoke(c.Ctx, app.SmokeOptions{URL: *url, Model: *model})
		if err != nil {
			return err
		}
		return c.Emit(res, func(w io.Writer) {
			fmt.Fprintf(w, "server  %s is healthy\n", res.URL)
			if res.Model != "" {
				fmt.Fprintf(w, "model   %s\n", res.Model)
			}
			fmt.Fprintf(w, "answer  %s\n", res.Reply)
			fmt.Fprintf(w, "check   %s\n", res.Check)
			fmt.Fprintf(w, "ttft    %d ms\n", res.TTFTMS)
			fmt.Fprintf(w, "tok/s   %.1f\n", res.TokS)
		})
	}
	cli.Register(cmd)
}
