package benchcmd

import (
	"errors"
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/bench"
)

func init() {
	cmd := &cli.Command{
		Name:    "bench",
		Summary: "Benchmark an OpenAI-compatible server and save the result",
		Usage:   "[--url U] [--model M] [--input-len N] [--output-len N] [--concurrency 1,8] [--num-prompts N] [--warmup N] [--seed N] [--tag T] [--recall]",
	}
	cmd.Run = func(c *cli.Context, args []string) error { return runBench(c, cmd, args) }

	list := &cli.Command{Name: "list", Summary: "List saved bench results"}
	list.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(list), args); err != nil {
			return err
		}
		rs, err := app.BenchList()
		if err != nil {
			return err
		}
		return c.Emit(rs, func(w io.Writer) { printBenchList(w, rs) })
	}

	show := &cli.Command{Name: "show", Summary: "Show a saved bench result", Usage: "<id>"}
	show.Run = func(c *cli.Context, args []string) error {
		pos, err := cli.Parse(c.Flags(show), args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return cli.Usagef("bench show needs exactly one result id")
		}
		r, err := bench.Get(pos[0])
		if err != nil {
			return err
		}
		return c.Emit(r, func(w io.Writer) { printBenchResult(w, r) })
	}

	compare := &cli.Command{Name: "compare", Summary: "Compare two saved bench results", Usage: "<id-a> <id-b>"}
	compare.Run = func(c *cli.Context, args []string) error {
		pos, err := cli.Parse(c.Flags(compare), args)
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
		cmp, err := bench.Compare(a, b)
		if err != nil {
			return err
		}
		return c.Emit(cmp, func(w io.Writer) { printComparison(w, cmp) })
	}

	cmd.Subs = []*cli.Command{list, show, compare}
	cli.Register(cmd)
}

func runBench(c *cli.Context, cmd *cli.Command, args []string) error {
	fs := c.Flags(cmd)
	var o bench.Options
	var conc string
	fs.StringVar(&o.URL, "url", "", "server base URL (default: the only run, else "+bench.FallbackURL+")")
	fs.StringVar(&o.Model, "model", "", "model name (default: first id of /v1/models)")
	fs.IntVar(&o.InputLen, "input-len", bench.DefaultInputLen, "prompt tokens (calibrated with /tokenize when present)")
	fs.IntVar(&o.OutputLen, "output-len", bench.DefaultOutputLen, "output tokens (ignore_eos, temperature 0)")
	fs.StringVar(&conc, "concurrency", "1", "comma list of concurrency levels, one cell each (e.g. 1,8)")
	fs.IntVar(&o.NumPrompts, "num-prompts", 0, "requests per cell (default max(4, 2 x concurrency))")
	fs.IntVar(&o.Warmup, "warmup", bench.DefaultWarmup, "requests sent and discarded before each cell")
	fs.Int64Var(&o.Seed, "seed", 0, "prompt seed")
	fs.StringVar(&o.Tag, "tag", "", "free label stored with the result")
	fs.BoolVar(&o.Recall, "recall", false, "hide a short random fact in every prompt and check the reply")
	pos, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return cli.Usagef("unexpected argument %q (subcommands: list, show, compare)", pos[0])
	}
	for _, f := range strings.Split(conc, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 1 {
			return cli.Usagef("--concurrency: %q is not a positive integer", f)
		}
		o.Concurrency = append(o.Concurrency, n)
	}
	if o.InputLen < 1 || o.OutputLen < 1 {
		return cli.Usagef("--input-len and --output-len must be positive")
	}
	if o.NumPrompts < 0 || o.Warmup < 0 {
		return cli.Usagef("--num-prompts and --warmup must not be negative")
	}

	r, path, err := app.Bench(c.Ctx, o, c.Err)
	if errors.Is(err, app.ErrAllFailed) {
		printBenchResult(c.Err, r)
	}
	if err != nil {
		return err
	}
	c.Logf("bench: saved %s", path)
	return c.Emit(r, func(w io.Writer) { printBenchResult(w, r) })
}

