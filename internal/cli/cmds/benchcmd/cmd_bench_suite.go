package benchcmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

// runSuite runs a named suite and saves the result.
func runSuite(c *cli.Context, o bench.Options) error {
	s, err := suite.Get(o.Suite)
	if err != nil {
		return err
	}
	c.Logf("bench: suite %s: %d cells, about %s", s.ID, s.Cells(), estText(s.EstS()))
	r, path, err := app.BenchSuite(c.Ctx, s.ID, o, c.Err)
	if errors.Is(err, app.ErrAllFailed) {
		printBenchResult(c.Err, r)
	}
	if err != nil {
		return err
	}
	c.Logf("bench: saved %s", path)
	return c.Emit(r, func(w io.Writer) { printBenchResult(w, r) })
}

// runSuites lists the built-in suites with their digest and their cells.
func runSuites(c *cli.Context, cmd *cli.Command, args []string) error {
	if _, err := cli.Parse(c.Flags(cmd), args); err != nil {
		return err
	}
	all, err := suite.List()
	if err != nil {
		return err
	}
	return c.Emit(all, func(w io.Writer) { printSuiteList(w, all) })
}

// printSuiteList writes the table of the built-in suites.
func printSuiteList(w io.Writer, all []*suite.Info) {
	if len(all) == 0 {
		fmt.Fprintln(w, "no bench suites")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tCELLS\tREPS\tEST\tDIGEST")
	for _, s := range all {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\n", s.ID, s.Name, s.Cells(), s.Reps, estText(s.EstS()), suite.ShortDigest(s.Digest))
	}
	tw.Flush()
	for _, s := range all {
		fmt.Fprintf(w, "\n%s: %s\n", s.ID, s.Description)
		printSuiteGroups(w, s.Suite)
	}
}

// printSuiteGroups writes the groups of a suite: the lens of each group, its
// levels and how often it runs.
func printSuiteGroups(w io.Writer, s *suite.Suite) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "GROUP\tIN/OUT\tLEVELS\tREPS\tKIND\tEST")
	for _, g := range s.Groups {
		fmt.Fprintf(tw, "%s\t%d/%d\t%s\t%d\t%s\t%ds\n", g.Name, g.InputLen, g.OutputLen,
			levelsText(g.Concurrency), g.Reps, g.Kind, g.EstS)
	}
	tw.Flush()
	fmt.Fprintf(w, "recall %s, warmup %d, seed %d, compare band %.3f, steady %.2f\n",
		onOffText(s.Recall), s.Warmup, s.Seed, s.Compare.Band, s.Compare.Steady)
}

// suiteFlagConflict lists the flags that shape the cells of a run. A suite
// decides them itself, so one of them with --suite is a usage error.
func suiteFlagConflict(fs *flag.FlagSet) []string {
	var bad []string
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "input-len", "output-len", "concurrency", "recall", "warmup":
			bad = append(bad, "--"+f.Name)
		}
	})
	return bad
}

// levelsText names the concurrency levels of a group.
func levelsText(levels []int) string {
	out := make([]string, len(levels))
	for i, l := range levels {
		out[i] = strconv.Itoa(l)
	}
	return strings.Join(out, ",")
}

// onOffText names a flag in words.
func onOffText(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// estText names an estimate in seconds.
func estText(s int) string {
	if s <= 0 {
		return "–"
	}
	return strconv.Itoa(s) + "s"
}
