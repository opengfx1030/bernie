package inventory

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/support/config"
)

func init() {
	cmd := &cli.Command{Name: "fleet", Summary: "List of other bernie hosts (the Fleet tab)"}

	add := &cli.Command{
		Name:  "add",
		Usage: "<NAME> <URL> [--token T]",
		Summary: "Add a bernie host to the fleet (its dashboard URL and the " +
			"token it printed). The token is stored in the config file with " +
			"0600 and is never sent to the browser.",
	}
	add.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(add)
		token := fs.String("token", "", "dashboard token of the host (optional; read-only loopback needs none)")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) < 2 {
			return cli.Usagef("fleet add needs NAME and URL")
		}
		entry, err := app.FleetAdd(pos[0], pos[1], *token)
		if err != nil {
			return cli.Usagef("%v", err)
		}
		return c.Emit(map[string]any{"path": config.Path(), "entry": fleetEntry(entry)},
			func(w io.Writer) {
				fmt.Fprintf(w, "saved %s\n", config.Path())
			})
	}

	rm := &cli.Command{
		Name:    "rm",
		Usage:   "<NAME>",
		Summary: "Forget a bernie host from the fleet (its URL and token are dropped).",
	}
	rm.Run = func(c *cli.Context, args []string) error {
		pos, err := cli.Parse(c.Flags(rm), args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return cli.Usagef("fleet rm needs NAME")
		}
		if err := app.FleetRemove(pos[0]); err != nil {
			return cli.Usagef("%v", err)
		}
		return c.Emit(map[string]any{"path": config.Path(), "removed": pos[0]},
			func(w io.Writer) {
				fmt.Fprintf(w, "forgot %s\n", pos[0])
			})
	}

	ls := &cli.Command{Name: "ls", Summary: "List the fleet hosts and what they are doing."}
	ls.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(ls), args); err != nil {
			return err
		}
		doc, err := app.Fleet(c.Ctx)
		if err != nil {
			return err
		}
		return c.Emit(doc, func(w io.Writer) {
			if len(doc.Hosts) <= 1 {
				fmt.Fprintln(w, "no fleet entries; add one with: bernie fleet add NAME URL")
				return
			}
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tURL\tSTATE\tMODEL\tTOK/S")
			for _, h := range doc.Hosts {
				url := h.URL
				if h.Self {
					url = "this box"
				}
				state := h.State
				if !h.Reachable {
					state = "unreachable"
				}
				tok := "-"
				if h.GenTokS != nil {
					tok = fmt.Sprintf("%.1f", *h.GenTokS)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", h.Name, url, state, dashIfEmpty(h.Recipe), tok)
			}
			tw.Flush()
		})
	}

	cmd.Subs = []*cli.Command{add, rm, ls}
	cli.Register(cmd)
}

// fleetEntry returns one entry for a JSON document: name and URL, never
// the token. The dashboard reads this shape; the token stays in the file.
func fleetEntry(e config.FleetEntry) map[string]any {
	return map[string]any{"name": e.Name, "url": e.URL}
}

// dashIfEmpty writes a dash for a missing value.
func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
