package recipe

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// set builds a Set from inline YAML files keyed by id.
func set(t *testing.T, files map[string]string) Set {
	t.Helper()
	s := Set{Recipes: map[string]*Recipe{}}
	for id, body := range files {
		r, err := Parse("test/"+id+".yaml", []byte(body))
		if err != nil {
			t.Fatalf("parse %s: %v", id, err)
		}
		s.Recipes[id] = r
	}
	return s
}

const leaf = `schema: 1
name: Leaf
status: experimental
model: {source: org/model}
requires: {arch: [gfx1030], gpus: 2}
`

func TestResolveMergeRules(t *testing.T) {
	s := set(t, map[string]string{
		"_a": `schema: 1
env: {A: "1", KEEP: "a", DROP: "x"}
args: {tensor-parallel-size: 2, compilation-config: {mode: 3, cudagraph_capture_sizes: [1, 2]}, enforce-eager: true}
tags: [one, two]
`,
		"_b": `schema: 1
env: {A: "2", B: "b"}
args: {compilation-config: {cudagraph_mode: FULL}}
`,
		"leaf": leaf + `extends: [_a, _b]
env: {DROP: null}
args: {compilation-config: {cudagraph_capture_sizes: [3, 6]}, enforce-eager: null}
tags: [three]
`,
	})
	r, err := s.Resolve("leaf")
	if err != nil {
		t.Fatal(err)
	}
	// Later parents win; the file itself wins last.
	if r.Env["A"] != "2" || r.Env["B"] != "b" || r.Env["KEEP"] != "a" {
		t.Errorf("env merge: %v", r.Env)
	}
	// A null under env stays as an unset marker.
	if v, ok := r.Env["DROP"]; !ok || v != nil {
		t.Errorf("env DROP = %v, %v; want nil marker", v, ok)
	}
	// A null elsewhere deletes the key.
	if _, ok := r.Args["enforce-eager"]; ok {
		t.Errorf("args.enforce-eager not deleted: %v", r.Args)
	}
	cc := r.Args["compilation-config"].(map[string]any)
	if cc["cudagraph_mode"] != "FULL" || cc["mode"] == nil {
		t.Errorf("nested maps must merge: %v", cc)
	}
	if got := len(cc["cudagraph_capture_sizes"].([]any)); got != 2 {
		t.Errorf("lists replace: got %v", cc["cudagraph_capture_sizes"])
	}
	if !reflect.DeepEqual(r.Tags, []string{"three"}) {
		t.Errorf("tags = %v; lists replace", r.Tags)
	}
	if !reflect.DeepEqual(r.Extends, []string{"_a", "_b"}) || r.ID != "leaf" {
		t.Errorf("identity not kept: id %s extends %v", r.ID, r.Extends)
	}
}

func TestResolveErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		id    string
		want  string
	}{
		{"cycle", map[string]string{
			"x": "schema: 1\nextends: [y]\n",
			"y": "schema: 1\nextends: [x]\n",
		}, "x", "extends cycle: x -> y -> x"},
		{"self", map[string]string{"x": "schema: 1\nextends: [x]\n"}, "x", "extends cycle"},
		{"missing parent", map[string]string{"x": "schema: 1\nextends: [_nope]\n"}, "x", `x extends unknown recipe "_nope"`},
		{"unknown id", map[string]string{}, "zzz", `unknown recipe "zzz"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := set(t, tt.files).Resolve(tt.id)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestParseRejectsSchemaErrors(t *testing.T) {
	tests := []struct{ body, want string }{
		{"schema: 1\nmodle: {source: x}\n", "unknown field"},
		{"schema: 1\nrequires: {gpus: four}\n", "requires.gpus"},
		{"- a\n- b\n", "must be a YAML map"},
		{"schema: 1\nenv: {A: [1\n", "yaml"},
	}
	for _, tt := range tests {
		_, err := Parse("t/x.yaml", []byte(tt.body))
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
			t.Errorf("Parse(%q) err = %v, want %q", tt.body, err, tt.want)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string // substrings; nil = valid
	}{
		{"valid", leaf + "args: {tensor-parallel-size: 2}\n", nil},
		{"world size", leaf + "args: {tensor-parallel-size: 2, pipeline-parallel-size: 2}\n", []string{"tensor x pipeline x data parallel size is 4"}},
		{"unknown input", leaf + "env: {X: \"${NOPE}\"}\n", []string{"unknown input ${NOPE}"}},
		{"declared input", leaf + "inputs: {DIR: {path: true}}\nenv: {X: \"${DIR}/a\"}\n", nil},
		{"escaped ref", leaf + "env: {X: \"$${HOME}\"}\n", nil},
		{"reserved arg", leaf + "args: {model: x, served-model-name: y}\n", []string{"args.model: use model.source", "args.served-model-name: use model.served_name"}},
		{"arg dashes", leaf + "args: {--port: 1}\n", []string{"write flags without '--'"}},
		{"env name", leaf + "env: {1BAD: x}\n", []string{"env.1BAD"}},
		{"env map value", leaf + "env: {A: {b: 1}}\n", []string{"env.A: want a string"}},
		{"required fields", "schema: 1\n", []string{"name: required", "status: required", "model.source: required", "requires.arch: required", "requires.gpus: required"}},
		{"container without image", leaf + "container: {shm_size: 8g}\n", []string{"container.image: required"}},
		{"device access", leaf + "container: {image: img, device_access: cdi}\n", nil},
		{"bad device access", leaf + "container: {image: img, device_access: gpu}\n", []string{"container.device_access: want one of classic, cdi, auto"}},
		{"bad status", strings.Replace(leaf, "experimental", "great", 1), []string{"status: want one of"}},
		{"schema 2", strings.Replace(leaf, "schema: 1", "schema: 2", 1), []string{"schema: want 1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Parse("t/x.yaml", []byte(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			errs := Validate(r)
			if tt.want == nil && len(errs) > 0 {
				t.Fatalf("want valid, got %v", errs)
			}
			all := strings.Join(errs, "\n")
			for _, w := range tt.want {
				if !strings.Contains(all, w) {
					t.Errorf("missing %q in:\n%s", w, all)
				}
			}
		})
	}
	// A fragment does not need the launch fields.
	r, _ := Parse("t/_frag.yaml", []byte("schema: 1\nenv: {A: \"1\"}\n"))
	if errs := Validate(r); len(errs) > 0 {
		t.Errorf("fragment: %v", errs)
	}
	r, _ = Parse("t/Bad_Id.yaml", []byte(leaf))
	if errs := Validate(r); len(errs) == 0 || !strings.Contains(errs[0], "id \"Bad_Id\"") {
		t.Errorf("bad id: %v", errs)
	}
}

func TestValidateRequiresArch(t *testing.T) {
	for _, arch := range []string{"[gfx1030]", "[gfx103, gfx11]", "[gfx90a, gfx942, gfx1201]"} {
		s := set(t, map[string]string{"m": strings.Replace(leaf, "[gfx1030]", arch, 1)})
		r, _ := s.Resolve("m")
		if errs := Validate(r); len(errs) != 0 {
			t.Errorf("arch %s: %v", arch, errs)
		}
	}
	for _, arch := range []string{"[rdna2]", "[GFX1030]", "[\"\"]", "[gfx]", "[gfx10 30]"} {
		s := set(t, map[string]string{"m": strings.Replace(leaf, "[gfx1030]", arch, 1)})
		r, _ := s.Resolve("m")
		errs := Validate(r)
		if len(errs) == 0 || !strings.Contains(strings.Join(errs, ";"), "requires.arch") {
			t.Errorf("arch %s: errors = %v", arch, errs)
		}
	}
}

func TestWhyMergeAndPlan(t *testing.T) {
	s := set(t, map[string]string{
		"_a": `schema: 1
env: {A: "1", B: "2"}
args: {max-num-seqs: 4}
why:
  env.A: "parent A"
  env.B: "parent B"
`,
		"_b": `schema: 1
why:
  env.B: "later B"
  args.max-num-seqs: "later sequence cap"
`,
		"leaf": leaf + `extends: [_a, _b]
env: {A: "1", B: "2"}
args: {max-num-seqs: 4}
why:
  env.A: "leaf A"
`,
	})
	r, err := s.Resolve("leaf")
	if err != nil {
		t.Fatal(err)
	}
	if r.Why["env.A"] != "leaf A" || r.Why["env.B"] != "later B" || r.Why["args.max-num-seqs"] != "later sequence cap" {
		t.Fatalf("merged why = %#v", r.Why)
	}
	p, err := BuildPlan(r, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Setting{
		{Setting: "args.max-num-seqs", Value: "4", Why: "later sequence cap"},
		{Setting: "env.A", Value: "1", Why: "leaf A"},
		{Setting: "env.B", Value: "2", Why: "later B"},
	}
	if !reflect.DeepEqual(p.Settings, want) {
		t.Fatalf("settings:\n got %#v\nwant %#v", p.Settings, want)
	}
	if p.Env["A"] != "1" || p.Env["B"] != "2" {
		t.Fatalf("env = %v", p.Env)
	}
	wantArgs := []string{"org/model", "--max-num-seqs", "4"}
	if !reflect.DeepEqual(p.Args, wantArgs) {
		t.Fatalf("args = %v", p.Args)
	}
}

func TestWhyNullRemovesInheritedReason(t *testing.T) {
	s := set(t, map[string]string{
		"_a":   "schema: 1\nenv: {A: \"1\"}\nwhy: {env.A: \"parent A\"}\n",
		"_off": "schema: 1\nwhy: {env.A: null}\n",
		"leaf": leaf + "extends: [_a, _off]\nenv: {A: \"1\"}\n",
	})
	r, err := s.Resolve("leaf")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Why["env.A"]; ok {
		t.Fatalf("inherited reason was not removed: %#v", r.Why)
	}
}

func TestPlanJSONSettingsShape(t *testing.T) {
	r, err := Parse("t/why.yaml", []byte(`schema: 1
name: Why
status: experimental
model: {source: org/model}
requires: {arch: [gfx1030], gpus: 1}
env: {MODE: "fast"}
args: {max-num-seqs: 4}
why:
  env.MODE: "Selects the fast path."
  args.max-num-seqs: "Caps the batch."
`))
	if err != nil {
		t.Fatal(err)
	}
	p, err := BuildPlan(r, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Env      map[string]string `json:"env"`
		Args     []string          `json:"args"`
		Settings []Setting         `json:"settings"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Env["MODE"] != "fast" || !reflect.DeepEqual(doc.Args, []string{"org/model", "--max-num-seqs", "4"}) {
		t.Fatalf("legacy fields changed: env=%v args=%v", doc.Env, doc.Args)
	}
	want := []Setting{
		{Setting: "args.max-num-seqs", Value: "4", Why: "Caps the batch."},
		{Setting: "env.MODE", Value: "fast", Why: "Selects the fast path."},
	}
	if !reflect.DeepEqual(doc.Settings, want) {
		t.Fatalf("settings:\n got %#v\nwant %#v", doc.Settings, want)
	}
}

func TestPlanWithoutWhyKeepsLegacyJSON(t *testing.T) {
	r, err := Parse("t/legacy.yaml", []byte(`schema: 1
name: Legacy
status: experimental
model: {source: org/model}
requires: {arch: [gfx1030], gpus: 1}
args: {tensor-parallel-size: 1}
`))
	if err != nil {
		t.Fatal(err)
	}
	p, err := BuildPlan(r, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"recipe":"legacy","name":"legacy","mode":"host","entrypoint":"cli","model":"org/model","host":"","port":8000,"env":{"HF_HUB_OFFLINE":"0","TRANSFORMERS_OFFLINE":"0"},"args":["org/model","--tensor-parallel-size","1"],"gpus":[],"hip_devices":"","gpu_select_reason":"no GPU data: HIP_VISIBLE_DEVICES not set","pull":false,"path_inputs":[],"warnings":["no GPU data: vLLM uses every visible GPU; set env.HIP_VISIBLE_DEVICES"]}`
	if string(b) != want {
		t.Fatalf("legacy plan JSON changed:\n got %s\nwant %s", b, want)
	}
}

func TestValidateWhy(t *testing.T) {
	r, err := Parse("t/why.yaml", []byte(`schema: 1
name: Why
status: experimental
model: {source: org/model}
requires: {arch: [gfx1030], gpus: 1}
env: {MODE: "fast"}
why:
  env.MISSING: "Names no setting."
  args.missing: "Names no flag."
  bad.path: "Uses an unknown namespace."
  env.MODE: ""
`))
	if err != nil {
		t.Fatal(err)
	}
	errs := strings.Join(Validate(r), "\n")
	for _, want := range []string{"recipe has no env.MISSING setting", "recipe has no args.missing setting", "want env.NAME, args.FLAG or tuning.tunableop_profile", "reason must not be empty"} {
		if !strings.Contains(errs, want) {
			t.Errorf("missing %q in:\n%s", want, errs)
		}
	}
}
