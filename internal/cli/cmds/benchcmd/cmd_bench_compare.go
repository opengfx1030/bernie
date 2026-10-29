package benchcmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

// runCompare puts two saved results side by side and says, for every
// metric, whether the second run is better, the same or worse.
func runCompare(c *cli.Context, cmd *cli.Command, args []string) error {
	fs := c.Flags(cmd)
	var band float64
	var markdown bool
	fs.Float64Var(&band, "band", 0, "share of a change that counts as noise (default: the band of the suite)")
	fs.BoolVar(&markdown, "markdown", false, "print the receipt as markdown, ready for a pull request")
	pos, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return cli.Usagef("bench compare needs exactly two result ids")
	}
	verdict, err := app.BenchCompare(pos[0], pos[1], band)
	if err != nil {
		return err
	}
	print := printVerdict
	if markdown {
		print = func(w io.Writer, v *suite.Verdict) { fmt.Fprint(w, suite.Receipt(v)) }
	}
	if err := c.Emit(verdict, func(w io.Writer) { print(w, verdict) }); err != nil {
		return err
	}
	return verdictExit(verdict, c.JSON)
}

// verdictExit is the exit code of a comparison: 0 when nothing is worse, 3
// when a metric is worse, 1 when the comparison could not decide. In JSON
// mode the document carries the answer, so nothing more is printed.
func verdictExit(v *suite.Verdict, jsonOut bool) error {
	switch v.ExitCode {
	case 0:
		return nil
	case 3:
		msg := fmt.Sprintf("%d metric(s) are worse than the baseline", v.Summary.Worse)
		if jsonOut {
			msg = ""
		}
		return &cli.ExitError{Code: cli.ExitChecks, Err: errors.New(msg)}
	default:
		msg := "the comparison could not decide: " + strings.Join(v.Reasons, "; ")
		if len(v.Reasons) == 0 {
			msg = "the comparison could not decide: the numbers of the two runs disagree too much"
		}
		if jsonOut {
			msg = ""
		}
		return &cli.ExitError{Code: cli.ExitFailure, Err: errors.New(msg)}
	}
}

// printVerdict writes the verdict of a comparison: the metrics of every
// cell with the numbers of the two runs and the verdict of the change.
func printVerdict(w io.Writer, v *suite.Verdict) {
	fmt.Fprintf(w, "%s  %s\n", v.A.ID, v.A.Model)
	fmt.Fprintf(w, "%s  %s\n", v.B.ID, v.B.Model)
	if v.Workload.SuiteID != "" {
		fmt.Fprintf(w, "suite %s, band %.3f, steady %.2f\n", v.Workload.SuiteID, v.Workload.CompareBand, v.Workload.CompareSteady)
	}
	fmt.Fprintln(w)
	for _, cv := range v.Cells {
		fmt.Fprintf(w, "cell %s\n", suite.CellLabel(cv.Group, cv.Concurrency))
		fmt.Fprintln(w, cellRow("metric", "base", "cand", "delta %", "band", "verdict"))
		for _, m := range cv.Metrics {
			fmt.Fprintln(w, cellRow(m.Label, suite.RateText(m.Base), suite.RateText(m.Cand),
				fmt.Sprintf("%+.1f", m.DeltaPct), fmt.Sprintf("%.3f", m.Band), m.Verdict))
		}
	}
	fmt.Fprintf(w, "\n%d better, %d same, %d worse, %d noisy; recall %s\n",
		v.Summary.Better, v.Summary.Same, v.Summary.Worse, v.Summary.Noisy, suite.RecallVerdictText(v))
	if !v.Valid {
		for _, r := range v.Reasons {
			fmt.Fprintf(w, "no verdict: %s\n", r)
		}
	}
}
