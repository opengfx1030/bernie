package suite

import (
	"fmt"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// The dash is what a number that was not measured shows as.
const dash = "–"

// RateText formats a token rate. A number that was not measured shows as a
// dash.
func RateText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.1f", v)
}

// ShortDigest is the first twelve characters of a digest.
func ShortDigest(d string) string {
	if len(d) <= 12 {
		return d
	}
	return d[:12]
}

// Receipt is the comparison as a markdown receipt: the scope of the two runs,
// their correctness, the numbers and the verdict. It is the text that goes
// into a pull request.
func Receipt(v *Verdict) string {
	var b strings.Builder
	b.WriteString("# Benchmark receipt\n\n")
	b.WriteString("## Scope\n\n")
	b.WriteString("| | |\n|---|---|\n")
	receiptRow(&b, "baseline", v.A.ID)
	receiptRow(&b, "candidate", v.B.ID)
	receiptRow(&b, "model", v.A.Model)
	receiptRow(&b, "server", v.A.URL)
	if v.Workload.SuiteID != "" {
		receiptRow(&b, "suite", v.Workload.SuiteID+" (digest "+ShortDigest(v.Workload.SuiteDigest)+")")
	}
	receiptRow(&b, "measured", v.B.Time.Format(time.RFC3339))
	receiptRow(&b, "host", v.B.Fingerprint["host"])
	receiptRow(&b, "GPUs", gpuScopeText(v.B.Fingerprint))
	receiptRow(&b, "run", v.B.Fingerprint["run"])
	receiptRow(&b, "recipe", v.B.Fingerprint["recipe"])
	b.WriteString("\n## Correctness\n\n")
	b.WriteString("| cell | recall | check | failed | warm-up | incomplete |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, cv := range v.Cells {
		base := findCell(v.A, cv.Group, cv.Concurrency)
		cand := findCell(v.B, cv.Group, cv.Concurrency)
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %s |\n",
			mdText(CellLabel(cv.Group, cv.Concurrency)),
			mdRecall(base, cand),
			mdCheck(base, cand),
			mdSum(base, cand, func(c *bench.Cell) int { return c.Failed }),
			mdSum(base, cand, func(c *bench.Cell) int { return c.WarmupFailed }),
			mdIncomplete(base, cand))
	}
	b.WriteString("\n## Timings\n")
	for _, cv := range v.Cells {
		fmt.Fprintf(&b, "\n### %s\n\n", mdText(CellLabel(cv.Group, cv.Concurrency)))
		b.WriteString("| metric | baseline | candidate | delta | verdict |\n")
		b.WriteString("|---|---|---|---|---|\n")
		for _, m := range cv.Metrics {
			fmt.Fprintf(&b, "| %s | %s | %s | %+.1f %% | %s |\n",
				mdText(m.Label), mdText(RateText(m.Base)), mdText(RateText(m.Cand)), m.DeltaPct, m.Verdict)
		}
	}
	b.WriteString("\n## Verdict\n\n")
	fmt.Fprintf(&b, "%d better, %d same, %d worse, %d noisy; recall %s.\n",
		v.Summary.Better, v.Summary.Same, v.Summary.Worse, v.Summary.Noisy, RecallVerdictText(v))
	for _, r := range v.Reasons {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	b.WriteString("\n## Reproduce\n\n")
	b.WriteString("```bash\n")
	fmt.Fprintf(&b, "bernie bench compare %s %s --markdown\n", v.A.ID, v.B.ID)
	b.WriteString("```\n")
	return b.String()
}

// receiptRow writes one row of the scope table. An empty value leaves the
// row out: a receipt says what it knows.
func receiptRow(b *strings.Builder, name, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "| %s | %s |\n", name, mdText(value))
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
		return dash
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
		return dash
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

// RecallVerdictText names the recall gate of a comparison.
func RecallVerdictText(v *Verdict) string {
	if !v.Summary.RecallOK {
		return "failed"
	}
	return "ok"
}