func printBenchResult(w io.Writer, r *bench.Result) {
	fmt.Fprintf(w, "%s  %s  %s\n", r.ID, r.Model, r.URL)
	cal := "calibrated"
	if !r.Calibrated {
		cal = "estimated"
	}
	fmt.Fprintf(w, "input %d tokens (%s), output %d tokens, warmup %d", r.Options.InputLen, cal, r.Options.OutputLen, r.Options.Warmup)
	if r.Tag != "" {
		fmt.Fprintf(w, ", tag %s", r.Tag)
	}
	fmt.Fprintln(w)
	if fp := fingerprintLine(r.Fingerprint); fp != "" {
		fmt.Fprintln(w, fp)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, metricHelp)
	for _, cl := range r.Cells {
		printCell(w, &cl)
		for _, e := range cl.Errors {
			fmt.Fprintf(w, "c=%d error: %s\n", cl.Concurrency, e)
		}
		if cl.Recall == nil {
			continue
		}
		fmt.Fprintf(w, "c=%d recall %d/%d passed (hidden fact at %d%% of the prompt, %s)\n",
			cl.Concurrency, cl.Recall.Passed, cl.Recall.Asked, int(cl.Recall.Depth*100), recallMethodText(cl.Recall.Method))
		if cl.Recall.Passed < cl.Recall.Asked {
			fmt.Fprintf(w, "c=%d the server did not repeat the hidden fact: it may not read the whole prompt, so the speed numbers of this cell are meaningless\n", cl.Concurrency)
		}
	}
}

// metricHelp explains in one line how the prefill and the decode numbers
// are measured.
const metricHelp = "prefill per request = prompt tokens ÷ time to first token (the queue time is in it), per cell = prompt tokens ÷ (first request → last first token).\n" +
	"decode per request = (tokens − 1) ÷ (last token − first token), per cell = decode tokens ÷ the time any request was decoding."

// dash is what a number that was not measured shows.
const dash = "–"

// printCell writes the table of one cell: the five groups of numbers of
// spec §10.
func printCell(w io.Writer, cl *bench.Cell) {
	fmt.Fprintf(w, "cell c=%d   %d ok / %d failed   wall %.2f s   %s tok/chunk   in/out %s/%s tok\n",
		cl.Concurrency, cl.Completed, cl.Failed, cl.DurationS, chunkText(cl.TokensPerChunk),
		countText(cl.MeanPromptTokens), countText(cl.MeanOutputTokens))
	fmt.Fprintln(w, cellRow("metric", "agg", "req mean", "req p50", "req p95", "req min", "req max"))
	fmt.Fprintln(w, rateRow("prefill tok/s", cl.Prefill))
	fmt.Fprintln(w, rateRow("decode tok/s", cl.Decode))
	fmt.Fprintln(w, distRow("TTFT s", cl.TTFTs, secText))
	fmt.Fprintln(w, distRow("ITL ms", cl.ITLms, msText))
	fmt.Fprintln(w, cellRow("output tok/s", rateText(cl.OutputTokS), dash, dash, dash, dash, dash))
	if cl.ServerSide != nil {
		fmt.Fprintf(w, "server side: %s\n", serverSideText(cl.ServerSide))
	}
}

// cellRow formats one line of the cell table: the name on the left, the
// numbers in fixed columns.
func cellRow(label string, cells ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-15s", label)
	for _, c := range cells {
		fmt.Fprintf(&b, " %9s", c)
	}
	return b.String()
}

// rateRow is the row of a speed: the rate of the cell and the rates of its
// requests.
func rateRow(label string, r *bench.Rate) string {
	if r == nil {
		return cellRow(label, dash, dash, dash, dash, dash, dash)
	}
	return cellRow(label, rateText(r.AggTokS), rateText(dMean(r.Req)), rateText(dP50(r.Req)),
		rateText(dP95(r.Req)), rateText(dMin(r.Req)), rateText(dMax(r.Req)))
}

// distRow is the row of a latency: the distribution over the requests.
func distRow(label string, d *bench.Dist, format func(float64) string) string {
	return cellRow(label, dash, format(dMean(d)), format(dP50(d)), format(dP95(d)), format(dMin(d)), format(dMax(d)))
}

