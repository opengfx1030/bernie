package onboarding

import (
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/setup"
)

// check is one doctor check of a fixture.
func check(id string, st doctor.Status, fix string) doctor.Check {
	return doctor.Check{ID: id, Category: strings.SplitN(id, ".", 2)[0], Status: st,
		Title: id, Fix: fix, Doc: "https://example.test/" + id}
}

// stepByID returns the step with the given id.
func stepByID(t *testing.T, doc *Document, id string) Step {
	t.Helper()
	for _, s := range doc.Steps {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no step %q in %+v", id, doc.Steps)
	return Step{}
}

// bareHost is a host without GPUs, without a venv and without a model.
func bareHost() *Facts {
	return &Facts{
		Doctor: doctor.Report{Checks: []doctor.Check{
			check("gpu.count", doctor.Fail, ""),
			check("gpu.kfd", doctor.Fail, "load the amdgpu driver"),
		}},
		Setup: []setup.Step{
			{ID: "groups", Title: "Add the user to video and render", Root: true,
				Commands: []string{"usermod -aG video,render $USER"}},
			{ID: "rocm.runtime", Title: "Install ROCm", Root: true, Commands: []string{"dnf install rocm"}},
			{ID: "venv.create", Title: "Create the venv", Commands: []string{"python3 -m venv /v"}},
			{ID: "model.download", Title: "Download the model"},
		},
	}
}

// readyHost is a host with a GPU, a venv, a model and a fitting recipe.
func readyHost() *Facts {
	return &Facts{
		GPUs:   []gpu.GPU{{HIPIndex: 0, GFX: "gfx1030", Name: "Radeon PRO V620"}},
		Doctor: doctor.Report{Checks: []doctor.Check{check("gpu.count", doctor.OK, "")}},
		Recipes: recipe.Listing{Recipes: []recipe.ListEntry{
			{ID: "qwen", Fit: recipe.Fit{Status: recipe.FitYes}},
			{ID: "other", Fit: recipe.Fit{Status: recipe.FitYes}},
		}},
		Best:   "qwen",
		Recipe: "qwen",
		Fits:   []string{"qwen", "other"},
		Setup: []setup.Step{
			{ID: "groups", Root: true, Done: true},
			{ID: "rocm.runtime", Root: true, Done: true},
			{ID: "venv.create", Done: true},
			{ID: "venv.torch", Done: true},
			{ID: "model.download", Done: true},
		},
	}
}

func TestBareHostListsThePathAndTheRootSteps(t *testing.T) {
	doc := Build(bareHost())
	if doc.Next != StepHardware {
		t.Errorf("next = %q, want %q", doc.Next, StepHardware)
	}
	hw := stepByID(t, doc, StepHardware)
	if hw.State != StateBlocked {
		t.Errorf("hardware = %q, want blocked", hw.State)
	}
	access := stepByID(t, doc, StepAccess)
	if access.State != StateBlocked || len(access.RootCommands) != 1 {
		t.Fatalf("access = %+v", access)
	}
	if !strings.Contains(access.RootCommands[0], "usermod") {
		t.Errorf("root command = %q", access.RootCommands[0])
	}
	rocm := stepByID(t, doc, StepROCm)
	if len(rocm.RootCommands) != 1 || !strings.Contains(rocm.RootCommands[0], "dnf") {
		t.Errorf("rocm root commands = %v", rocm.RootCommands)
	}
	py := stepByID(t, doc, StepPythonEnv)
	if py.State != StateTodo || !py.CanRun {
		t.Errorf("python-env = %+v", py)
	}
	if py.Command != "bernie setup apply --step venv.create" {
		t.Errorf("python-env command = %q", py.Command)
	}
	model := stepByID(t, doc, StepModel)
	if model.State != StateTodo || !model.CanRun {
		t.Errorf("model = %+v", model)
	}
	if stepByID(t, doc, StepRecipe).State != StateBlocked {
		t.Error("no recipe fits: the recipe step must block")
	}
	if stepByID(t, doc, StepVerify).State != StateSkipped {
		t.Error("no server: the verify step must be skipped")
	}
}

func TestReadyHostHasOneStepLeft(t *testing.T) {
	f := readyHost()
	f.Runs = []Run{{Name: "qwen", State: "running", URL: "http://127.0.0.1:8000", Phase: "Loading the model weights"}}
	doc := Build(f)
	if doc.Next != StepServe {
		t.Errorf("next = %q, want %q", doc.Next, StepServe)
	}
	for _, id := range []string{StepHardware, StepAccess, StepROCm, StepPythonEnv, StepModel, StepRecipe} {
		if s := stepByID(t, doc, id); s.State != StateDone {
			t.Errorf("%s = %q, want done", id, s.State)
		}
	}
	serve := stepByID(t, doc, StepServe)
	if serve.State != StateRunning || serve.Progress == nil || serve.Progress.Phase != "Loading the model weights" {
		t.Errorf("serve = %+v", serve)
	}
	verify := stepByID(t, doc, StepVerify)
	if verify.State != StateTodo || !verify.CanRun {
		t.Errorf("verify = %+v", verify)
	}
}

func TestTheRestrictionIsPartOfTheHardwareStep(t *testing.T) {
	f := readyHost()
	f.Restriction = "GPUs limited to 8 by HIP_VISIBLE_DEVICES"
	hw := stepByID(t, Build(f), StepHardware)
	if !strings.Contains(hw.Detail, "HIP_VISIBLE_DEVICES") {
		t.Errorf("hardware detail = %q", hw.Detail)
	}
}

func TestAProblemCarriesItsFixAndDoc(t *testing.T) {
	f := readyHost()
	f.Doctor.Checks = append(f.Doctor.Checks, check("gpu.groups", doctor.Fail, "usermod -aG video,render $USER"))
	access := stepByID(t, Build(f), StepAccess)
	if len(access.Problems) != 1 {
		t.Fatalf("problems = %+v", access.Problems)
	}
	p := access.Problems[0]
	if p.Status != "fail" || p.Fix == "" || p.Doc == "" {
		t.Errorf("problem = %+v", p)
	}
}

func TestPickSkipsRecipesWithUnsetRequiredInputs(t *testing.T) {
	needsModel := map[string]recipe.Input{"MODEL": {Required: true, Description: "path"}}
	withDefault := map[string]recipe.Input{"MODEL": {Required: true, Default: "/m"}}
	l := recipe.Listing{Recipes: []recipe.ListEntry{
		fits("high-needs-input", 9, needsModel),
		fits("low-ready", 1, nil),
		fits("mid-default", 5, withDefault),
		{ID: "top-but-no-fit", Priority: 99, Fit: recipe.Fit{Status: recipe.FitNo}},
	}}
	cases := []struct {
		name string
		sets []string
		want string
	}{
		{"ready recipe with the best priority", nil, "mid-default"},
		{"--set satisfies the input", []string{"MODEL=/x"}, "high-needs-input"},
		{"unrelated --set does not", []string{"MODELX=/x"}, "mid-default"},
	}
	for _, c := range cases {
		if got := pick(l, c.sets); got != c.want {
			t.Errorf("%s: pick = %q, want %q", c.name, got, c.want)
		}
	}
	only := recipe.Listing{Recipes: []recipe.ListEntry{fits("needs", 1, needsModel)}}
	if got := pick(only, nil); got != "needs" {
		t.Errorf("pick = %q, want needs", got)
	}
	if got := pick(recipe.Listing{}, nil); got != "" {
		t.Errorf("pick of nothing = %q", got)
	}
}

func fits(id string, prio int, inputs map[string]recipe.Input) recipe.ListEntry {
	return recipe.ListEntry{ID: id, Priority: prio, Inputs: inputs, Fit: recipe.Fit{Status: recipe.FitYes}}
}

func TestEngineUsable(t *testing.T) {
	h := host.Info{Engines: []host.Engine{{Name: "docker", Usable: false}, {Name: "podman", Usable: true}}}
	for want, ok := range map[string]bool{"": true, "podman": true, "docker": false, "other": false} {
		if got := engineUsable(h, want); got != ok {
			t.Errorf("engineUsable(%q) = %v, want %v", want, got, ok)
		}
	}
	if engineUsable(host.Info{}, "") {
		t.Error("no engines")
	}
}

func TestModelRepo(t *testing.T) {
	for in, want := range map[string]string{
		"org/model":      "org/model",
		"/models/x":      "",
		"${MODEL_DIR}/x": "",
		"":               "",
	} {
		if got := modelRepo(in); got != want {
			t.Errorf("modelRepo(%q) = %q, want %q", in, got, want)
		}
	}
}
