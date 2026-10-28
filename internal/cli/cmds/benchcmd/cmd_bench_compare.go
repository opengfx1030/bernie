package benchcmd

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

// runCompare puts two saved results side by side.
func runCompare(c *cli.Context, cmd *cli.Command, args []string) error {
	pos, err := cli.Parse(c.Flags(cmd), args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return cli.Usagef("bench compare needs exactly two result ids")
	}
	a, err := bench.Get(pos[0])
	if err != nil {
		return err
	}
	b, err := bench.Get(pos[1])
	if err != nil {
		return err
	}
	cmp, err := suite.Compare(a, b)
	if err != nil {
		return err
	}
	return c.Emit(cmp, func(w io.Writer) { printComparison(w, cmp) })
}

// printComparison puts two results of the same workload side by side.
func printComparison(w io.Writer, cmp *suite.Comparison) {
	fmt.Fprintf(w, "%s  %s  %s\n", cmp.A.ID, cmp.A.Model, cmp.A.URL)
	fmt.Fprintf(w, "%s  %s  %s\n", cmp.B.ID, cmp.B.Model, cmp.B.URL)
	conc := make([]string, len(cmp.Workload.Concurrency))
	for i, c := range cmp.Workload.Concurrency {
		conc[i] = strconv.Itoa(c)
	}
	recall := "off"
	if cmp.Workload.Recall {
		recall = "on"
	}
	fmt.Fprintf(w, "workload: protocol %s, input %d tokens, output %d tokens, concurrency %s, recall %s\n",
		cmp.Workload.ProtocolID, cmp.Workload.InputLen, cmp.Workload.OutputLen, strings.Join(conc, ","), recall)
	if cmp.Workload.SuiteID != "" {
		fmt.Fprintf(w, "suite: %s (digest %s)\n", cmp.Workload.SuiteID, shortDigest(cmp.Workload.SuiteDigest))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, metricHelp)
	for _, cl := range cmp.A.Cells {
		fmt.Fprintf(w, "result %s\n", cmp.A.ID)
		printCell(w, &cl)
		if other := cellOf(cmp.B, cl.Group, cl.Concurrency); other != nil {
			fmt.Fprintf(w, "result %s\n", cmp.B.ID)
			printCell(w, other)
		}
	}
}

// cellOf returns the cell of a result with this group and concurrency.
func cellOf(r *bench.Result, group string, concurrency int) *bench.Cell {
	for i := range r.Cells {
		if r.Cells[i].Concurrency == concurrency && r.Cells[i].Group == group {
			return &r.Cells[i]
		}
	}
	return nil
}
