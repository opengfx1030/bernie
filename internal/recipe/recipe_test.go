package recipe

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/opengfx1030/rdnactl/internal/fixture"
	"github.com/opengfx1030/rdnactl/internal/gpu"
	"github.com/opengfx1030/rdnactl/internal/topo"
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

func TestParseSet(t *testing.T) {
	tests := []struct {
		in      string
		input   bool
		path    []string
		value   any
		wantErr string
	}{
		{in: "PLE_QUANT_DIR=/x", input: true},
		{in: "MAXLEN=32768", input: true}, // inputs stay strings
		{in: "args.max-model-len=32768", path: []string{"args", "max-model-len"}, value: int64(32768)},
		{in: "args.enforce-eager=true", path: []string{"args", "enforce-eager"}, value: true},
		{in: "args.enforce-eager=null", path: []string{"args", "enforce-eager"}, value: nil},
		{in: "env.GPU_MAX_HW_QUEUES=2", path: []string{"env", "GPU_MAX_HW_QUEUES"}, value: int64(2)},
		{in: "model.source=", path: []string{"model", "source"}, value: ""},
		{in: "args.compilation-config.mode=3", path: []string{"args", "compilation-config", "mode"}, value: int64(3)},
		{in: "noequals", wantErr: "want KEY=VALUE"},
		{in: "=x", wantErr: "want KEY=VALUE"},
		{in: "bad-input=1", wantErr: "not an input name"},
		{in: "args..x=1", wantErr: "empty path element"},
		{in: "foo.bar=1", wantErr: "unknown section"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			o, err := ParseSet(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if o.Input != tt.input {
				t.Fatalf("Input = %v", o.Input)
			}
			if tt.input {
				if o.Value != nil || o.Raw != strings.SplitN(tt.in, "=", 2)[1] {
					t.Errorf("input value: raw %q value %v", o.Raw, o.Value)
				}
				return
			}
			if !reflect.DeepEqual(o.Path, tt.path) {
				t.Errorf("Path = %v", o.Path)
			}
			if scalarString(o.Value) != scalarString(tt.value) || (o.Value == nil) != (tt.value == nil) {
				t.Errorf("Value = %#v, want %#v", o.Value, tt.value)
			}
		})
	}
}

