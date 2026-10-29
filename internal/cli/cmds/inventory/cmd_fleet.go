package inventory

import (
	"fmt"
	"io"
	"text/tabwriter"

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
		name, url := pos[0], pos[1]
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := cfg.SetKey("fleet", "add "+name+" "+url+" "+*token); err != nil {
			return cli.Usagef("%v", err)
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		return c.Emit(map[string]any{"path": config.Path(), "entry": fleetForJSON(cfg, name)},
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
		name := pos[0]
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.RemoveFleet(name) {
			return cli.Usagef("no fleet entry named %q", name)
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		return c.Emit(map[string]any{"path": config.Path(), "removed": name},
			func(w io.Writer) {
				fmt.Fprintf(w, "forgot %s\n", name)
			})
	}

	ls := &cli.Command{Name: "ls", Summary: "List the configured fleet hosts (names and URLs)."}
	ls.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(ls), args); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		// JSON carries every field except the token; the human output
		// prints name and URL only (the token never leaves the file).
		entries := make([]map[string]any, 0, len(cfg.Fleet))
		for _, e := range cfg.Fleet {
			entries = append(entries, fleetForJSON(cfg, e.Name))
		}
		return c.Emit(map[string]any{"path": config.Path(), "entries": entries},
			func(w io.Writer) {
				if len(cfg.Fleet) == 0 {
					fmt.Fprintln(w, "no fleet entries; add one with: bernie fleet add NAME URL")
					return
				}
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tURL")
				for _, e := range cfg.Fleet {
					fmt.Fprintf(tw, "%s\t%s\n", e.Name, e.URL)
				}
				tw.Flush()
			})
	}

	cmd.Subs = []*cli.Command{add, rm, ls}
	cli.Register(cmd)
}

// fleetForJSON returns one entry for a JSON document: name and URL, never
// the token. The dashboard reads this shape; the token stays in the file.
func fleetForJSON(cfg config.Config, name string) map[string]any {
	for _, e := range cfg.Fleet {
		if e.Name == name {
			return map[string]any{"name": e.Name, "url": e.URL}
		}
	}
	return nil
}
