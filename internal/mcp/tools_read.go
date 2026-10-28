package mcp

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/flow"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// Input types of the read-only tools. A field without omitempty is required.

type noIn struct{}

type venvIn struct {
	Venv string `json:"venv,omitempty" jsonschema:"host-mode venv directory (default: BERNIE_VENV, the config venv, then VIRTUAL_ENV)"`
}

type topoIn struct {
	Bandwidth bool   `json:"bandwidth,omitempty" jsonschema:"also measure GPU-to-GPU copy bandwidth; this loads the GPUs"`
	Venv      string `json:"venv,omitempty" jsonschema:"venv for the torch peer-copy probe when rocm-bandwidth-test is missing"`
}

type doctorIn struct {
	Categories []string `json:"categories,omitempty" jsonschema:"limit the checks to these categories (default: all)"`
	Venv       string   `json:"venv,omitempty" jsonschema:"venv for the python checks (default: BERNIE_VENV, the config venv, then VIRTUAL_ENV)"`
}

type runIn struct {
	Run string `json:"run" jsonschema:"name of a run (ps)"`
}

type recipeListIn struct {
	All bool `json:"all,omitempty" jsonschema:"include recipes that do not fit this host"`
}

type idIn struct {
	ID string `json:"id" jsonschema:"recipe id (recipe_list)"`
}

type launchIn struct {
	ID          string   `json:"id" jsonschema:"recipe id (recipe_list)"`
	Set         []string `json:"set,omitempty" jsonschema:"overrides, each INPUT=VALUE or section.key=VALUE with a YAML value"`
	Mode        string   `json:"mode,omitempty" jsonschema:"host or container (default: config, else container when the recipe has an image and an engine is usable)"`
	Name        string   `json:"name,omitempty" jsonschema:"run name (default: the recipe id)"`
	Venv        string   `json:"venv,omitempty" jsonschema:"host-mode venv directory"`
	Engine      string   `json:"engine,omitempty" jsonschema:"container engine (default: config, else the first usable)"`
	Profile     string   `json:"profile,omitempty" jsonschema:"force a TunableOp profile"`
	NoTunableOp bool     `json:"no_tunableop,omitempty" jsonschema:"do not set TunableOp"`
	Tuning      bool     `json:"tuning,omitempty" jsonschema:"TunableOp online tuning into the per-user rows file"`
}

func (l launchIn) prepare() runner.PrepareOptions {
	return runner.PrepareOptions{Recipe: l.ID, Sets: l.Set, Mode: l.Mode, Name: l.Name, Venv: l.Venv, Engine: l.Engine,
		Profile: l.Profile, NoTunableOp: l.NoTunableOp, Tuning: l.Tuning}
}

type logsIn struct {
	Name string `json:"name,omitempty" jsonschema:"run name (default: the only run)"`
	Tail int    `json:"tail,omitempty" jsonschema:"last N lines"`
}

type benchShowIn struct {
	ID string `json:"id" jsonschema:"bench result id (bench_list)"`
}

type benchCompareIn struct {
	A string `json:"a" jsonschema:"baseline bench result id (bench_list)"`
	B string `json:"b" jsonschema:"candidate bench result id (bench_list)"`
}

// Limits of the logs tool.
const (
	defaultTail = 100
	maxTail     = 10000
	maxLogBytes = 256 << 10
)

// namePattern is a run name or recipe id: no path separators, no leading dot.
const namePattern = `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`

func prop(s *jsonschema.Schema, name string) *jsonschema.Schema { return s.Properties[name] }

func enum(vals ...string) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = v
	}
	return out
}

func patternOf(names ...string) func(*jsonschema.Schema) {
	return func(s *jsonschema.Schema) {
		for _, n := range names {
			prop(s, n).Pattern = namePattern
		}
	}
}

// launchSchema refines the inputs shared by recipe_plan and serve.
func launchSchema(s *jsonschema.Schema) {
	patternOf("id", "name")(s)
	prop(s, "set").Items.MinLength = jsonschema.Ptr(1)
	prop(s, "mode").Enum = enum("host", "container")
	prop(s, "engine").Enum = enum("docker", "podman")
}

