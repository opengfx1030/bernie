// Package onboarding derives the onboarding checklist of spec §3.1 and §11:
// the ordered steps that lead from a bare machine to a serving model. The
// steps come from the real state of the host (doctor report, setup plan,
// recipes, runs), so the CLI, the dashboard and the MCP server cannot
// disagree about what is missing.
//
// The package holds three things: the facts (the state of the host, gathered
// once), the steps (derived from the facts) and the jobs (the background
// runs of a step). Every front end reads the same document.
package onboarding

// The state of a step.
//
//	done: the postcondition holds.
//	todo: something is missing and bernie can do it.
//	running: a job is doing it now.
//	blocked: only the operator can do it (it needs root).
//	skipped: the step does not apply to this host.
const (
	StateDone    = "done"
	StateTodo    = "todo"
	StateRunning = "running"
	StateBlocked = "blocked"
	StateSkipped = "skipped"
)

// The step ids, in the order of the path. They are stable: the dashboard,
// the scripts and the API use them.
const (
	StepHardware  = "hardware"
	StepAccess    = "access"
	StepROCm      = "rocm"
	StepPythonEnv = "python-env"
	StepModel     = "model"
	StepRecipe    = "recipe"
	StepServe     = "serve"
	StepVerify    = "verify"
	StepBench     = "bench"
	StepTuning    = "tuning"
	StepPower     = "power"
)

// Document is the onboarding document: the ordered steps and the step the
// operator should do now.
type Document struct {
	Next  string `json:"next"`
	Steps []Step `json:"steps"`
}

// Step is one step of the path.
type Step struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Summary string `json:"summary"`
	Detail  string `json:"detail,omitempty"`
	// Command is the exact command that does the step.
	Command string `json:"command,omitempty"`
	// CanRun says that bernie can do the step itself, without root.
	CanRun bool `json:"can_run"`
	// RootCommands are the commands the operator runs as root. bernie never
	// runs them.
	RootCommands []string `json:"root_commands,omitempty"`
	// Progress is what a running step is doing now.
	Progress *Progress `json:"progress,omitempty"`
	// Problems are the doctor checks that hold the step back.
	Problems []Problem `json:"problems,omitempty"`
}

// Progress is the progress of a running step: the phase sentence and, when
// the step knows it, the fraction of the work that is done.
type Progress struct {
	Phase string `json:"phase,omitempty"`
	Done  int64  `json:"done,omitempty"`
	Total int64  `json:"total,omitempty"`
}

// Problem is one doctor check that holds a step back.
type Problem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
	Doc    string `json:"doc,omitempty"`
}

// Build derives the onboarding document from the facts. The steps come in
// the order of the path; the last three are extras that never block.
func Build(f *Facts) *Document {
	doc := &Document{Steps: []Step{}}
	doc.Steps = append(doc.Steps,
		hardwareStep(f),
		accessStep(f),
		rocmStep(f),
		pythonStep(f),
		modelStep(f),
		recipeStep(f),
		serveStep(f),
		verifyStep(f),
		benchStep(f),
		tuningStep(f),
		powerStep(f),
	)
	doc.Next = nextStep(doc.Steps)
	return doc
}

// nextStep returns the id of the step to do now: the first step of the path
// that is not done and not skipped. The extras never hold the path.
func nextStep(steps []Step) string {
	for _, s := range steps {
		if s.State == StateDone || s.State == StateSkipped {
			continue
		}
		if s.ID == StepBench || s.ID == StepTuning || s.ID == StepPower {
			continue
		}
		return s.ID
	}
	return ""
}
