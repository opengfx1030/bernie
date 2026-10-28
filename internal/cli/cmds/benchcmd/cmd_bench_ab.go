package benchcmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

// runAB runs the visits of an A/B comparison and prints the verdict of the
// two arms.
func runAB(c *cli.Context, cmd *cli.Command, args []string) error {
	fs := c.Flags(cmd)
	var o suite.ABOptions
	var visits string
	fs.StringVar(&o.Base, "base", "", "the baseline arm: a recipe id or the URL of a server")
	fs.StringVar(&o.Cand, "cand", "", "the candidate arm: a recipe id or the URL of a server")
	fs.StringVar(&o.Suite, "suite", "", "the suite every visit runs (default: the cells of the flags)")
	fs.StringVar(&visits, "visits", strings.Join(suite.DefaultVisits, ","), "the order of the visits")
	fs.StringVar(&o.Options.Model, "model", "", "model name (default: first id of /v1/models)")
	fs.IntVar(&o.Options.NumPrompts, "num-prompts", 0, "requests per cell (default max(4, 2 x concurrency))")
	fs.Int64Var(&o.Options.Seed, "seed", 0, "prompt seed")
	fs.StringVar(&o.Options.Tag, "tag", "", "free label stored with the result")
	fs.IntVar(&o.WaitS, "wait", 600, "seconds a started server may take to answer")
	fs.Float64Var(&o.Band, "band", 0, "share of a change that counts as noise (default: the band of the suite)")
	pos, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return cli.Usagef("unexpected argument %q", pos[0])
	}
	if o.Base == "" || o.Cand == "" {
		return cli.Usagef("bench ab needs a base arm and a candidate arm")
	}
	for _, v := range strings.Split(visits, ",") {
		arm := strings.TrimSpace(v)
		if arm != suite.ArmBase && arm != suite.ArmCand {
			return cli.Usagef("--visits: %q is not base or cand", arm)
		}
		o.Visits = append(o.Visits, arm)
	}
	o.Progress = c.Err
	res, paths, err := app.BenchAB(c.Ctx, o)
	if err != nil {
		return err
	}
	for _, p := range paths {
		c.Logf("bench: saved %s", p)
	}
	if err := c.Emit(res, func(w io.Writer) { printAB(w, res) }); err != nil {
		return err
	}
	if res.Verdict == nil {
		return nil
	}
	return verdictExit(res.Verdict, c.JSON)
}

// printAB writes the visits of an A/B run and the verdict of the arms.
func printAB(w io.Writer, res *suite.ABResult) {
	for _, v := range res.Visits {
		if v.Result == nil {
			continue
		}
		fmt.Fprintf(w, "visit %d %s: %s, %d cells\n", v.Index, v.Arm, v.Result.ID, len(v.Result.Cells))
	}
	if res.Verdict == nil {
		return
	}
	fmt.Fprintln(w)
	printVerdict(w, res.Verdict)
}
