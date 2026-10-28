package flow

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

func TestNextCommand(t *testing.T) {
	one := []gpu.GPU{{GFX: "gfx1030"}}
	fail := doctor.Check{Status: doctor.Fail, Title: "/dev/kfd is readable and writable"}
	warn := doctor.Check{Status: doctor.Warn, Title: "iommu=pt"}
	cases := []struct {
		name string
		s    Summary
		want []string
	}{
		{"no GPU: look at the driver", Summary{}, []string{"bernie doctor   # no AMD GPU found: check /dev/kfd and the amdgpu driver"}},
		{"fitting recipe: up", Summary{GPUs: one, Best: "r1"}, []string{"bernie up   # serves r1"}},
		{"no fitting recipe", Summary{GPUs: one}, []string{"bernie recipe list --all   # no recipe fits this host"}},
		{"failed check goes first", Summary{GPUs: one, Best: "r1", Problems: []doctor.Check{warn, fail}},
			[]string{"bernie doctor   # /dev/kfd is readable and writable", "bernie up   # serves r1"}},
		{"warnings alone do not add doctor", Summary{GPUs: one, Best: "r1", Problems: []doctor.Check{warn}}, []string{"bernie up   # serves r1"}},
		{"running: dash and bench instead of up", Summary{GPUs: one, Best: "r1", Runs: []runner.Status{{State: "running"}}},
			[]string{"bernie dash", "bernie bench"}},
		{"starting counts as running", Summary{GPUs: one, Best: "r1", Runs: []runner.Status{{State: "starting"}}},
			[]string{"bernie dash", "bernie bench"}},
		{"exited run is not running", Summary{GPUs: one, Best: "r1", Runs: []runner.Status{{State: "exited"}}},
			[]string{"bernie up   # serves r1"}},
	}
	for _, c := range cases {
		if got := next(&c.s); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: next = %q, want %q", c.name, got, c.want)
		}
	}
}

// Snapshot probes the real host; on one without GPUs (CI) the document must
// still be complete: empty lists, never nulls, and the doctor as next step.
func TestSnapshotWithoutGPUs(t *testing.T) {
	if g, _ := gpu.List("/"); len(g) > 0 {
		t.Skip("host has GPUs")
	}
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	s, err := Snapshot(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if s.GPUs == nil || s.Runs == nil || s.Problems == nil || s.Fits == nil {
		t.Errorf("nil slice in %+v", s)
	}
	if len(s.Next) != 1 || !strings.HasPrefix(s.Next[0], "bernie doctor") {
		t.Errorf("next = %q", s.Next)
	}
}

func TestUpBlockedWithoutGPUs(t *testing.T) {
	if g, _ := gpu.List("/"); len(g) > 0 {
		t.Skip("host has GPUs")
	}
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	res, err := Up(context.Background(), UpOptions{Yes: true}, &bytes.Buffer{})
	var b *Blocked
	if res != nil || !errors.As(err, &b) {
		t.Fatalf("Up = %v, %v; want *Blocked", res, err)
	}
	if b.Reason != "no AMD GPU found" || !reflect.DeepEqual(b.Commands, []string{"bernie doctor"}) {
		t.Errorf("blocked = %+v", b)
	}
}

func fits(id string, prio int, inputs map[string]recipe.Input) recipe.ListEntry {
	return recipe.ListEntry{ID: id, Priority: prio, Inputs: inputs, Fit: recipe.Fit{Status: recipe.FitYes}}
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
	// Nothing ready: the best fitting recipe, so up can name the missing input.
	only := recipe.Listing{Recipes: []recipe.ListEntry{fits("needs", 1, needsModel)}}
	if got := pick(only, nil); got != "needs" {
		t.Errorf("pick = %q, want needs", got)
	}
	if got := pick(recipe.Listing{}, nil); got != "" {
		t.Errorf("pick of nothing = %q", got)
	}
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

func TestConfirmFrom(t *testing.T) {
	for in, want := range map[string]bool{"y\n": true, "YES\n": true, " yes \n": true, "n\n": false, "\n": false, "": false, "maybe\n": false, "y": true} {
		var out bytes.Buffer
		if got := ConfirmFrom(strings.NewReader(in), &out)("go?"); got != want {
			t.Errorf("answer %q = %v, want %v", in, got, want)
		}
		if out.String() != "go? [y/N] " {
			t.Errorf("prompt = %q", out.String())
		}
	}
}
