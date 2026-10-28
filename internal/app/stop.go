package app

import (
	"context"

	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// StopResult is the `stop --json` document.
type StopResult struct {
	Name    string `json:"name"`
	Stopped bool   `json:"stopped"`
}

// Stop stops a run (the only running one when name is ""). Name echoes the
// argument, as given.
func Stop(ctx context.Context, name string) (StopResult, error) {
	if err := runner.Stop(ctx, name); err != nil {
		return StopResult{}, err
	}
	return StopResult{Name: name, Stopped: true}, nil
}