// serverSideText names what the server measured over the same requests.
func serverSideText(s *bench.ServerSide) string {
	parts := []string{fmt.Sprintf("%d requests", s.Count)}
	if s.TTFTs != nil {
		parts = append(parts, fmt.Sprintf("TTFT s p50 %s p95 %s", secText(dP50(s.TTFTs)), secText(dP95(s.TTFTs))))
	}
	if s.ITLms != nil {
		parts = append(parts, fmt.Sprintf("ITL ms p50 %s p95 %s", msText(dP50(s.ITLms)), msText(dP95(s.ITLms))))
	}
	return strings.Join(parts, ", ")
}

// dMean, dP50, dP95, dMin and dMax read one number of a distribution. An
// absent distribution has no numbers.
func dMean(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.Mean
}

func dP50(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.P50
}

func dP95(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.P95
}

func dMin(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.Min
}

func dMax(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.Max
}

// rateText formats a token rate, secText a time in seconds and msText a
// time in milliseconds. A number that was not measured shows as "–".
func rateText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.1f", v)
}

func secText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.3f", v)
}

func msText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.2f", v)
}

// countText formats a token count and chunkText the tokens of a chunk. A
// number that was not measured shows as "–".
func countText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.0f", v)
}

func chunkText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.2f", v)
}

// recallMethodText names how the prompt length was measured, in words.
func recallMethodText(method string) string {
	if method == "tokenize" {
		return "prompt sized with /tokenize"
	}
	return "prompt length estimated, the server has no /tokenize"
}

// recallText names the verdict of a cell in words.
func recallText(r *bench.Recall) string {
	if r == nil {
		return "–"
	}
	return fmt.Sprintf("%d/%d passed", r.Passed, r.Asked)
}

// resultRecall totals the recall of the cells of a result.
func resultRecall(r *bench.Result) *bench.Recall {
	var asked, passed int
	for _, c := range r.Cells {
		if c.Recall == nil {
			continue
		}
		asked += c.Recall.Asked
		passed += c.Recall.Passed
	}
	if asked == 0 {
		return nil
	}
	return &bench.Recall{Asked: asked, Passed: passed}
}

// printComparison puts two results of the same workload side by side.
func printComparison(w io.Writer, cmp *bench.Comparison) {
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
	fmt.Fprintln(w)
	fmt.Fprintln(w, metricHelp)
	for _, cl := range cmp.A.Cells {
		fmt.Fprintf(w, "result %s\n", cmp.A.ID)
		printCell(w, &cl)
		if other := cellOf(cmp.B, cl.Concurrency); other != nil {
			fmt.Fprintf(w, "result %s\n", cmp.B.ID)
			printCell(w, other)
		}
	}
}

// cellOf returns the cell of a result with this concurrency.
func cellOf(r *bench.Result, concurrency int) *bench.Cell {
	for i := range r.Cells {
		if r.Cells[i].Concurrency == concurrency {
			return &r.Cells[i]
		}
	}
	return nil
}

func fingerprintLine(fp map[string]string) string {
	var parts []string
	if v := fp["host"]; v != "" {
		parts = append(parts, "host "+v)
	}
	if v := fp["gpus"]; v != "" {
		gpus := "gpus " + v
		if scope := fp["gpu_scope"]; scope != "" {
			gpus += " (" + scope + ")"
		}
		parts = append(parts, gpus)
	}
	for _, k := range []string{"run", "recipe", "plan_hash"} {
		if v := fp[k]; v != "" {
			parts = append(parts, k+" "+v)
		}
	}
	return strings.Join(parts, ", ")
}

func printBenchList(w io.Writer, rs []bench.Result) {
	if len(rs) == 0 {
		fmt.Fprintln(w, "no bench results (run `bernie bench`)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTIME\tMODEL\tIN/OUT\tCONCURRENCY\tBEST OUT TOK/S\tBEST DECODE TOK/S\tRECALL\tTAG")
	for _, r := range rs {
		cs := make([]string, len(r.Cells))
		best, bestDecode := 0.0, 0.0
		for i, cl := range r.Cells {
			cs[i] = strconv.Itoa(cl.Concurrency)
			best = max(best, cl.OutputTokS)
			if cl.Decode != nil {
				bestDecode = max(bestDecode, cl.Decode.AggTokS)
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d/%d\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Time.Local().Format("2006-01-02 15:04"),
			r.Model, r.Options.InputLen, r.Options.OutputLen, strings.Join(cs, ","), rateText(best), rateText(bestDecode),
			recallText(resultRecall(&r)), r.Tag)
	}
	tw.Flush()
}
