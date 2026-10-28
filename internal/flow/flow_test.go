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
	"github.com/opengfx1030/bernie/internal/onboarding"
)

func TestNextCommand(t *testing.T) {
	one := []gpu.GPU{{GFX: "gfx1030"}}
	fail := doctor.Check{ID: "gpu.kfd", Status: doctor.Fail, Title: "/dev/kfd is readable and writable",
		Fix: "load the amdgpu driver"}
	warn := doctor.Check{ID: "kernel.iommu", Status: doctor.Warn, Title: "iommu=pt"}
	cases := []struct {
		name string
		f    *onboarding.Facts
		want []string
	}{
		{"no GPU: look at the driver", &onboarding.Facts{},
			[]string{"bernie doctor   # No AMD GPU is visible to bernie."}},
		{"fitting recipe: up", &onboarding.Facts{GPUs: one, Recipe: "r1", Best: "r1"},
			[]string{"bernie up r1"}},
		{"no fitting recipe", &onboarding.Facts{GPUs: one},
			[]string{"bernie recipe list --all   # No recipe fits this host."}},
		{"failed check goes first", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Doctor: doctor.Report{Checks: []doctor.Check{warn, fail}}},
			[]string{"load the amdgpu driver   # This user cannot open the GPU device nodes."}},
		{"warnings alone do not add doctor", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Doctor: doctor.Report{Checks: []doctor.Check{warn}}},
			[]string{"bernie up r1"}},
		{"running: dash and bench instead of up", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Runs: []onboarding.Run{{Name: "r1", State: "running"}}},
			[]string{"bernie dash", "bernie bench"}},
		{"starting counts as running", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Runs: []onboarding.Run{{Name: "r1", State: "starting"}}},
			[]string{"bernie dash", "bernie bench"}},
		{"exited run is not running", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Runs: []onboarding.Run{{Name: "r1", State: "exited"}}},
			[]string{"bernie up r1"}},
	}
	for _, c := range cases {
		s := &Summary{Onboarding: onboarding.Build(c.f)}
		if got := next(s); !reflect.DeepEqual(got, c.want) {
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
	if len(s.Next) == 0 {
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
	if b.Reason != "No AMD GPU is visible to bernie." || len(b.Commands) == 0 {
		t.Errorf("blocked = %+v", b)
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
