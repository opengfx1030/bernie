package flow

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/setup"
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
	// Dashboard is the dashboard Up starts while the server loads the
	// model (spec §3.1). Nil means no dashboard.
	Dashboard Dashboard
	// Runner starts the run and lists the runs. Nil means the real one.
	Runner Runner
	// Prepared is an already built plan. Nil means the shared serve
	// operation prepares it.
	Prepared *recipe.Plan
	// OnEvent receives the events of the setup run (spec §6.4): the CLI
	// streams them, one line per step.
	OnEvent func(setup.Event)
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
	f, err := onboarding.Gather(ctx, onboarding.Options{
		Venv: o.Venv, Recipe: o.Recipe, Mode: o.Mode, Sets: o.Sets, Engine: o.Engine, Name: o.Name,
	})
	if err != nil {
		return nil, err
	}
	doc := onboarding.Build(f)

	// The one action of the first run, in the words of the checklist
	// (spec §3.3). The CLI, the MCP server and the page print this one
	// record, so they cannot tell the newcomer different stories.
	if p := doc.Primary; p != nil && p.Sentence != "" {
		fmt.Fprintf(log, "up: %s\n", p.Sentence)
		if p.Command != "" {
			fmt.Fprintf(log, "  %s\n", p.Command)
		}
	}

	// 1. Hardware access problems are not fixable by up. The steps of the
	// onboarding say what holds the path up (spec §3.1). The caller prints
	// the reason and the commands, so nothing is written here.
	if b := blocked(doc); b != nil {
		return nil, b
	}

	// 2. Recipe: the named one, else the best fitting one.
	id := f.Recipe
	if id == "" {
		return nil, &Blocked{Reason: "no recipe fits this host", Commands: []string{"bernie recipe list --all"}}
	}
	if o.Recipe == "" {
		// The same sentence the dashboard and `status --steps` print: the
		// recipe that fits this host and why it is the one.
		if r := doc.Recommendation; r != nil {
			fmt.Fprintf(log, "up: recipe: %s (%s) - %s\n", r.Name, r.ID, r.Why)
		} else {
			fmt.Fprintf(log, "up: best recipe for this host: %s\n", id)
		}
	}

	// 3. Mode: an engine is optional (spec §2).
	mode := f.Mode
	fmt.Fprintf(log, "up: mode %s\n", mode)

	// 4. Setup. Root steps stop up before anything runs.
	var rootCmds, apply []string
	var pending []setup.Step
	for _, s := range f.Setup {
		if s.Done {
			continue
		}
		if s.Root {
			rootCmds = append(rootCmds, s.Commands...)
			continue
		}
		apply = append(apply, s.ID)
		pending = append(pending, s)
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
		action := onboarding.SetupAction(pending, onboarding.Options{
			Mode: mode, Recipe: id, Venv: o.Venv,
		})
		action.Apply.Event = o.OnEvent
		run, err := action.Run(ctx, log)
		if err != nil {
			return nil, err
		}
		// The setup created/used this venv; pass it to serve so the
		// same run resolves the venv the steps used (spec §6.2).
		if run != nil && run.Venv != "" {
			o.Venv = run.Venv
		}
	}

	// 6. Serve, start the dashboard, wait for health.
	res := &UpResult{Recipe: id, Mode: mode, Applied: apply}
	if err := serveAndWait(ctx, o, log, res); err != nil {
		return res, err
	}
	return res, nil
}

// blocked returns what holds the path up, or nil when the path is clear.
// The steps come from the onboarding, so the page and `up` see the same
// state.
func blocked(doc *onboarding.Document) *Blocked {
	for _, id := range []string{onboarding.StepHardware, onboarding.StepAccess, onboarding.StepROCm, onboarding.StepRecipe} {
		s := findStep(doc.Steps, id)
		if s == nil || s.State != onboarding.StateBlocked {
			continue
		}
		return &Blocked{Reason: s.Summary, Commands: stepCommands(s)}
	}
	return nil
}

// stepCommands returns the commands that unblock a step: the root commands
// it needs, the fixes of its problems, or the command of the step itself.
func stepCommands(s *onboarding.Step) []string {
	cmds := append([]string{}, s.RootCommands...)
	for _, p := range s.Problems {
		if p.Fix != "" {
			cmds = append(cmds, p.Fix)
		}
	}
	if len(cmds) == 0 && s.ID == onboarding.StepRecipe {
		cmds = append(cmds, s.Command)
	}
	if len(cmds) == 0 {
		cmds = append(cmds, "bernie doctor")
	}
	return cmds
}

// WaitHealthy waits until the run answers /health (limit 0 = 30 minutes). A
// run that exits first yields a *StartupExit with the known failures in its
// log. When progress is not nil, it gets one line per phase change: what the
// server is doing now, in words (spec §3.1).
func WaitHealthy(ctx context.Context, name string, limit time.Duration, progress io.Writer) error {
	return waitHealthy(ctx, name, limit, progress, runner.List)
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
