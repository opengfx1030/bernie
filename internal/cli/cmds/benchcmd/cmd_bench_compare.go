package benchcmd

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/bench"
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
	verdict := suite.Judge(cmp, band)
	print := printVerdict
	if markdown {
		print = printReceipt
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
		fmt.Fprintf(w, "cell %s\n", cellLabel(cv.Group, cv.Concurrency))
		fmt.Fprintln(w, cellRow("metric", "base", "cand", "delta %", "band", "verdict"))
		for _, m := range cv.Metrics {
			fmt.Fprintln(w, cellRow(m.Label, rateText(m.Base), rateText(m.Cand),
				fmt.Sprintf("%+.1f", m.DeltaPct), fmt.Sprintf("%.3f", m.Band), m.Verdict))
		}
	}
	fmt.Fprintf(w, "\n%d better, %d same, %d worse, %d noisy; recall %s\n",
		v.Summary.Better, v.Summary.Same, v.Summary.Worse, v.Summary.Noisy, recallVerdictText(v))
	if !v.Valid {
		for _, r := range v.Reasons {
			fmt.Fprintf(w, "no verdict: %s\n", r)
		}
	}
}

// recallVerdictText names the recall gate of a comparison.
func recallVerdictText(v *suite.Verdict) string {
	if !v.Summary.RecallOK {
		return "failed"
	}
	return "ok"
}

// printReceipt writes the comparison as a markdown receipt: the scope of the
// two runs, their correctness, the numbers and the verdict. It is the text
// that goes into a pull request.
func printReceipt(w io.Writer, v *suite.Verdict) {
	fmt.Fprintln(w, "# Benchmark receipt")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Scope")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| | |")
	fmt.Fprintln(w, "|---|---|")
	receiptRow(w, "baseline", v.A.ID)
	receiptRow(w, "candidate", v.B.ID)
	receiptRow(w, "model", v.A.Model)
	receiptRow(w, "server", v.A.URL)
	if v.Workload.SuiteID != "" {
		receiptRow(w, "suite", v.Workload.SuiteID+" (digest "+shortDigest(v.Workload.SuiteDigest)+")")
	}
	receiptRow(w, "measured", v.B.Time.Format(time.RFC3339))
	receiptRow(w, "host", v.B.Fingerprint["host"])
	receiptRow(w, "GPUs", gpuScopeText(v.B.Fingerprint))
	receiptRow(w, "run", v.B.Fingerprint["run"])
	receiptRow(w, "recipe", v.B.Fingerprint["recipe"])
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Correctness")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| cell | recall | check | failed | warm-up | incomplete |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, cv := range v.Cells {
		base := findCell(v.A, cv.Group, cv.Concurrency)
		cand := findCell(v.B, cv.Group, cv.Concurrency)
		fmt.Fprintf(w, "| %s | %s | %s | %d | %d | %s |\n",
			mdCell(cv.Group, cv.Concurrency),
			mdRecall(base, cand),
			mdCheck(base, cand),
			mdSum(base, cand, func(c *bench.Cell) int { return c.Failed }),
			mdSum(base, cand, func(c *bench.Cell) int { return c.WarmupFailed }),
			mdIncomplete(base, cand))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Timings")
	for _, cv := range v.Cells {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "### %s\n\n", mdCell(cv.Group, cv.Concurrency))
		fmt.Fprintln(w, "| metric | baseline | candidate | delta | verdict |")
		fmt.Fprintln(w, "|---|---|---|---|---|")
		for _, m := range cv.Metrics {
			fmt.Fprintf(w, "| %s | %s | %s | %+.1f %% | %s |\n",
				mdText(m.Label), mdText(rateText(m.Base)), mdText(rateText(m.Cand)), m.DeltaPct, m.Verdict)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Verdict")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%d better, %d same, %d worse, %d noisy; recall %s.\n",
		v.Summary.Better, v.Summary.Same, v.Summary.Worse, v.Summary.Noisy, recallVerdictText(v))
	for _, r := range v.Reasons {
		fmt.Fprintf(w, "- %s\n", r)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Reproduce")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "```bash")
	fmt.Fprintf(w, "bernie bench compare %s %s --markdown\n", v.A.ID, v.B.ID)
	fmt.Fprintln(w, "```")
}

