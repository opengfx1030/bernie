package serving

import (
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/support/config"
)

func init() {
	cmd := &cli.Command{Name: "config", Summary: "Show or change user defaults (config.yaml)"}

	show := &cli.Command{Name: "show", Summary: "Show the config file"}
	show.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(show), args); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.Set == nil {
			cfg.Set = []string{}
		}
		doc := map[string]any{"path": config.Path(), "config": cfg}
		return c.Emit(doc, func(w io.Writer) {
			fmt.Fprintf(w, "# %s\n", config.Path())
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, kv := range cfg.Map() {
				fmt.Fprintf(tw, "%s\t%s\n", kv[0], kv[1])
			}
			tw.Flush()
		})
	}

	set := &cli.Command{Name: "set", Summary: "Set one key (venv, mode, engine, hf_home, set)",
		Usage: "<KEY> <VALUE>   (set: KEY=VALUE adds or replaces an override, -KEY removes it, \"\" clears)"}
	set.Run = func(c *cli.Context, args []string) error {
		pos, err := cli.Parse(c.Flags(set), args)
		if err != nil {
			return err
		}
		if len(pos) != 2 {
			return cli.Usagef("config set needs KEY and VALUE")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := cfg.SetKey(pos[0], pos[1]); err != nil {
			return cli.Usagef("%v", err)
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		if cfg.Set == nil {
			cfg.Set = []string{}
		}
		return c.Emit(map[string]any{"path": config.Path(), "config": cfg}, func(w io.Writer) {
			fmt.Fprintf(w, "saved %s\n", config.Path())
		})
	}

	cmd.Subs = []*cli.Command{show, set}
	cli.Register(cmd)
}
