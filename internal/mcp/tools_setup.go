package mcp

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/setup"
)

// This file is the setup plan as an MCP tool. `setup plan` is read-only:
// it probes the host and returns the ordered install steps. The tool
// returns the same document as `bernie setup plan --json` (spec §13.4):
// an array of steps, each with its id, title, mode, root flag, done
// flag, commands and why.

// setupPlanIn is the input of setup_plan. The fields are the flags of
// `bernie setup plan`.
type setupPlanIn struct {
	Mode  string `json:"mode,omitempty" jsonschema:"host or container (default: config mode, else both)"`
	Venv  string `json:"venv,omitempty" jsonschema:"host-mode venv directory"`
	Image string `json:"image,omitempty" jsonschema:"container image (default: the built-in one)"`
	Model string `json:"model,omitempty" jsonschema:"Hugging Face repo to download (adds the model.download step)"`
	ROCm  string `json:"rocm,omitempty" jsonschema:"ROCm line to install (default: the newest one)"`
}

// setupPlan implements the read-only setup_plan tool.
func (s *Server) setupPlan(ctx context.Context, _ *call, in setupPlanIn) (any, error) {
	steps, err := app.SetupPlan(ctx, app.SetupOptions{
		Mode:  in.Mode,
		Venv:  in.Venv,
		Image: in.Image,
		Model: in.Model,
		ROCm:  in.ROCm,
	})
	if err != nil {
		return nil, err
	}
	return steps, nil
}

// registerSetupTools wires setup_plan into the catalog. The tool is
// read-only: it probes the host and changes nothing.
func registerSetupTools(s *Server) {
	addTool[setupPlanIn, []setup.Step](s, spec{
		name: "setup_plan",
		desc: "The ordered install steps for this host (bernie setup plan): the venv, the wheels, the image and the model download, with the root steps bernie never runs. It changes nothing.",
	}, func(sc *jsonschema.Schema) {
		prop(sc, "mode").Enum = enum("host", "container")
	}, s.setupPlan)
}
