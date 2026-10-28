package benchcmd

import (
	"fmt"
	"github.com/opengfx1030/rdnactl/internal/cli"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/rdnactl/internal/bench"
)

func init() {
	cmd := &cli.Command{
		Name:    "bench",
		Summary: "Benchmark an OpenAI-compatible server and save the result",
		Usage:   "[--url U] [--model M] [--input-len N] [--output-len N] [--concurrency 1,8] [--num-prompts N] [--warmup N] [--seed N] [--tag T]",
	}
	cmd.Run = func(c *cli.Context, args []string) error { return runBench(c, cmd, args) }

	list := &cli.Command{Name: "list", Summary: "List saved bench results"}
	list.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(list), args); err != nil {
			return err
		}
		rs, err := bench.List()
		if err != nil {
			return err
		}
		if rs == nil {
			rs = []bench.Result{}
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

	cmd.Subs = []*cli.Command{list, show}
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
	pos, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return cli.Usagef("unexpected argument %q (subcommands: list, show)", pos[0])
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

	r, err := bench.Run(c.Ctx, o, c.Err)
	if err != nil {
		return err
	}
	if r.Completed() == 0 {
		printBenchResult(c.Err, r)
		return fmt.Errorf("all requests failed; result not saved")
	}
	path, err := bench.Save(r)
	if err != nil {
		return fmt.Errorf("save result: %w", err)
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
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "c\tok/fail\tTTFT p50\tp95 ms\tITL p50\tp95 ms\tdecode tok/s\tprefill tok/s\toutput tok/s\tin/out tok\ttok/chunk\t")
	for _, cl := range r.Cells {
		fmt.Fprintf(tw, "%d\t%d/%d\t%.1f\t%.1f\t%.2f\t%.2f\t%.1f\t%.0f\t%.1f\t%.0f/%.0f\t%.2f\t\n",
			cl.Concurrency, cl.Completed, cl.Failed, cl.TTFTP50, cl.TTFTP95, cl.ITLP50, cl.ITLP95,
			cl.DecodeTokS, cl.PrefillTokS, cl.OutputTokS, cl.MeanPromptTokens, cl.MeanOutputTokens, cl.TokensPerChunk)
	}
	tw.Flush()
	for _, cl := range r.Cells {
		for _, e := range cl.Errors {
			fmt.Fprintf(w, "c=%d error: %s\n", cl.Concurrency, e)
		}
	}
}

func fingerprintLine(fp map[string]string) string {
	var parts []string
	for _, k := range []string{"host", "gpus", "run", "recipe", "plan_hash"} {
		if v := fp[k]; v != "" {
			parts = append(parts, k+" "+v)
		}
	}
	return strings.Join(parts, ", ")
}

func printBenchList(w io.Writer, rs []bench.Result) {
	if len(rs) == 0 {
		fmt.Fprintln(w, "no bench results (run `rdnactl bench`)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTIME\tMODEL\tIN/OUT\tCONCURRENCY\tBEST OUT TOK/S\tTAG")
	for _, r := range rs {
		cs := make([]string, len(r.Cells))
		best := 0.0
		for i, cl := range r.Cells {
			cs[i] = strconv.Itoa(cl.Concurrency)
			best = max(best, cl.OutputTokS)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d/%d\t%s\t%.1f\t%s\n", r.ID, r.Time.Local().Format("2006-01-02 15:04"),
			r.Model, r.Options.InputLen, r.Options.OutputLen, strings.Join(cs, ","), best, r.Tag)
	}
	tw.Flush()
}
