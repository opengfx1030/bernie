package recipe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/gpu"
	"github.com/opengfx1030/rdnactl/internal/topo"
)

// Modes.
const (
	ModeHost      = "host"
	ModeContainer = "container"
)

// DefaultPort is vLLM's port when the recipe sets none.
const DefaultPort = 8000

// DefaultShmSize is the container shared memory size when the recipe sets none.
const DefaultShmSize = "16g"

// ImageTunedMoE is the fork's tuned MoE config folder inside the vllm-rdna
// image. A host plan replaces it with PlanOptions.TunedMoEDir: a container
// path never reaches a host process.
const ImageTunedMoE = "/src/vllm/tuned-moe"

const gib = 1 << 30

// Exec is the exact process a plan starts: argv, the environment added to
// (Env) and removed from (Unset) the inherited one, and the working
// directory. The runner fills it.
type Exec struct {
	Argv  []string `json:"argv"`
	Env   []string `json:"env,omitempty"`
	Unset []string `json:"unset,omitempty"`
	Dir   string   `json:"dir,omitempty"`
}

// Mount is one extra bind mount of a container plan.
type Mount struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"read_only"`
}

// Plan is one resolved launch (spec terms: recipe + overrides + host).
type Plan struct {
	Recipe     string `json:"recipe"`
	Name       string `json:"name"`
	Mode       string `json:"mode"`
	Entrypoint string `json:"entrypoint"` // cli | api
	Model      string `json:"model"`
	ServedName string `json:"served_name,omitempty"`
	Host       string `json:"host"` // --host; "" = vLLM default
	Port       int    `json:"port"`
	// Env is the vLLM environment. Unset lists variables to remove from the
	// inherited environment (env: KEY: null).
	Env   map[string]string `json:"env"`
	Unset []string          `json:"unset,omitempty"`
	// Args follow "serve": the model, then flags sorted by name.
	Args            []string `json:"args"`
	GPUs            []int    `json:"gpus"`
	HIPDevices      string   `json:"hip_devices"`
	GPUSelectReason string   `json:"gpu_select_reason"`
	Image           string   `json:"image,omitempty"`
	ShmSize         string   `json:"shm_size,omitempty"`
	Pull            bool     `json:"pull"`
	// TunableOpProfile is the selected profile; TunableOpRows its host
	// directory (empty: no rows, or the per-user fallback).
	TunableOpProfile string   `json:"tunableop_profile,omitempty"`
	TunableOpRows    string   `json:"tunableop_rows,omitempty"`
	PathInputs       []string `json:"path_inputs"`
	Mounts           []Mount  `json:"mounts,omitempty"`
	Warnings         []string `json:"warnings"`
	Exec             *Exec    `json:"exec,omitempty"`
}

// PlanOptions are the inputs of BuildPlan. Sets are applied in order (config
// defaults first, then --set). GPUs nil means no GPU data (no selection).
// TunedMoEDir is the host copy of the tuned MoE configs (host mode).
type PlanOptions struct {
	Mode, Name  string
	Sets        []string
	GPUs        []gpu.GPU
	Topo        *topo.Topology
	TunedMoEDir string
}

// Override is one parsed --set.
type Override struct {
	Key   string   // as given
	Input bool     // KEY has no dot: sets the input KEY
	Path  []string // section path when !Input
	Raw   string   // VALUE as given
	Value any      // VALUE parsed as YAML (fields only)
}

var settable = map[string]bool{
	"name": true, "description": true, "status": true, "priority": true, "entrypoint": true,
	"tags": true, "model": true, "requires": true, "inputs": true, "container": true,
	"env": true, "args": true,
}

// ParseSet parses KEY=VALUE (spec §7.5). Input values stay plain strings;
// field values are parsed as YAML (empty VALUE is the empty string).
func ParseSet(s string) (Override, error) {
	k, v, ok := strings.Cut(s, "=")
	if !ok || strings.TrimSpace(k) == "" {
		return Override{}, fmt.Errorf("--set %q: want KEY=VALUE", s)
	}
	o := Override{Key: k, Raw: v}
	if !strings.Contains(k, ".") {
		if !varPattern.MatchString(k) {
			return o, fmt.Errorf("--set %q: %q is not an input name", s, k)
		}
		o.Input = true
		return o, nil
	}
	o.Path = strings.Split(k, ".")
	for _, p := range o.Path {
		if p == "" {
			return o, fmt.Errorf("--set %q: empty path element", s)
		}
	}
	if !settable[o.Path[0]] {
		return o, fmt.Errorf("--set %q: unknown section %q (sections: args, env, model, requires, inputs, container, ...)", s, o.Path[0])
	}
	val, err := parseValue(v)
	if err != nil {
		return o, fmt.Errorf("--set %q: value: %w", s, err)
	}
	o.Value = val
	return o, nil
}

// Apply returns a copy of r with the field overrides of sets applied, plus
// the input values they set. Unknown inputs are an error.
func Apply(r *Recipe, sets []string) (*Recipe, map[string]string, error) {
	doc := docOf(r)
	inputs := map[string]string{}
	var inputKeys []string
	for _, s := range sets {
		o, err := ParseSet(s)
		if err != nil {
			return nil, nil, err
		}
		if o.Input {
			inputs[o.Key] = o.Raw
			inputKeys = append(inputKeys, o.Key)
			continue
		}
		if err := setPath(doc, o.Path, o.Value); err != nil {
			return nil, nil, fmt.Errorf("--set %s: %w", o.Key, err)
		}
	}
	out, err := decode(doc)
	if err != nil {
		return nil, nil, fmt.Errorf("after --set: %w", err)
	}
	out.ID, out.Source, out.Fragment, out.Extends, out.doc = r.ID, r.Source, r.Fragment, r.Extends, r.doc
	for _, k := range inputKeys {
		if _, ok := out.Inputs[k]; !ok {
			return nil, nil, fmt.Errorf("--set %s: recipe %s has no input %s%s", k, r.ID, k, inputHint(out))
		}
	}
	return out, inputs, nil
}

func inputHint(r *Recipe) string {
	if len(r.Inputs) == 0 {
		return " (it has no inputs; use section.key=VALUE for fields, e.g. args.port=8001)"
	}
	return " (inputs: " + strings.Join(sortedKeys(r.Inputs), ", ") + ")"
}

// Modes returns the modes a recipe supports: host always, container when it
// names an image.
func Modes(r *Recipe) []string {
	if r.Container != nil && r.Container.Image != "" {
		return []string{ModeHost, ModeContainer}
	}
	return []string{ModeHost}
}

// BuildPlan turns a resolved recipe into a Plan.
func BuildPlan(r *Recipe, o PlanOptions) (*Plan, error) {
	if r.Fragment {
		return nil, fmt.Errorf("%s is a fragment; fragments are only used through extends", r.ID)
	}
	r, inputVals, err := Apply(r, o.Sets)
	if err != nil {
		return nil, err
	}
	if errs := Validate(r); len(errs) > 0 {
		return nil, fmt.Errorf("recipe %s is not valid:\n  %s", r.ID, strings.Join(errs, "\n  "))
	}

	mode := o.Mode
	if mode == "" {
		mode = ModeHost
	}
	switch mode {
	case ModeHost:
	case ModeContainer:
		if r.Container == nil {
			return nil, fmt.Errorf("recipe %s needs features not in any published image; use --mode host", r.ID)
		}
	default:
		return nil, fmt.Errorf("mode must be host or container, not %q", mode)
	}

	// Input values: --set, then default. Required inputs must have one.
	vals := map[string]string{}
	var missing []string
	for _, name := range sortedKeys(r.Inputs) {
		in := r.Inputs[name]
		v, ok := inputVals[name]
		if !ok {
			v = in.Default
		}
		if in.Required && v == "" {
			hint := ""
			if in.Description != "" {
				hint = " (" + in.Description + ")"
			}
			missing = append(missing, fmt.Sprintf("input %s is required%s: --set %s=...", name, hint, name))
		}
		vals[name] = v
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("recipe %s: %s", r.ID, strings.Join(missing, "; "))
	}
	interp := func(s string) string { return interpolate(s, vals) }

	p := &Plan{
		Recipe:     r.ID,
		Name:       o.Name,
		Mode:       mode,
		Entrypoint: r.Entrypoint,
		Model:      interp(r.Model.Source),
		ServedName: interp(r.Model.ServedName),
		Env:        map[string]string{},
		GPUs:       []int{},
		PathInputs: []string{},
		Warnings:   []string{},
	}
	if p.Name == "" {
		p.Name = r.ID
	}
	if !idPattern.MatchString(p.Name) || strings.HasPrefix(p.Name, "_") {
		return nil, fmt.Errorf("run name %q: use lowercase letters, digits, '.', '_' and '-'", p.Name)
	}
	if p.Entrypoint == "" {
		p.Entrypoint = "cli"
	}
	revision := interp(r.Model.Revision)

	for _, k := range sortedKeys(r.Env) {
		v := r.Env[k]
		if v == nil {
			p.Unset = append(p.Unset, k)
			continue
		}
		p.Env[k] = interp(scalarString(v))
	}
	local := filepath.IsAbs(p.Model)
	for _, k := range []string{"HF_HUB_OFFLINE", "TRANSFORMERS_OFFLINE"} {
		if _, set := r.Env[k]; !set {
			p.Env[k] = map[bool]string{true: "1", false: "0"}[local]
		}
	}

	// Arguments.
	args := map[string]any{}
	for k, v := range r.Args {
		args[k] = interpAny(v, interp)
	}
	if p.ServedName != "" {
		args["served-model-name"] = p.ServedName
	}
	if revision != "" {
		args["revision"] = revision
	}
	flags, err := RenderArgs(args)
	if err != nil {
		return nil, err
	}
	p.Args = append([]string{p.Model}, flags...)
	p.Host = scalarString(args["host"])
	p.Port = DefaultPort
	if v, ok := args["port"]; ok && v != nil {
		n, ok := toInt(v)
		if !ok || n <= 0 || n > 65535 {
			return nil, fmt.Errorf("args.port: want a TCP port, got %v", v)
		}
		p.Port = n
	}

	// Paths: path inputs and a local model are host paths (mounted in a
	// container at the same path).
	seen := map[string]bool{}
	addPath := func(path, what string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		if !filepath.IsAbs(path) {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s %q is not an absolute path", what, path))
			return
		}
		p.PathInputs = append(p.PathInputs, path)
		if _, err := os.Stat(path); err != nil {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s %s does not exist on this host", what, path))
		}
	}
	if local {
		addPath(p.Model, "model")
	}
	for _, name := range sortedKeys(r.Inputs) {
		if r.Inputs[name].Path {
			addPath(vals[name], "input "+name)
		}
	}
	sort.Strings(p.PathInputs)

	if r.Container != nil {
		p.Image = r.Container.Image
		p.ShmSize = r.Container.ShmSize
		if p.ShmSize == "" {
			p.ShmSize = DefaultShmSize
		}
		p.Pull = r.Container.Pull == nil || *r.Container.Pull
		p.TunableOpProfile = r.Container.TunableOpProfile
	}
	if mode == ModeHost {
		// Host mode selects the profile from the loaded rocBLAS (runner).
		p.Image, p.ShmSize, p.Pull, p.TunableOpProfile = "", "", false, ""
		if p.Env["VLLM_TUNED_CONFIG_FOLDER"] == ImageTunedMoE {
			if o.TunedMoEDir != "" {
				p.Env["VLLM_TUNED_CONFIG_FOLDER"] = o.TunedMoEDir
			} else {
				delete(p.Env, "VLLM_TUNED_CONFIG_FOLDER")
				p.Warnings = append(p.Warnings, "tuned MoE configs not materialized: VLLM_TUNED_CONFIG_FOLDER dropped")
			}
		}
	}

	if err := selectGPUs(r, p, o); err != nil {
		return nil, err
	}
	return p, nil
}

// selectGPUs applies spec §7.7 and sets HIP_VISIBLE_DEVICES.
func selectGPUs(r *Recipe, p *Plan, o PlanOptions) error {
	need := r.Requires.GPUs
	if v, ok := p.Env["HIP_VISIBLE_DEVICES"]; ok {
		p.HIPDevices = v
		p.GPUSelectReason = "HIP_VISIBLE_DEVICES set by the recipe or --set"
		idx, err := parseIndices(v)
		if err != nil {
			return fmt.Errorf("env.HIP_VISIBLE_DEVICES: %w", err)
		}
		p.GPUs = idx
		if o.GPUs != nil {
			byIdx := map[int]gpu.GPU{}
			for _, g := range o.GPUs {
				byIdx[g.HIPIndex] = g
			}
			for _, i := range idx {
				g, ok := byIdx[i]
				if !ok {
					return fmt.Errorf("HIP_VISIBLE_DEVICES: GPU %d does not exist on this host", i)
				}
				if !archMatch(r.Requires.Arch, g.GFX) {
					return fmt.Errorf("HIP_VISIBLE_DEVICES: GPU %d is %s; recipe %s needs %s", i, g.GFX, r.ID, strings.Join(r.Requires.Arch, ", "))
				}
			}
		}
		if len(idx) != need {
			p.Warnings = append(p.Warnings, fmt.Sprintf("HIP_VISIBLE_DEVICES lists %d GPUs; the recipe needs %d", len(idx), need))
		}
		return nil
	}
	if o.GPUs == nil {
		p.GPUSelectReason = "no GPU data: HIP_VISIBLE_DEVICES not set"
		p.Warnings = append(p.Warnings, "no GPU data: vLLM uses every visible GPU; set env.HIP_VISIBLE_DEVICES")
		return nil
	}
	var cand []int
	for _, g := range o.GPUs {
		if archMatch(r.Requires.Arch, g.GFX) {
			cand = append(cand, g.HIPIndex)
		}
	}
	if len(cand) < need {
		return fmt.Errorf("recipe %s needs %d GPUs of %s; this host has %d (%s)", r.ID, need,
			strings.Join(r.Requires.Arch, "/"), len(cand), archSummary(o.GPUs))
	}
	var sel []int
	var reason string
	if o.Topo != nil {
		sel, reason = topo.BestGroup(*o.Topo, cand, need)
	} else {
		sort.Ints(cand)
		sel = cand[:need]
		reason = fmt.Sprintf("GPUs %s: no topology data; lowest HIP indices", joinInts(sel))
	}
	if len(sel) != need {
		return fmt.Errorf("GPU selection failed: %s", reason)
	}
	p.GPUs = sel
	p.HIPDevices = joinInts(sel)
	p.GPUSelectReason = reason
	p.Env["HIP_VISIBLE_DEVICES"] = p.HIPDevices
	return nil
}

func parseIndices(s string) ([]int, error) {
	out := []int{}
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("want comma-separated HIP indices, got %q", s)
		}
		out = append(out, n)
	}
	return out, nil
}

func archMatch(arch []string, gfx string) bool {
	for _, a := range arch {
		if gfx != "" && strings.HasPrefix(gfx, a) {
			return true
		}
	}
	return false
}

func archSummary(gpus []gpu.GPU) string {
	if len(gpus) == 0 {
		return "no GPUs"
	}
	count := map[string]int{}
	for _, g := range gpus {
		count[g.GFX]++
	}
	parts := make([]string, 0, len(count))
	for _, k := range sortedKeys(count) {
		parts = append(parts, fmt.Sprintf("%dx %s", count[k], k))
	}
	return strings.Join(parts, ", ")
}

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ",")
}

// interpolate replaces ${NAME} with vals[NAME]; $${ is a literal ${.
func interpolate(s string, vals map[string]string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	const esc = "\x00"
	s = strings.ReplaceAll(s, "$${", esc)
	s = refPattern.ReplaceAllStringFunc(s, func(m string) string {
		return vals[m[2:len(m)-1]]
	})
	return strings.ReplaceAll(s, esc, "${")
}

func interpAny(v any, f func(string) string) any {
	switch t := v.(type) {
	case string:
		return f(t)
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = interpAny(x, f)
		}
		return m
	case []any:
		l := make([]any, len(t))
		for i, x := range t {
			l[i] = interpAny(x, f)
		}
		return l
	}
	return v
}

// scalarString renders an env or arg scalar.
func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// RenderArgs renders vllm flags (spec §7.6): sorted by key; true gives
// --key; false and null give nothing; maps and lists give compact JSON;
// other values give --key VALUE.
func RenderArgs(args map[string]any) ([]string, error) {
	var out []string
	for _, k := range sortedKeys(args) {
		switch v := args[k].(type) {
		case nil:
		case bool:
			if v {
				out = append(out, "--"+k)
			}
		case map[string]any, []any:
			j, err := compactJSON(v)
			if err != nil {
				return nil, fmt.Errorf("args.%s: %w", k, err)
			}
			out = append(out, "--"+k, j)
		default:
			out = append(out, "--"+k, scalarString(v))
		}
	}
	return out, nil
}

func compactJSON(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
