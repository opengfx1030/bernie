package inventory

import (
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/machine/topo"
)

func init() {
	cmd := &cli.Command{Name: "topo", Summary: "PCIe tree, P2P matrix and link classes", Usage: "[--bandwidth] [--venv DIR] [--check] [--recipe ID]"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		bw := fs.Bool("bandwidth", false, "measure GPU-to-GPU copy bandwidth (runs GPU copies)")
		check := fs.Bool("check", false, "print the P2P readiness checklist")
		recipeFlag := fs.String("recipe", "", "recipe whose NCCL/RCCL environment the checklist shows (default: the best fit)")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		if *check && *bw {
			return cli.Usagef("--check and --bandwidth are mutually exclusive")
		}
		if *check {
			return printChecklist(c, *recipeFlag)
		}
		t, err := app.Topo(c.Ctx, app.TopoOptions{Bandwidth: *bw, Venv: c.Venv, Progress: c.Err})
		if err != nil {
			return err
		}
		return c.Emit(t, func(w io.Writer) { printTopo(w, t) })
	}
	cli.Register(cmd)
}

// printChecklist prints the P2P readiness checklist (spec §4.5).
func printChecklist(c *cli.Context, recipeID string) error {
	rep, err := app.P2PChecklist(c.Ctx, app.P2PChecklistOptions{Recipe: recipeID})
	if err != nil {
		return err
	}
	if err := c.Emit(rep, func(w io.Writer) { printP2PChecklist(w, rep) }); err != nil {
		return err
	}
	if rep.Summary["fail"] > 0 {
		return cli.ChecksFailed("%d P2P readiness checks failed", rep.Summary["fail"])
	}
	return nil
}

var p2pMark = map[string]string{"ok": "ok  ", "warn": "WARN", "fail": "FAIL", "skip": "skip"}

func printP2PChecklist(w io.Writer, rep app.P2PChecklistReport) {
	fmt.Fprintln(w, "P2P readiness checklist")
	for _, it := range rep.Checklist {
		fmt.Fprintf(w, "  [%s] %s: %s\n", p2pMark[string(it.Status)], it.Title, it.Detail)
		if it.Fix != "" {
			fmt.Fprintf(w, "         fix: %s\n", it.Fix)
		}
		if it.Doc != "" {
			fmt.Fprintf(w, "         doc: %s\n", it.Doc)
		}
	}
	s := rep.Summary
	fmt.Fprintf(w, "\n%d ok, %d warn, %d fail, %d skip\n", s["ok"], s["warn"], s["fail"], s["skip"])
	switch {
	case s["fail"] > 0:
		fmt.Fprintln(w, "not ready: a failed item blocks peer-to-peer copies")
	case s["ok"] == 0 && s["warn"] == 0:
		fmt.Fprintln(w, "nothing to check on this host")
	default:
		fmt.Fprintln(w, "ready: the GPUs can copy peer to peer")
	}
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
