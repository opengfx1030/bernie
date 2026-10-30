package mcp

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/setup"
)

// This file is the check of the venv as an MCP tool. `setup verify` runs
// the check of the venv again, even when its postcondition holds, and
// records the run (spec §6.4). The tool returns the same document as
// `bernie setup verify --json`: the record of the run with the evidence
// and the components of the check.
//
// The tool is mutating: it starts the Python of the venv, which imports
// torch and vLLM and touches the GPU, and it writes the record of the run
// under the state directory.

// setupVerifyIn is the input of setup_verify: the venv to check and the
// mode of the plan. The check needs a host plan.
type setupVerifyIn struct {
	Mode string `json:"mode,omitempty" jsonschema:"host or container (default: config mode, else both)"`
	Venv string `json:"venv,omitempty" jsonschema:"venv directory for host mode"`
	confirmToken
}

// registerSetupVerify wires setup_verify into the catalog. It exists only
// when the write gate is open.
func registerSetupVerify(s *Server) {
	addTool[setupVerifyIn, Outcome[setupRunResult]](s, spec{
		name:  "setup_verify",
		desc:  "Check the venv again (bernie setup verify): the versions of the main parts, the fork ops and the libraries. It runs the check even when it passed before, it starts the Python of the venv, and it records a run with the new evidence." + twoCalls,
		write: true, destructive: false, idempotent: false, openWorld: true,
	}, func(sc *jsonschema.Schema) {
		prop(sc, "mode").Enum = enum("host", "container")
		tokenSchema(sc)
	}, s.setupVerify)
}

// setupVerify checks the venv again: it runs the check of the venv even
// when its postcondition holds and records the run with the new evidence
// (spec §6.4). The plan is the step the check runs: the operator sees what
// is about to start before the token confirms it.
func (s *Server) setupVerify(ctx context.Context, c *call, in setupVerifyIn) (any, error) {
	opts := app.SetupOptions{Mode: in.Mode, Venv: in.Venv, Mirror: true}
	plan := func() (any, error) {
		steps, err := app.SetupPlan(ctx, opts)
		if err != nil {
			return nil, err
		}
		forced, err := app.SetupForceVerify(steps)
		if err != nil {
			return nil, err
		}
		run := []setup.Step{}
		for _, st := range forced {
			if st.ID == setup.StepVenvVerify {
				run = append(run, st)
			}
		}
		return setupPlanDoc{Run: run, Manual: []setup.Step{}}, nil
	}
	token := in.ConfirmToken
	in.ConfirmToken = ""
	return execute(s, c, mutation[setupRunResult]{tool: "setup_verify", token: token, args: in, plan: plan, lock: &s.mutate,
		run: func(ctx context.Context) (*setupRunResult, error) {
			res, err := app.SetupVerify(ctx, opts, c.progress())
			out := &setupRunResult{}
			if res != nil {
				out.Steps, out.Run = res.Steps, res.Run
				if res.Run != nil {
					out.RunID = res.Run.ID
				}
			}
			return out, err
		}})
}
