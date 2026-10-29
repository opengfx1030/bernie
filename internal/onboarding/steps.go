package onboarding

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/setup"
)

// The doctor checks that hold a step back. An entry matches a check id
// exactly or as a prefix.
var stepChecks = map[string][]string{
	StepHardware:  {"gpu.count", "gpu.supported.", "gpu.ecc.", "gpu.igpu"},
	StepAccess:    {"gpu.kfd", "gpu.render", "gpu.groups", "container.cdi", "container.device_access"},
	StepROCm:      {"kernel.", "rocm."},
	StepPythonEnv: {"python."},
	StepTuning:    {"tuning.tunableop"},
	StepPower:     {"tuning.power"},
}

// pythonSetupSteps are the setup steps that build the host stack.
var pythonSetupSteps = []string{"venv.create", "venv.torch", "causal-conv1d.build", "flash-attn.build", "vllm.build"}

// hardwareStep: the GPUs are found and supported.
func hardwareStep(f *Facts) Step {
	s := Step{ID: StepHardware, Title: "GPUs found and supported", Command: "bernie hw", Action: "Check the cards"}
	s.Problems = problemsOf(f.Doctor, stepChecks[StepHardware]...)
	switch {
	case len(f.GPUs) == 0:
		s.State = StateBlocked
		s.Summary = "No AMD GPU is visible to bernie."
		s.Detail = "bernie finds the cards through the amdgpu driver. On Linux that driver must be loaded; then /dev/kfd is the file a program opens to reach the GPU."
	case hasFail(f.Doctor, stepChecks[StepHardware]...):
		s.State = StateBlocked
		s.Summary = "One of the GPUs is not usable yet."
	default:
		s.State = StateDone
		s.Summary = fmt.Sprintf("%d GPU(s) found: %s.", len(f.GPUs), f.gpuSummary())
	}
	if f.Restriction != "" {
		s.Detail = joinDetail(s.Detail, f.Restriction)
	}
	return s
}

// accessStep: this user can open the device nodes.
func accessStep(f *Facts) Step {
	s := Step{ID: StepAccess, Title: "Device access and groups", Command: "bernie doctor --category gpu", Action: "Fix the device access"}
	s.Problems = problemsOf(f.Doctor, stepChecks[StepAccess]...)
	if hasFail(f.Doctor, stepChecks[StepAccess]...) || len(rootCommands(f.Setup, "groups")) > 0 {
		s.State = StateBlocked
		s.Summary = "This account cannot open the GPU devices."
		s.RootCommands = rootCommands(f.Setup, "groups")
		// The note of the plan step says what the operator has to do after
		// the command (a new session, for the groups).
		s.Detail = setupWhy(f.Setup, "groups")
		return s
	}
	s.State = StateDone
	s.Summary = "The user can open /dev/kfd and the render nodes."
	return s
}

// rocmStep: the ROCm runtime and the kernel are ready.
func rocmStep(f *Facts) Step {
	s := Step{ID: StepROCm, Title: "ROCm runtime and kernel", Command: "bernie doctor --category rocm", Action: "Install the ROCm tools"}
	s.Problems = problemsOf(f.Doctor, stepChecks[StepROCm]...)
	if hasFail(f.Doctor, stepChecks[StepROCm]...) || len(rootCommands(f.Setup, "rocm.runtime")) > 0 {
		s.State = StateBlocked
		s.Summary = "The ROCm runtime or the kernel is not ready."
		s.RootCommands = rootCommands(f.Setup, "rocm.runtime")
		s.Detail = setupWhy(f.Setup, "rocm.runtime")
		return s
	}
	s.State = StateDone
	s.Summary = "ROCm and the kernel are ready."
	return s
}

// pythonStep: the venv holds torch and vLLM for this GPU.
func pythonStep(f *Facts) Step {
	s := Step{ID: StepPythonEnv, Title: "Python environment (torch and vLLM)", Action: "Build the Python environment"}
	s.Problems = problemsOf(f.Doctor, stepChecks[StepPythonEnv]...)
	pending := pendingSetup(f.Setup, pythonSetupSteps)
	if len(setupSteps(f.Setup, pythonSetupSteps)) == 0 {
		s.State = StateSkipped
		s.Summary = "The container image carries the stack; no venv is needed."
		s.Command = "bernie doctor --category python"
		return s
	}
	if len(pending) == 0 {
		s.State = StateDone
		s.Summary = "The venv holds torch and vLLM for this GPU."
		s.Command = "bernie doctor --category python"
		return s
	}
	s.State = StateTodo
	s.CanRun = true
	s.Summary = fmt.Sprintf("The Python environment with torch and vLLM is not built yet (%d of %d steps are missing).", len(pending), len(pythonSetupSteps))
	s.Detail = "It writes the environment in " + venvPath(f) + "."
	s.Command = applyCommand(pending)
	return s
}

