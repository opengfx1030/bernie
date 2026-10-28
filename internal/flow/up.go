package flow

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/setup"
	"github.com/opengfx1030/bernie/internal/support/config"
)

// Blocked means `up` stopped before a step it must not run itself: a step
// that needs root, a missing input, a hardware access problem, or a missing
// confirmation. Commands are what the operator runs next. Front ends map it
// to exit code 3.
type Blocked struct {
	Reason   string   `json:"reason"`
	Commands []string `json:"commands"`
}

func (b *Blocked) Error() string { return b.Reason }

// UpOptions are the inputs of Up.
type UpOptions struct {
	Recipe string // "" = best fitting recipe
	Sets   []string
	Mode   string // "" = config, else container when possible, else host
	Venv   string
	Engine string
	Name   string
	// Yes skips the confirmation. Without Yes, Confirm is asked once; a nil
	// Confirm means "cannot ask" and Up returns Blocked.
	Yes     bool
	Confirm func(prompt string) bool
	// Wait bounds how long Up waits for /health. 0 = 30 minutes.
	Wait time.Duration
}

// UpResult is the outcome of a completed Up.
type UpResult struct {
	Recipe  string         `json:"recipe"`
	Mode    string         `json:"mode"`
	Applied []string       `json:"applied_steps"`
	Run     *runner.Record `json:"run"`
	Healthy bool           `json:"healthy"`
	URL     string         `json:"url"`
}

// Up runs the whole path of spec §3.1: doctor, setup (non-root), recipe,
// serve, wait for health. Progress goes to log.
func Up(ctx context.Context, o UpOptions, log io.Writer) (*UpResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	h, _ := host.Probe(ctx, "/")
	gpus, _ := gpu.List("/")
	if len(gpus) == 0 {
		return nil, &Blocked{Reason: "no AMD GPU found", Commands: []string{"bernie doctor"}}
	}

	// 1. Hardware access problems are not fixable by up.
	rep := doctor.Run(ctx, doctor.Options{Root: "/", Venv: cfg.ResolveVenv(o.Venv), HFHome: cfg.ResolveHFHome(),
		Categories: []string{"gpu", "kernel"}, ValidatedArchs: recipe.ValidatedArchs(recipe.Load())})
	var cmds []string
	for _, ch := range rep.Checks {
		if ch.Status == doctor.Fail {
			fmt.Fprintf(log, "up: FAIL %s\n", ch.Title)
			if ch.Fix != "" {
				cmds = append(cmds, ch.Fix)
			}
		}
	}
	if len(cmds) > 0 || rep.Failed() {
		return nil, &Blocked{Reason: "the GPUs are not usable by this user yet", Commands: cmds}
	}

	// 2. Recipe.
	set := recipe.Load()
	id := o.Recipe
	if id == "" {
		id = pick(recipe.List(set, gpus, h.RAMBytes, false), o.Sets)
		if id == "" {
			return nil, &Blocked{Reason: "no recipe fits this host", Commands: []string{"bernie recipe list --all"}}
		}
		fmt.Fprintf(log, "up: best recipe for this host: %s\n", id)
	}
	r, err := set.Resolve(id)
	if err != nil {
		return nil, err
	}
	for name, in := range r.Inputs {
		if in.Required && in.Default == "" && !hasSet(o.Sets, name) {
			return nil, &Blocked{
				Reason:   fmt.Sprintf("recipe %s needs input %s: %s", id, name, in.Description),
				Commands: []string{fmt.Sprintf("bernie up %s --set %s=<value>", id, name)},
			}
		}
	}

	// 3. Mode: an engine is optional (spec §2).
	mode := o.Mode
	if mode == "" {
		mode = cfg.Mode
	}
	if mode == "" {
		mode = recipe.ModeHost
		if slices.Contains(recipe.Modes(r), recipe.ModeContainer) && engineUsable(h, firstNonEmpty(o.Engine, cfg.Engine)) {
			mode = recipe.ModeContainer
		}
	}
	fmt.Fprintf(log, "up: mode %s\n", mode)

	// 4. Setup. Root steps stop up before anything runs.
	var archs []string
	for _, g := range gpus {
		archs = append(archs, g.GFX)
	}
	steps := setup.Plan(ctx, h, rocm.Probe(ctx, "/"), setup.Options{
		Mode: mode, Venv: cfg.ResolveVenv(o.Venv), HFHome: cfg.ResolveHFHome(), GPUArchs: archs,
		ModelRepo: modelRepo(r.Model.Source),
	})
	var rootCmds, apply []string
	for _, s := range steps {
		if s.Done {
			continue
		}
		if s.Root {
			rootCmds = append(rootCmds, s.Commands...)
			continue
		}
		apply = append(apply, s.ID)
	}
	if len(rootCmds) > 0 {
		return nil, &Blocked{Reason: "steps that need root are pending; run them, then run `bernie up` again", Commands: rootCmds}
	}

	// 5. One confirmation.
	if !o.Yes {
		prompt := fmt.Sprintf("Run setup steps [%s] and start %s (%s mode)?", strings.Join(apply, ", "), id, mode)
		if o.Confirm == nil {
			return nil, &Blocked{Reason: "confirmation required; re-run with --yes", Commands: []string{"bernie up " + id + " --yes"}}
		}
		if !o.Confirm(prompt) {
			return nil, &Blocked{Reason: "declined"}
		}
	}
	if len(apply) > 0 {
		if err := setup.Apply(ctx, steps, apply, log); err != nil {
			return nil, err
		}
	}

	// 6. Serve and wait for health.
	p, err := runner.Prepare(ctx, runner.PrepareOptions{Recipe: id, Mode: mode, Name: o.Name, Venv: o.Venv, Engine: o.Engine, Sets: o.Sets})
	if err != nil {
		return nil, err
	}
	for _, w := range p.Warnings {
		fmt.Fprintf(log, "up: warning: %s\n", w)
	}
	rec, err := runner.Start(ctx, p, runner.StartOptions{Venv: o.Venv, Engine: o.Engine, Progress: log})
	if err != nil {
		return nil, err
	}
	res := &UpResult{Recipe: id, Mode: mode, Applied: apply, Run: rec, URL: rec.URL()}
	if err := waitHealthy(ctx, rec.Name, o.Wait, log); err != nil {
		return res, err
	}
	res.Healthy = true
	return res, nil
}