func registerRead(s *Server) {
	ro := func(name, desc string) spec { return spec{name: name, desc: desc} }

	addTool[venvIn, *flow.Summary](s, ro("status", "Overview of this host: GPUs, doctor summary, problems, the best fitting recipe, runs and the next commands (bernie status)."), nil,
		func(ctx context.Context, _ *call, in venvIn) (any, error) { return flow.Snapshot(ctx, in.Venv) })

	addTool[noIn, app.HWReport](s, ro("hw", "Hardware and OS inventory (bernie hw)."), nil,
		func(ctx context.Context, _ *call, _ noIn) (any, error) { return app.HW(ctx) })

	addTool[topoIn, topo.Topology](s, ro("topo", "PCIe tree, P2P matrix and link classes (bernie topo). With bandwidth it runs GPU copies."), nil,
		func(ctx context.Context, _ *call, in topoIn) (any, error) {
			return app.Topo(ctx, app.TopoOptions{Bandwidth: in.Bandwidth, Venv: in.Venv})
		})

	addTool[doctorIn, doctor.Report](s, ro("doctor", "Run the host checks (bernie doctor). Each failed or warned check has a fix."),
		func(sc *jsonschema.Schema) {
			prop(sc, "categories").Items.Enum = enum(doctor.Categories...)
			prop(sc, "categories").UniqueItems = true
		},
		func(ctx context.Context, _ *call, in doctorIn) (any, error) {
			return app.Doctor(ctx, in.Categories, in.Venv)
		})

	addTool[runIn, app.LogReport](s, ro("doctor_log", "Match the log of a run against the known vLLM failures (bernie doctor --run). Returns catalog matches, not raw log text."),
		patternOf("run"),
		func(ctx context.Context, _ *call, in runIn) (any, error) { return app.RunLog(ctx, in.Run) })

	addTool[recipeListIn, recipe.Listing](s, ro("recipe_list", "Recipes with their fit to this host (bernie recipe list)."), nil,
		func(_ context.Context, _ *call, in recipeListIn) (any, error) { return app.RecipeList(in.All) })

	addTool[idIn, *recipe.Recipe](s, ro("recipe_show", "The merged recipe after extends (bernie recipe show)."), patternOf("id"),
		func(_ context.Context, _ *call, in idIn) (any, error) { return recipe.Load().Resolve(in.ID) })

	addTool[launchIn, *recipe.Plan](s, ro("recipe_plan", "The plan of a recipe on this host: exact environment, command, GPUs, TunableOp rows and the reason for each annotated setting (bernie recipe plan). It starts nothing; it may write TunableOp rows to the data directory."),
		launchSchema,
		func(ctx context.Context, _ *call, in launchIn) (any, error) { return runner.Prepare(ctx, in.prepare()) })

	addTool[noIn, []flow.Run](s, ro("ps", "Runs with state and health; an exited run carries the known failures of its log (bernie ps)."), nil,
		func(ctx context.Context, _ *call, _ noIn) (any, error) { return flow.Runs(ctx) })

	addTool[logsIn, logOut](s, ro("logs", "The last lines of a run log, wrapped as untrusted data and redacted. Prefer doctor_log for diagnosis."),
		func(sc *jsonschema.Schema) {
			patternOf("name")(sc)
			t := prop(sc, "tail")
			t.Minimum, t.Maximum = jsonschema.Ptr(1.0), jsonschema.Ptr(float64(maxTail))
			t.Default = []byte("100")
		},
		func(ctx context.Context, _ *call, in logsIn) (any, error) { return runLogs(ctx, in) })

	addTool[noIn, []bench.Result](s, ro("bench_list", "Saved benchmark results (bernie bench list)."), nil,
		func(context.Context, *call, noIn) (any, error) { return app.BenchList() })

	addTool[benchShowIn, *bench.Result](s, ro("bench_show", "One saved benchmark result (bernie bench show)."), nil,
		func(_ context.Context, _ *call, in benchShowIn) (any, error) { return bench.Get(in.ID) })

	addTool[noIn, []*suite.Info](s, ro("bench_suites", "The built-in bench suites with their digest and their groups (bernie bench suites)."), nil,
		func(_ context.Context, _ *call, _ noIn) (any, error) { return suite.List() })

	addTool[benchCompareIn, *suite.Verdict](s, ro("bench_compare", "The verdict of two saved results: better, same, worse or noisy, per metric and per cell (bernie bench compare)."), nil,
		func(_ context.Context, _ *call, in benchCompareIn) (any, error) {
			a, err := bench.Get(in.A)
			if err != nil {
				return nil, err
			}
			b, err := bench.Get(in.B)
			if err != nil {
				return nil, err
			}
			cmp, err := suite.Compare(a, b)
			if err != nil {
				return nil, err
			}
			return suite.Judge(cmp, 0), nil
		})

	addTool[venvIn, app.TunableOpState](s, ro("tunableop_status", "Loaded rocBLAS build and its matching TunableOp profile (bernie tune tunableop status)."), nil,
		func(_ context.Context, _ *call, in venvIn) (any, error) { return app.TunableOpStatus(in.Venv) })

	addTool[noIn, app.PowerServiceStatus](s, ro("power_status", "Power service state, desired caps, actual caps and drift (bernie tune power service status)."), nil,
		func(ctx context.Context, _ *call, _ noIn) (any, error) { return app.ReadPowerServiceStatus(ctx) })

	addTool[venvIn, *onboarding.Document](s, ro("onboarding_status", "The onboarding checklist of this host: the ordered steps from a bare machine to a serving model, with the step to do now (bernie status --steps)."), nil,
		func(ctx context.Context, _ *call, in venvIn) (any, error) {
			return app.Onboarding(ctx, app.OnboardingOptions{Venv: in.Venv})
		})
}
