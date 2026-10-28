package inventory

import (
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/support/config"
)

func init() {
	cmd := &cli.Command{Name: "topo", Summary: "PCIe tree, P2P matrix and link classes", Usage: "[--bandwidth] [--venv DIR]"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		bw := fs.Bool("bandwidth", false, "measure GPU-to-GPU copy bandwidth (runs GPU copies)")
		venvFlag := fs.String("venv", "", "venv for the torch peer-copy probe (when rocm-bandwidth-test is missing)")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		gpus, err := gpu.List("/")
		if err != nil {
			return err
		}
		t, err := topo.Discover("/", gpus)
		if err != nil {
			return err
		}
		if *bw {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			c.Logf("measuring bandwidth...")
			if err := topo.MeasureBandwidth(c.Ctx, &t, cfg.ResolveVenv(*venvFlag)); err != nil {
				return err
			}
		}
		return c.Emit(t, func(w io.Writer) { printTopo(w, t) })
	}
	cli.Register(cmd)
}

func printTopo(w io.Writer, t topo.Topology) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "IDX\tPCI\tROOT\tROOT PORT\tSWITCH")
	for _, n := range t.Nodes {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", n.HIPIndex, n.PCI, n.Root, cli.OrDash(n.RootPort), cli.OrDash(n.Switch))
	}
	tw.Flush()

	matrix := func(title string, cell func(p topo.Pair) string) {
		fmt.Fprintf(w, "\n%s\n", title)
		tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', tabwriter.AlignRight)
		head := []string{""}
		for _, n := range t.Nodes {
			head = append(head, fmt.Sprint(n.HIPIndex))
		}
		fmt.Fprintln(tw, strings.Join(head, "\t")+"\t")
		for _, a := range t.Nodes {
			row := []string{fmt.Sprint(a.HIPIndex)}
			for _, b := range t.Nodes {
				if a.HIPIndex == b.HIPIndex {
					row = append(row, "X")
					continue
				}
				p, _ := t.PairOf(a.HIPIndex, b.HIPIndex)
				row = append(row, cell(p))
			}
			fmt.Fprintln(tw, strings.Join(row, "\t")+"\t")
		}
		tw.Flush()
	}
	matrix("link class (PIX same switch, PXB several switches, PHB host bridge, SYS across sockets)", func(p topo.Pair) string { return p.Class })
	matrix("p2p (KFD links)", func(p topo.Pair) string {
		if p.P2P {
			return "yes"
		}
		return "no"
	})
	if t.Bandwidth != "" {
		matrix("bandwidth GB/s ("+t.Bandwidth+")", func(p topo.Pair) string {
			if p.GBps == 0 {
				return "-"
			}
			return fmt.Sprintf("%.1f", p.GBps)
		})
	}
}
