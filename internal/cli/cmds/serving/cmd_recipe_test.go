package serving

import (
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
)

func TestPrintPlanShowsSettingReasons(t *testing.T) {
	p := &recipe.Plan{
		Recipe: "demo", Name: "demo", Mode: recipe.ModeHost, Entrypoint: "cli",
		Model: "org/model", Port: 8000, Env: map[string]string{"MODE": "fast"},
		Args: []string{"org/model"}, Settings: []recipe.Setting{
			{Setting: "env.MODE", Value: "fast", Why: "Selects the fast path."},
		},
		Exec: &recipe.Exec{Argv: []string{"vllm", "serve", "org/model"}},
	}
	var out strings.Builder
	printPlan(&out, p)
	got := out.String()
	if !strings.Contains(got, "# env.MODE=fast\n#   why: Selects the fast path.\n") {
		t.Fatalf("plan does not show the reason:\n%s", got)
	}
}
