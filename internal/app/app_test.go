package app

import (
	"encoding/json"
	"testing"

	"github.com/opengfx1030/bernie/internal/setup"
)

func TestSetupPending(t *testing.T) {
	steps := []setup.Step{
		{ID: "groups", Root: true},
		{ID: "venv.create"},
		{ID: "venv.torch", Done: true},
		{ID: "vllm.build"},
	}
	ids := func(s []setup.Step) (out []string) {
		for _, x := range s {
			out = append(out, x.ID)
		}
		return
	}
	if got := ids(SetupPending(steps, nil)); len(got) != 2 || got[0] != "venv.create" || got[1] != "vllm.build" {
		t.Errorf("all: got %v, want the not-done non-root steps", got)
	}
	if got := ids(SetupPending(steps, []string{"vllm.build", "groups", "venv.torch"})); len(got) != 1 || got[0] != "vllm.build" {
		t.Errorf("only: got %v, want [vllm.build]", got)
	}
}

// The documents keep the key order of the map they replaced (sorted keys), so
// `--json` output stays byte-identical.
func TestDocumentKeyOrder(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{StopResult{Name: "r", Stopped: true}, `{"name":"r","stopped":true}`},
		{PowerDesiredResult{Changed: true, File: "power.yaml", Watts: 180}, `{"changed":true,"file":"power.yaml","watts":180,"gpu_indexes":null,"selectors":null}`},
		{NewLogReport(nil), `{"matches":[]}`},
	}
	for _, c := range cases {
		b, err := json.Marshal(c.v)
		if err != nil || string(b) != c.want {
			t.Errorf("got %s (%v), want %s", b, err, c.want)
		}
	}
}