func waitHealthy(ctx context.Context, name string, limit time.Duration, log io.Writer) error {
	if limit == 0 {
		limit = 30 * time.Minute
	}
	deadline := time.Now().Add(limit)
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	fmt.Fprintf(log, "up: waiting for %s to answer /health (logs: bernie logs %s -f)\n", name, name)
	for {
		runs, err := runner.List(ctx)
		if err != nil {
			return err
		}
		for _, s := range runs {
			if s.Name != name {
				continue
			}
			switch s.State {
			case "running":
				return nil
			case "exited":
				return fmt.Errorf("%s exited during startup; see: bernie logs %s", name, name)
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s is not healthy after %s; see: bernie logs %s", name, limit, name)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// pick returns the best fitting recipe whose required inputs are all
// satisfied (a default, or a --set). When none is, it returns the best
// fitting recipe, so up can say which input is missing.
func pick(l recipe.Listing, sets []string) string {
	var ready recipe.Listing
	for _, e := range l.Recipes {
		ok := true
		for name, in := range e.Inputs {
			if in.Required && in.Default == "" && !hasSet(sets, name) {
				ok = false
			}
		}
		if ok {
			ready.Recipes = append(ready.Recipes, e)
		}
	}
	if id := recipe.Best(ready); id != "" {
		return id
	}
	return recipe.Best(l)
}

func engineUsable(h host.Info, want string) bool {
	for _, e := range h.Engines {
		if e.Usable && (want == "" || e.Name == want) {
			return true
		}
	}
	return false
}

func hasSet(sets []string, name string) bool {
	for _, s := range sets {
		if strings.HasPrefix(s, name+"=") {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// modelRepo returns the Hugging Face repo to download, or "" for a local path.
func modelRepo(source string) string {
	if strings.HasPrefix(source, "/") || strings.HasPrefix(source, "${") || source == "" {
		return ""
	}
	return source
}

// ConfirmFrom returns a Confirm that asks on w and reads a y/N line from r.
func ConfirmFrom(r io.Reader, w io.Writer) func(string) bool {
	br := bufio.NewReader(r)
	return func(prompt string) bool {
		fmt.Fprintf(w, "%s [y/N] ", prompt)
		line, err := br.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false
		}
		a := strings.ToLower(strings.TrimSpace(line))
		return a == "y" || a == "yes"
	}
}