// venvPath is the directory of the Python environment of the host.
func venvPath(f *Facts) string {
	if f.Venv != "" {
		return f.Venv
	}
	return setup.DefaultVenv()
}

// modelStep: the model files are in the Hugging Face cache.
func modelStep(f *Facts) Step {
	s := Step{ID: StepModel, Title: "Model files", Command: "bernie setup apply --step model.download", Action: "Download the model"}
	st := setupStep(f.Setup, "model.download")
	switch {
	case st == nil:
		s.State = StateSkipped
		s.Summary = "No model is known yet; choose a recipe first."
	case st.Done:
		s.State = StateDone
		s.Summary = "The model is in the Hugging Face cache."
	default:
		s.State = StateTodo
		s.CanRun = true
		s.Summary = "The model files are not in the cache yet."
		s.Detail = "It writes the files in " + f.HFHome + "."
		if st.Download != nil && st.Download.ResumeBytes > 0 {
			s.Progress = &Progress{
				Phase: fmt.Sprintf("%s already downloaded", humanBytes(st.Download.ResumeBytes)),
				Done:  st.Download.ResumeBytes,
			}
		}
	}
	return s
}

// recipeStep: the recipe to serve is known and complete.
func recipeStep(f *Facts) Step {
	s := Step{ID: StepRecipe, Title: "Recipe", Command: "bernie recipe list --all", Action: "Pick the recipe"}
	if f.Recipe == "" {
		s.State = StateBlocked
		s.Summary = "No recipe fits this host."
		s.Detail = "bernie recipe list --all shows every recipe and why it does not fit."
		return s
	}
	if name := missingInputOf(f); name != "" {
		s.State = StateBlocked
		s.Summary = fmt.Sprintf("The recipe %s needs the input %s.", f.Recipe, name)
		s.Command = fmt.Sprintf("bernie up %s --set %s=<value>", f.Recipe, name)
		return s
	}
	s.State = StateDone
	s.Summary = fmt.Sprintf("Serving %s.", f.Recipe)
	if alt := alternatives(f); alt != "" {
		s.Detail = alt
	}
	return s
}

// serveStep: the server runs.
func serveStep(f *Facts) Step {
	s := Step{ID: StepServe, Title: "Serve the model", Command: "bernie up", Action: "Start the server"}
	if f.Recipe != "" {
		s.Command = "bernie up " + f.Recipe
	}
	if run := f.live(); run != nil {
		s.State = StateRunning
		s.Summary = fmt.Sprintf("%s is %s.", run.Name, run.State)
		s.Detail = run.URL
		s.Progress = &Progress{Phase: run.Phase}
		// The run answers when its health check passes: the runner calls
		// that state "running".
		s.Serving = servingRun(f)
		return s
	}
	s.State = StateTodo
	s.CanRun = true
	s.Summary = "No server is running."
	if last := f.lastExited(); last != nil {
		s.Detail = fmt.Sprintf("The last run %s exited; bernie logs %s shows why.", last.Name, last.Name)
	}
	return s
}

// loadingPhases are the phases in which the server does not answer yet.
var loadingPhases = regexp.MustCompile(`(?i)loading|compiling|sizing|capturing|starting|warming`)

// servingRun says whether the live run answers requests now: its health
// check passed and its log shows no step that comes before that.
func servingRun(f *Facts) bool {
	run := f.live()
	if run == nil {
		return false
	}
	if run.State != runner.StateRunning {
		return false
	}
	return !loadingPhases.MatchString(run.Phase)
}

// verifyStep: the server answers a request.
func verifyStep(f *Facts) Step {
	s := Step{ID: StepVerify, Title: "The server answers a request", Command: "bernie smoke", Action: "Ask the server"}
	run := f.live()
	if run == nil {
		s.State = StateSkipped
		s.Summary = "There is no server to test."
		return s
	}
	if j := lastJob(f.Jobs, StepVerify); j != nil && j.State == JobDone {
		s.State = StateDone
		s.Summary = "The server answered a request."
		return s
	}
	s.State = StateTodo
	s.CanRun = true
	s.Summary = "Send one token to the server to be sure it answers."
	s.Detail = run.URL
	return s
}