func TestInterpolate(t *testing.T) {
	vals := map[string]string{"A": "x", "B": ""}
	tests := map[string]string{
		"plain":         "plain",
		"${A}/dir":      "x/dir",
		"${A}${A}":      "xx",
		"pre-${B}-post": "pre--post",
		"$${A}":         "${A}",
		"$${A}-${A}":    "${A}-x",
	}
	for in, want := range tests {
		if got := interpolate(in, vals); got != want {
			t.Errorf("interpolate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderArgs(t *testing.T) {
	r, err := Parse("t/x.yaml", []byte(`schema: 1
args:
  zeta: 1
  enable-prefix-caching: true
  enforce-eager: false
  gone: null
  speculative-config: {method: mtp, num_speculative_tokens: 2}
  cudagraph-sizes: [1, 2]
  host: 127.0.0.1
  ratio: 0.5
  url: "http://a/?x=1&y=<2>"
`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderArgs(r.Args)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--cudagraph-sizes", "[1,2]",
		"--enable-prefix-caching",
		"--host", "127.0.0.1",
		"--ratio", "0.5",
		"--speculative-config", `{"method":"mtp","num_speculative_tokens":2}`,
		"--url", "http://a/?x=1&y=<2>",
		"--zeta", "1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RenderArgs:\n got %q\nwant %q", got, want)
	}
}

func TestApplyAndBuildPlan(t *testing.T) {
	s := set(t, map[string]string{
		"p": `schema: 1
name: P
status: validated
entrypoint: api
model: {source: "${MODEL}", served_name: srv, revision: abc}
requires: {arch: [gfx1030], gpus: 2}
inputs:
  MODEL: {default: org/m}
  DIR: {required: true, path: true, description: table dir}
container: {image: img:1}
env: {PLE: "${DIR}", GONE: null, N: 4}
args: {tensor-parallel-size: 2, port: 9000, max-model-len: 1024}
`,
	})
	r, err := s.Resolve("p")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := BuildPlan(r, PlanOptions{}); err == nil || !strings.Contains(err.Error(), "input DIR is required (table dir): --set DIR=...") {
		t.Errorf("missing required input: %v", err)
	}
	if _, err := BuildPlan(r, PlanOptions{Sets: []string{"DIR=/d", "NOPE=1"}}); err == nil || !strings.Contains(err.Error(), "has no input NOPE (inputs: DIR, MODEL)") {
		t.Errorf("unknown input: %v", err)
	}
	if _, err := BuildPlan(r, PlanOptions{Sets: []string{"DIR=/d", "args.port=99999"}}); err == nil || !strings.Contains(err.Error(), "args.port") {
		t.Errorf("bad port: %v", err)
	}
	if _, err := BuildPlan(r, PlanOptions{Sets: []string{"DIR=/d"}, Name: "Bad Name"}); err == nil {
		t.Error("bad run name accepted")
	}

	// Config defaults come first; a later --set wins.
	sets := []string{"args.max-model-len=2048", "DIR=/nonexistent/dir", "args.max-model-len=4096", "args.enforce-eager=true"}
	p, err := BuildPlan(r, PlanOptions{Sets: sets})
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"org/m", "--enforce-eager", "--max-model-len", "4096", "--port", "9000",
		"--revision", "abc", "--served-model-name", "srv", "--tensor-parallel-size", "2"}
	if !reflect.DeepEqual(p.Args, wantArgs) {
		t.Errorf("args:\n got %q\nwant %q", p.Args, wantArgs)
	}
	if p.Env["PLE"] != "/nonexistent/dir" || p.Env["N"] != "4" {
		t.Errorf("env interpolation: %v", p.Env)
	}
	if !reflect.DeepEqual(p.Unset, []string{"GONE"}) {
		t.Errorf("unset = %v", p.Unset)
	}
	// A repo id model allows downloads; a local path forces offline.
	if p.Env["HF_HUB_OFFLINE"] != "0" || p.Env["TRANSFORMERS_OFFLINE"] != "0" {
		t.Errorf("offline for repo id: %v", p.Env)
	}
	if p.Port != 9000 || p.Mode != ModeHost || p.Name != "p" || p.Entrypoint != "api" {
		t.Errorf("plan header: %+v", p)
	}
	if p.Image != "" || p.TunableOpProfile != "" {
		t.Errorf("host mode must not carry container fields: %+v", p)
	}
	if !reflect.DeepEqual(p.PathInputs, []string{"/nonexistent/dir"}) || len(p.Warnings) == 0 {
		t.Errorf("path inputs %v warnings %v", p.PathInputs, p.Warnings)
	}

	local := t.TempDir()
	p, err = BuildPlan(r, PlanOptions{Mode: ModeContainer, Sets: []string{"DIR=" + local, "MODEL=" + local}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Env["HF_HUB_OFFLINE"] != "1" || p.Image != "img:1" || p.ShmSize != DefaultShmSize || !p.Pull {
		t.Errorf("container plan: %+v", p)
	}
	if !reflect.DeepEqual(p.PathInputs, []string{local}) {
		t.Errorf("model and input paths dedupe: %v", p.PathInputs)
	}

	// Host-only recipe (no container block) in container mode.
	r2, err := Parse("t/h.yaml", []byte(leaf+"args: {tensor-parallel-size: 2}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPlan(r2, PlanOptions{Mode: ModeContainer}); err == nil || !strings.Contains(err.Error(), "use --mode host") {
		t.Errorf("host-only recipe in container mode: %v", err)
	}
}

func TestTunedMoEFolderInHostMode(t *testing.T) {
	s := set(t, map[string]string{"m": leaf + "args: {tensor-parallel-size: 2}\ncontainer: {image: i}\nenv: {VLLM_TUNED_CONFIG_FOLDER: " + ImageTunedMoE + "}\n"})
	r, _ := s.Resolve("m")
	tests := []struct {
		mode, dir, want string
	}{
		{ModeContainer, "/data/tm", ImageTunedMoE},
		{ModeHost, "/data/tm", "/data/tm"},
		{ModeHost, "", ""},
	}
	for _, tt := range tests {
		p, err := BuildPlan(r, PlanOptions{Mode: tt.mode, TunedMoEDir: tt.dir})
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Env["VLLM_TUNED_CONFIG_FOLDER"]; got != tt.want {
			t.Errorf("%s dir=%q: got %q, want %q", tt.mode, tt.dir, got, tt.want)
		}
	}
}

func par1(t *testing.T) ([]gpu.GPU, *topo.Topology) {
	t.Helper()
	root := fixture.Root(t, "par1-cs25")
	gpus, err := gpu.List(root)
	if err != nil {
		t.Fatal(err)
	}
	tp, err := topo.Discover(root, gpus)
	if err != nil {
		t.Fatal(err)
	}
	return gpus, &tp
}

func TestGPUSelection(t *testing.T) {
	gpus, tp := par1(t) // 0-1 W7800 gfx1100, 2-9 V620 gfx1030 on two switches
	mk := func(arch string, n int, extra string) *Recipe {
		body := strings.NewReplacer("gfx1030]", arch+"]", "gpus: 2", "gpus: "+strconv.Itoa(n)).Replace(leaf) + extra
		r, err := Parse("t/sel.yaml", []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	tests := []struct {
		name    string
		r       *Recipe
		gpus    []gpu.GPU
		tp      *topo.Topology
		want    string
		wantErr string
	}{
		{"one switch, not the W7800s", mk("gfx1030", 4, ""), gpus, tp, "2,3,4,5", ""},
		{"gfx1100 pair", mk("gfx1100", 2, ""), gpus, tp, "0,1", ""},
		{"no topology: lowest indices", mk("gfx1030", 3, ""), gpus, nil, "2,3,4", ""},
		{"too many", mk("gfx1030", 9, ""), gpus, tp, "", "needs 9 GPUs of gfx1030; this host has 8 (8x gfx1030, 2x gfx1100)"},
		{"no GPU data", mk("gfx1030", 4, ""), nil, nil, "", ""},
		{"explicit", mk("gfx1030", 2, "env: {HIP_VISIBLE_DEVICES: \"6,7\"}\n"), gpus, tp, "6,7", ""},
		{"explicit wrong arch", mk("gfx1030", 2, "env: {HIP_VISIBLE_DEVICES: \"0,1\"}\n"), gpus, tp, "", "GPU 0 is gfx1100"},
		{"explicit missing", mk("gfx1030", 2, "env: {HIP_VISIBLE_DEVICES: \"2,42\"}\n"), gpus, tp, "", "GPU 42 does not exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := BuildPlan(tt.r, PlanOptions{GPUs: tt.gpus, Topo: tt.tp})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Env["HIP_VISIBLE_DEVICES"] != tt.want || p.HIPDevices != tt.want {
				t.Errorf("HIP_VISIBLE_DEVICES = %q (%s), want %q", p.Env["HIP_VISIBLE_DEVICES"], p.GPUSelectReason, tt.want)
			}
			if p.GPUSelectReason == "" {
				t.Error("no selection reason")
			}
		})
	}
}

func TestCheckFit(t *testing.T) {
	gpus, _ := par1(t)
	const g = uint64(1 << 30)
	mk := func(req string) *Recipe {
		r, err := Parse("t/f.yaml", []byte("schema: 1\nrequires: "+req+"\n"))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	tests := []struct {
		name   string
		req    string
		gpus   []gpu.GPU
		ram    uint64
		status string
		reason string
	}{
		{"fits", "{arch: [gfx1030], gpus: 4, vram_gib: 28}", gpus, 256 * g, FitYes, ""}, // V620 reports 29.98 GiB
		{"arch", "{arch: [gfx1200], gpus: 1}", gpus, 0, FitNo, "arch: needs gfx1200"},
		{"count", "{arch: [gfx1100], gpus: 4}", gpus, 0, FitNo, "gpus: needs 4 gfx1100 GPUs; host has 2"},
		{"vram", "{arch: [gfx1030], gpus: 4, vram_gib: 40}", gpus, 0, FitNo, "vram: needs 40 GiB"},
		{"ram", "{arch: [gfx1030], gpus: 4, ram_gib: 95}", gpus, 64 * g, FitNo, "ram: needs 95 GiB"},
		{"ram unknown skips", "{arch: [gfx1030], gpus: 4, ram_gib: 95}", gpus, 0, FitYes, ""},
		{"no gpu data", "{arch: [gfx1030], gpus: 4}", nil, 0, FitUnknown, "no GPU data"},
		{"no gpu data, ram rules out", "{arch: [gfx1030], gpus: 4, ram_gib: 95}", nil, 64 * g, FitNo, "ram:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := CheckFit(mk(tt.req), tt.gpus, tt.ram)
			if f.Status != tt.status {
				t.Errorf("status = %s (%v), want %s", f.Status, f.Reasons, tt.status)
			}
			if tt.reason != "" && !strings.Contains(strings.Join(f.Reasons, "; "), tt.reason) {
				t.Errorf("reasons %v, want %q", f.Reasons, tt.reason)
			}
		})
	}
}

func TestLoadOrderAndErrors(t *testing.T) {
	cfg := t.TempDir()
	env := t.TempDir()
	extra := t.TempDir()
	write := func(dir, name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := os.MkdirAll(filepath.Join(cfg, "recipes"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(cfg, "recipes"), "full.yaml", "schema: 1\nextends: [full]\nname: from-config\n")
	write(filepath.Join(cfg, "recipes"), "mine.yaml", strings.Replace(leaf, "Leaf", "config", 1))
	write(env, "mine.yaml", strings.Replace(leaf, "Leaf", "env", 1))
	write(env, "broken.yml", "schema: 1\nwhat: 1\n")
	extraFile := write(extra, "mine.yaml", strings.Replace(leaf, "Leaf", "extra", 1))
	t.Setenv("RDNACTL_CONFIG_DIR", cfg)
	t.Setenv("RDNACTL_RECIPES", env+string(os.PathListSeparator)+filepath.Join(env, "missing"))

	s := Load(extraFile)
	if got := s.Recipes["mine"].Name; got != "extra" {
		t.Errorf("mine = %q, want the extra file to win", got)
	}
	if !strings.HasPrefix(s.Recipes["27b-awq"].Source, "builtin:") {
		t.Errorf("built-in source = %s", s.Recipes["27b-awq"].Source)
	}
	var srcs []string
	for _, e := range s.Errors {
		srcs = append(srcs, filepath.Base(e.Source))
	}
	if !reflect.DeepEqual(srcs, []string{"broken.yml", "missing"}) {
		t.Errorf("errors = %v", s.Errors)
	}
	// A user file with a built-in id replaces it; extending its own id is a cycle.
	if _, err := s.Resolve("full"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("shadowed self-extend: %v", err)
	}
}
