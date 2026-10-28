package inventory

import (
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/app"
)

func init() {
	cmd := &cli.Command{Name: "hw", Summary: "Hardware and OS inventory", Usage: ""}
	cmd.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(cmd), args); err != nil {
			return err
		}
		r, err := app.HW(c.Ctx)
		if err != nil {
			return err
		}
		if r.GPUErr != nil {
			c.Logf("warning: %v", r.GPUErr)
		}
		return c.Emit(r, func(w io.Writer) { printHW(w, r) })
	}
	cli.Register(cmd)
}

func printHW(w io.Writer, r app.HWReport) {
	h := r.Host
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "host\t%s\n", h.Hostname)
	fmt.Fprintf(tw, "os\t%s (kernel %s, pkg %s)\n", cli.OrDash(h.OS.Pretty), cli.OrDash(h.Kernel), cli.OrDash(h.PkgManager))
	if h.CPU.Threads > 0 {
		fmt.Fprintf(tw, "cpu\t%s (%d sockets, %d cores, %d threads)\n", h.CPU.Model, h.CPU.Sockets, h.CPU.Cores, h.CPU.Threads)
	} else {
		fmt.Fprintf(tw, "cpu\t-\n")
	}
	fmt.Fprintf(tw, "ram\t%s\n", gibStr(h.RAMBytes))
	fmt.Fprintf(tw, "virt\t%s\n", cli.OrDash(h.Virtualization))
	for _, e := range h.Engines {
		state := "usable"
		if !e.Usable {
			state = "not usable"
		}
		if e.Rootless {
			state += ", rootless"
		}
		fmt.Fprintf(tw, "engine\t%s %s (%s; compose: %s)\n", e.Name, e.Version, state, cli.OrDash(e.Compose))
	}
	tw.Flush()
	fmt.Fprintln(w)
	if r.Restriction != "" {
		fmt.Fprintln(w, r.Restriction)
	}
	if len(r.GPUs) == 0 {
		fmt.Fprintln(w, "no GPUs (KFD topology empty or missing)")
		return
	}
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "IDX\tGFX\tNAME\tPCI\tVRAM\tLINK\tNUMA\tCAP")
	for _, g := range r.GPUs {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\tgen%d x%d\t%d\t%.0f W\n",
			g.HIPIndex, g.GFX, g.Name, g.PCI, gibStr(g.VRAMBytes), g.Link.Gen, g.Link.Width, g.NUMANode, g.PowerCapW)
	}
	tw.Flush()
}

func gibStr(b uint64) string {
	if b == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
}