// problemsOf returns the failed and warned checks as problems.
func problemsOf(rep doctor.Report, ids ...string) []Problem {
	var out []Problem
	for _, c := range rep.Checks {
		if c.Status != doctor.Fail && c.Status != doctor.Warn {
			continue
		}
		if !matches(c.ID, ids) {
			continue
		}
		out = append(out, Problem{ID: c.ID, Title: c.Title, Status: string(c.Status),
			Detail: c.Detail, Fix: c.Fix, Doc: c.Doc})
	}
	return out
}

// hasFail reports whether one of the checks failed.
func hasFail(rep doctor.Report, ids ...string) bool {
	return hasStatus(rep, doctor.Fail, ids)
}

// hasWarn reports whether one of the checks warned.
func hasWarn(rep doctor.Report, ids ...string) bool {
	return hasStatus(rep, doctor.Warn, ids)
}

func hasStatus(rep doctor.Report, st doctor.Status, ids []string) bool {
	for _, c := range rep.Checks {
		if c.Status == st && matches(c.ID, ids) {
			return true
		}
	}
	return false
}

// matches reports whether a check id matches one of the patterns (exactly or
// as a prefix).
func matches(id string, pats []string) bool {
	for _, p := range pats {
		if id == p || strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

// rootCommands returns the commands of the pending root setup steps with the
// given ids. bernie prints them; it never runs them.
func rootCommands(steps []setup.Step, ids ...string) []string {
	var out []string
	for _, s := range steps {
		if !s.Root || s.Done || !contains(ids, s.ID) {
			continue
		}
		out = append(out, s.Commands...)
	}
	return out
}

// setupWhy returns the note of a setup step: why the root command is what
// it is, and what the operator has to do after it. It is empty when the
// step has no note.
func setupWhy(steps []setup.Step, ids ...string) string {
	for _, s := range steps {
		if !contains(ids, s.ID) {
			continue
		}
		if s.Why != "" {
			return s.Why
		}
	}
	return ""
}

// pendingSetup returns the plan steps with the given ids that are not done
// and need no root.
func pendingSetup(steps []setup.Step, ids []string) []setup.Step {
	var out []setup.Step
	for _, s := range steps {
		if !contains(ids, s.ID) || s.Done || s.Root {
			continue
		}
		out = append(out, s)
	}
	return out
}

// setupSteps returns the ids of the plan steps with the given ids.
func setupSteps(steps []setup.Step, ids []string) []string {
	var out []string
	for _, s := range steps {
		if contains(ids, s.ID) {
			out = append(out, s.ID)
		}
	}
	return out
}

// setupStep returns the plan step with the given id, or nil.
func setupStep(steps []setup.Step, id string) *setup.Step {
	for i := range steps {
		if steps[i].ID == id {
			return &steps[i]
		}
	}
	return nil
}

// applyCommand is the exact command that runs the given setup steps.
func applyCommand(steps []setup.Step) string {
	cmd := "bernie setup apply"
	for _, s := range steps {
		cmd += " --step " + s.ID
	}
	return cmd
}

// missingInputOf returns the required input of the chosen recipe that has no
// value, or "".
func missingInputOf(f *Facts) string {
	for _, e := range f.Recipes.Recipes {
		if e.ID != f.Recipe {
			continue
		}
		return missingInput(e, nil)
	}
	return ""
}

// alternatives names the other recipes that fit this host.
func alternatives(f *Facts) string {
	var out []string
	for _, id := range f.Fits {
		if id == f.Recipe {
			continue
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return ""
	}
	return "Also fits this host: " + strings.Join(out, ", ") + "."
}

// lastExited returns the last run that is not running, or nil.
func (f *Facts) lastExited() *Run {
	for i := len(f.Runs) - 1; i >= 0; i-- {
		if f.Runs[i].State == "exited" {
			return &f.Runs[i]
		}
	}
	return nil
}

// joinDetail puts two texts together.
func joinDetail(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " " + b
}

// contains reports whether a list holds a value.
func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// humanBytes is a size in bytes as a short string (1.8 MB).
func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}
