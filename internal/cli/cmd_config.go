package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/opengfx1030/rdnactl/internal/config"
)

func init() {
	cmd := &Command{Name: "config", Summary: "Show or change user defaults (config.yaml)"}

	show := &Command{Name: "show", Summary: "Show the config file"}
	show.Run = func(c *Context, args []string) error {
		if _, err := Parse(c.Flags(show), args); err != nil {
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

	set := &Command{Name: "set", Summary: "Set one key (venv, mode, engine, hf_home, set)",
		Usage: "<KEY> <VALUE>   (set: KEY=VALUE adds or replaces an override, -KEY removes it, \"\" clears)"}
	set.Run = func(c *Context, args []string) error {
		pos, err := Parse(c.Flags(set), args)
		if err != nil {
			return err
		}
		if len(pos) != 2 {
			return Usagef("config set needs KEY and VALUE")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := cfg.SetKey(pos[0], pos[1]); err != nil {
			return Usagef("%v", err)
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

	cmd.Subs = []*Command{show, set}
	Register(cmd)
}
