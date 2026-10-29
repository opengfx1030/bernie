package onboarding

import "strings"

// The kinds of the primary action.
const (
	// PrimaryRoot means that only the operator can move the path on: a
	// step needs root.
	PrimaryRoot = "root"
	// PrimarySetup means that bernie can do everything that is missing.
	PrimarySetup = "setup"
	// PrimaryServe means that everything is in place and the server has to
	// start.
	PrimaryServe = "serve"
	// PrimaryVerify means that the server runs and has to answer a
	// question.
	PrimaryVerify = "verify"
	// PrimaryDone means that the path is complete.
	PrimaryDone = "done"
)

// Primary is the one action of the first run: the button the reader presses.
// The CLI, the MCP server and the page read this one record, so they cannot
// disagree about what the reader does next.
type Primary struct {
	// Kind is what the action does: root, setup, serve, verify or done.
	Kind string `json:"kind"`
	// Label is the text of the button.
	Label string `json:"label"`
	// Sentence says what the action does and who does it.
	Sentence string `json:"sentence"`
	// Command is the command that does the same work in a terminal.
	Command string `json:"command,omitempty"`
	// RootCommands are the commands the operator runs as root. They are
	// only there when Kind is root.
	RootCommands []string `json:"root_commands,omitempty"`
	// Steps are the ids of the steps the action covers. An action that
	// covers the whole path names them all.
	Steps []string `json:"steps,omitempty"`
}

// The labels of the primary action. They name what the button does, never
// only "Next" or "Continue" (the rules of the Setup tab).
const (
	primaryLabelRoot   = "Copy the root command"
	primaryLabelSetup  = "Set up this box"
	primaryLabelServe  = "Start the server"
	primaryLabelVerify = "Ask the server a question"
	primaryLabelDone   = "Ask the server a question"
)

// primary derives the one action of the first run from the checklist.
func primary(doc *Document) *Primary {
	if doc.Next == "" {
		return &Primary{
			Kind: PrimaryDone, Label: primaryLabelDone,
			Sentence: "This box serves a model. Ask it a question.",
		}
	}
	// A step that needs root comes first: nobody else can do it.
	for _, s := range doc.Steps {
		if len(s.RootCommands) == 0 {
			continue
		}
		if s.State != StateTodo && s.State != StateBlocked && s.State != StateRunning {
			continue
		}
		return &Primary{
			Kind: PrimaryRoot, Label: primaryLabelRoot,
			Sentence:     "Copy the command, run it as root in a terminal, then press Re-check.",
			Command:      s.RootCommands[0],
			RootCommands: s.RootCommands,
			Steps:        []string{s.ID},
		}
	}
	// A step that only the operator can do, without root: the recipe that
	// does not fit, or one that needs an input.
	for _, s := range doc.Steps {
		if s.State != StateBlocked || len(s.RootCommands) > 0 {
			continue
		}
		return &Primary{
			Kind: PrimaryRoot, Label: s.Action,
			Sentence: nextActionSentence(s),
			Command:  s.Command,
			Steps:    []string{s.ID},
		}
	}
	// Everything that is missing is bernie's work: one action does it all.
	if ids := pendingRunnables(doc); len(ids) > 0 {
		return &Primary{
			Kind: PrimarySetup, Label: primaryLabelSetup,
			Sentence: setupSentence(ids),
			Command:  "bernie up",
			Steps:    ids,
		}
	}
	for _, s := range doc.Steps {
		if s.ID != StepServe {
			continue
		}
		if s.Serving {
			return &Primary{
				Kind: PrimaryVerify, Label: primaryLabelVerify,
				Sentence: "The server answers. Ask it a question to be sure.",
				Command:  "bernie smoke",
				Steps:    []string{StepVerify},
			}
		}
		return &Primary{
			Kind: PrimaryServe, Label: primaryLabelServe,
			Sentence: "Everything is in place. Start the server and it answers.",
			Command:  "bernie up",
			Steps:    []string{StepServe},
		}
	}
	return &Primary{
		Kind: PrimaryDone, Label: primaryLabelDone,
		Sentence: "This box serves a model. Ask it a question.",
	}
}

// pendingRunnables are the ids of the steps of the path that bernie can do
// itself and that are not done yet.
func pendingRunnables(doc *Document) []string {
	var out []string
	for _, s := range doc.Steps {
		if s.State != StateTodo || !s.CanRun {
			continue
		}
		if s.ID == StepBench || s.ID == StepTuning || s.ID == StepPower {
			continue
		}
		out = append(out, s.ID)
	}
	return out
}

// setupSentence says what the setup action does, in the words of the steps
// it covers: "bernie builds the Python environment, downloads the model and
// starts the server."
func setupSentence(ids []string) string {
	var parts []string
	for _, id := range ids {
		switch id {
		case StepPythonEnv:
			parts = append(parts, "builds the Python environment")
		case StepModel:
			parts = append(parts, "downloads the model")
		case StepServe:
			parts = append(parts, "starts the server")
		case StepVerify:
			parts = append(parts, "asks the server a question")
		default:
			parts = append(parts, "does the "+id+" step")
		}
	}
	if len(parts) == 0 {
		return "bernie does the rest of the path."
	}
	return "bernie " + strings.Join(parts, ", ") + "."
}