// receiptRow writes one row of the scope table. An empty value leaves the
// row out: a receipt says what it knows.
func receiptRow(w io.Writer, name, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(w, "| %s | %s |\n", name, mdText(value))
}

// gpuScopeText names the cards of a run and how wide the selection was.
func gpuScopeText(fp map[string]string) string {
	gpus := fp["gpus"]
	if gpus == "" {
		return ""
	}
	if scope := fp["gpu_scope"]; scope != "" {
		return gpus + " (" + scope + ")"
	}
	return gpus
}

// mdCell names a cell of a comparison.
func mdCell(group string, concurrency int) string {
	return mdText(cellLabel(group, concurrency))
}

// mdRecall names the verdict of the recall gate of the two runs.
func mdRecall(base, cand *bench.Cell) string {
	var b, c bench.Recall
	hasB := base != nil && base.Recall != nil
	hasC := cand != nil && cand.Recall != nil
	if hasB {
		b = *base.Recall
	}
	if hasC {
		c = *cand.Recall
	}
	return mdGateText(b.Asked, b.Passed, c.Asked, c.Passed, hasB, hasC)
}

// mdCheck names the verdict of the declared-answer check of the two runs.
func mdCheck(base, cand *bench.Cell) string {
	var b, c bench.Check
	hasB := base != nil && base.Check != nil
	hasC := cand != nil && cand.Check != nil
	if hasB {
		b = *base.Check
	}
	if hasC {
		c = *cand.Check
	}
	return mdGateText(b.Asked, b.Passed, c.Asked, c.Passed, hasB, hasC)
}

// mdGateText names the verdict of a gate of the two runs. It shows both
// when they differ, because a gate that changed is a finding.
func mdGateText(baseAsked, basePassed, candAsked, candPassed int, hasBase, hasCand bool) string {
	switch {
	case !hasBase && !hasCand:
		return "–"
	case !hasCand:
		return gateText(basePassed, baseAsked)
	case !hasBase:
		return gateText(candPassed, candAsked)
	case basePassed != candPassed:
		return fmt.Sprintf("%d/%d → %d/%d", basePassed, baseAsked, candPassed, candAsked)
	default:
		return gateText(candPassed, candAsked)
	}
}

// gateText names the verdict of a gate in words.
func gateText(passed, asked int) string {
	if asked == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d passed", passed, asked)
}

// mdSum adds a number of the two runs.
func mdSum(base, cand *bench.Cell, read func(*bench.Cell) int) int {
	n := 0
	if base != nil {
		n += read(base)
	}
	if cand != nil {
		n += read(cand)
	}
	return n
}

// mdIncomplete names whether one of the two runs lost a request.
func mdIncomplete(base, cand *bench.Cell) string {
	if (base != nil && base.Incomplete) || (cand != nil && cand.Incomplete) {
		return "yes"
	}
	return "no"
}

// mdText makes a value safe for a markdown table cell.
func mdText(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

// findCell returns the cell of a result with this group and level.
func findCell(r *bench.Result, group string, concurrency int) *bench.Cell {
	for i := range r.Cells {
		if r.Cells[i].Concurrency == concurrency && r.Cells[i].Group == group {
			return &r.Cells[i]
		}
	}
	return nil
}

// cellLabel names a cell of a comparison.
func cellLabel(group string, concurrency int) string {
	if group == "" {
		return fmt.Sprintf("c=%d", concurrency)
	}
	return fmt.Sprintf("%s c=%d", group, concurrency)
}
