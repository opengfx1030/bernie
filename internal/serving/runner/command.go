// Package runner starts, lists, stops and tails vLLM runs (spec §8): host
// processes from a venv, or containers with Docker or Podman. It also
// renders the exact commands and Compose files of a plan. Host mode never
// calls a container engine.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
	"github.com/opengfx1030/bernie/internal/support/config"
	"github.com/opengfx1030/bernie/internal/support/hostroot"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// Container paths of the plan's mounts.
const (
	containerHFCache  = "/root/.cache/huggingface"
	containerRowsDir  = tuning.ContainerRowsDir
	containerTunedMoE = "/bernie/tuned-moe"
)

// Python module entry points (spec §8.1).
const (
	moduleCLI = "vllm.entrypoints.cli.main"
	moduleAPI = "vllm.entrypoints.openai.api_server"
)

// StartOptions control Start and Command.
type StartOptions struct {
	Venv   string // host mode; "" = config resolution order (spec §8.1)
	Engine string // container mode: docker | podman | "" (config, then first usable)
	// Foreground keeps the process attached and waits for it to exit.
	Foreground bool
	// NoPull skips the image pull of a container plan.
	NoPull bool
	// Progress receives pull output and, in the foreground, the server log.
	Progress interface{ Write([]byte) (int, error) }
}

// RunDir is <state>/runs/<name>: run.json, serve.log, and the working
// directory of a host run.
func RunDir(name string) string { return filepath.Join(paths.State(), "runs", name) }

// CacheDir is <cache>/runs/<name>: per-run compile caches.
func CacheDir(name string) string { return filepath.Join(paths.Cache(), "runs", name) }

// cacheEnv are the per-run cache variables (host mode).
var cacheEnv = [][2]string{
	{"VLLM_CACHE_ROOT", "vllm"},
	{"TRITON_CACHE_DIR", "triton"},
	{"TORCHINDUCTOR_CACHE_DIR", "inductor"},
	{"TORCH_EXTENSIONS_DIR", "extensions"},
}

// serveArgs is the command after the program: cli = serve <model> <flags>,
// api = --model <model> <flags>.
func serveArgs(p *recipe.Plan) []string {
	if len(p.Args) == 0 {
		return nil
	}
	if p.Entrypoint == "api" {
		return append([]string{"--model", p.Args[0]}, p.Args[1:]...)
	}
	return append([]string{"serve"}, p.Args...)
}

// HostCommand returns the argv and the environment entries (KEY=VALUE, to
// add to the inherited environment) of a host-mode plan (spec §8.1). The
// variables in p.Unset must be removed from the inherited environment.
func HostCommand(p *recipe.Plan, venv string) (argv []string, env []string, err error) {
	if venv == "" {
		return nil, nil, fmt.Errorf("no venv: use --venv, $BERNIE_VENV, `bernie config set venv DIR` or activate one")
	}
	py := filepath.Join(venv, "bin", "python")
	if _, err := os.Stat(py); err != nil {
		return nil, nil, fmt.Errorf("venv %s has no bin/python", venv)
	}
	return hostArgv(p, py), hostEnv(p, venv), nil
}

func hostArgv(p *recipe.Plan, py string) []string {
	mod := moduleCLI
	args := serveArgs(p)
	if p.Entrypoint == "api" {
		mod = moduleAPI
	}
	return append([]string{py, "-m", mod}, args...)
}

func hostEnv(p *recipe.Plan, venv string) []string {
	env := map[string]string{}
	for k, v := range p.Env {
		env[k] = v
	}
	libs := rocm.VenvLibPaths(venv)
	if len(libs) > 0 {
		ld := strings.Join(libs, ":")
		if cur := env["LD_LIBRARY_PATH"]; cur != "" {
			ld += ":" + cur
		} else if cur := os.Getenv("LD_LIBRARY_PATH"); cur != "" {
			ld += ":" + cur
		}
		env["LD_LIBRARY_PATH"] = ld
	}
	for _, c := range cacheEnv {
		if _, ok := env[c[0]]; !ok {
			env[c[0]] = filepath.Join(CacheDir(p.Name), c[1])
		}
	}
	return sortedEnv(env)
}

func sortedEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// rowsMount is the host directory mounted at /bernie/tunableop: the
// parent of the profile's rows, so the in-container file is
// /bernie/tunableop/<profile>/tunableop_results.csv.
func rowsMount(p *recipe.Plan) string {
	if p.TunableOpRows == "" {
		return ""
	}
	return filepath.Dir(p.TunableOpRows)
}

// ContainerCommand returns the `<engine> run` argv of a container plan
// (spec §8.2). rowsDir is the host directory mounted at /bernie/tunableop
// ("" = none); hfCache is mounted at /root/.cache/huggingface.
func ContainerCommand(p *recipe.Plan, eng host.Engine, rowsDir, hfCache string) []string {
	name := eng.Name
	if name == "" {
		name = "docker"
	}
	a := []string{name, "run", "-d",
		"--name", ContainerName(p.Name), "--label", "bernie.run=" + p.Name,
		"--network", "host", "--ipc", "host", "--shm-size", p.ShmSize,
		"--device", "/dev/kfd", "--device", "/dev/dri",
	}
	a = append(a, groupArgs(p, eng)...)
	a = append(a, "--security-opt", "seccomp=unconfined", "--security-opt", "label=disable")
	for _, v := range volumes(p, rowsDir, hfCache) {
		a = append(a, "-v", v)
	}
	for _, kv := range sortedEnv(p.Env) {
		a = append(a, "-e", kv)
	}
	for _, k := range p.Unset {
		if name == "podman" {
			a = append(a, "--unsetenv", k)
		} else {
			// Docker cannot unset an image variable; empty disables the
			// flags bernie recipes remove (they compare with "TRUE"/"1").
			a = append(a, "-e", k+"=")
		}
	}
	if p.Entrypoint == "api" {
		a = append(a, "--entrypoint", "python3", p.Image, "-m", moduleAPI)
	} else {
		a = append(a, p.Image)
	}
	return append(a, serveArgs(p)...)
}

// ContainerName is the container of run name.
func ContainerName(name string) string { return "bernie-" + name }

// groupArgs returns the --group-add flags of a container run: the numeric
// group ids of the GPU devices of the host. A group name does not work:
// the image has no /etc/group to resolve it. Rootless Podman is the
// exception: its container runs in its own user namespace, where a host
// gid is not mapped, so it keeps the groups of the user instead.
func groupArgs(p *recipe.Plan, eng host.Engine) []string {
	if eng.Name == "podman" && eng.Rootless {
		return []string{"--group-add", "keep-groups"}
	}
	out := make([]string, 0, 2*len(p.GroupAdd))
	for _, g := range p.GroupAdd {
		out = append(out, "--group-add", strconv.Itoa(g.GID))
	}
	return out
}

// groupIDs are the numeric group ids of a Compose file.
func groupIDs(p *recipe.Plan) []string {
	out := make([]string, 0, len(p.GroupAdd))
	for _, g := range p.GroupAdd {
		out = append(out, strconv.Itoa(g.GID))
	}
	return out
}

func volumes(p *recipe.Plan, rowsDir, hfCache string) []string {
	var v []string
	if hfCache != "" {
		v = append(v, hfCache+":"+containerHFCache)
	}
	if rowsDir != "" {
		v = append(v, rowsDir+":"+containerRowsDir+":ro")
	}
	for _, m := range p.Mounts {
		s := m.Host + ":" + m.Container
		if m.ReadOnly {
			s += ":ro"
		}
		v = append(v, s)
	}
	for _, path := range p.PathInputs {
		v = append(v, path+":"+path)
	}
	return v
}

// Command returns the exact process of a plan: the venv python for host
// mode, `<engine> run` for container mode.
func Command(p *recipe.Plan, o StartOptions) (*recipe.Exec, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if p.Mode == recipe.ModeContainer {
		eng, err := resolveEngine(context.Background(), o.Engine, cfg)
		if err != nil {
			return nil, err
		}
		return &recipe.Exec{Argv: ContainerCommand(p, eng, rowsMount(p), cfg.ResolveHFHome())}, nil
	}
	argv, env, err := HostCommand(p, cfg.ResolveVenv(o.Venv))
	if err != nil {
		return nil, err
	}
	return &recipe.Exec{Argv: argv, Env: env, Unset: p.Unset, Dir: RunDir(p.Name)}, nil
}

// resolveEngine picks the engine: flag, then config, then the first usable.
func resolveEngine(ctx context.Context, name string, cfg config.Config) (host.Engine, error) {
	if name == "" {
		name = cfg.Engine
	}
	info, _ := host.Probe(ctx, hostroot.Root())
	eng, ok := host.Preferred(info.Engines, name)
	if !ok {
		if name != "" {
			return host.Engine{}, fmt.Errorf("container engine %s not found", name)
		}
		return host.Engine{}, fmt.Errorf("no container engine (docker or podman) found; use --mode host")
	}
	if !eng.Usable {
		return eng, fmt.Errorf("%s is installed but not usable by this user (`%s info` fails)", eng.Name, eng.Name)
	}
	return eng, nil
}

// composeFile is the Compose document. Field order is the output order.
type composeFile struct {
	Name     string                    `yaml:"name"`
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image         string            `yaml:"image"`
	ContainerName string            `yaml:"container_name"`
	Labels        map[string]string `yaml:"labels"`
	NetworkMode   string            `yaml:"network_mode"`
	IPC           string            `yaml:"ipc"`
	ShmSize       string            `yaml:"shm_size"`
	Devices       []string          `yaml:"devices"`
	GroupAdd      []string          `yaml:"group_add"`
	SecurityOpt   []string          `yaml:"security_opt"`
	Volumes       []string          `yaml:"volumes"`
	Environment   map[string]string `yaml:"environment"`
	Entrypoint    []string          `yaml:"entrypoint,omitempty"`
	Command       []string          `yaml:"command"`
	Restart       string            `yaml:"restart"`
}

// Compose renders a container plan as a Compose file for docker compose,
// podman compose and podman-compose (spec §8.2). rowsDir and hfCache are
// as for ContainerCommand. "$" is escaped as "$$" (Compose interpolation).
func Compose(p *recipe.Plan, rowsDir, hfCache string) ([]byte, error) {
	if p.Image == "" {
		return nil, fmt.Errorf("recipe %s has no container image; compose needs a container plan", p.Recipe)
	}
	esc := func(s string) string { return strings.ReplaceAll(s, "$", "$$") }
	escAll := func(l []string) []string {
		out := make([]string, len(l))
		for i, s := range l {
			out[i] = esc(s)
		}
		return out
	}
	env := map[string]string{}
	for k, v := range p.Env {
		env[k] = esc(v)
	}
	for _, k := range p.Unset {
		env[k] = ""
	}
	svc := composeService{
		Image:         p.Image,
		ContainerName: ContainerName(p.Name),
		Labels:        map[string]string{"bernie.run": p.Name},
		NetworkMode:   "host",
		IPC:           "host",
		ShmSize:       p.ShmSize,
		Devices:       []string{"/dev/kfd", "/dev/dri"},
		GroupAdd:      groupIDs(p),
		SecurityOpt:   []string{"seccomp=unconfined", "label=disable"},
		Volumes:       escAll(volumes(p, rowsDir, hfCache)),
		Environment:   env,
		Command:       escAll(serveArgs(p)),
		Restart:       "no",
	}
	if p.Entrypoint == "api" {
		svc.Entrypoint = []string{"python3", "-m", moduleAPI}
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Generated by bernie from recipe %s (run %s).\n", p.Recipe, p.Name)
	fmt.Fprintf(&b, "# Start: docker compose -f FILE up -d   (or podman compose / podman-compose)\n")
	fmt.Fprintf(&b, "# group_add holds the numeric ids of the GPU groups of this host:\n")
	for _, g := range p.GroupAdd {
		fmt.Fprintf(&b, "#   %d (%s, from %s)\n", g.GID, g.Role, g.Source)
	}
	fmt.Fprintf(&b, "# Rootless Podman: replace group_add with [keep-groups].\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(composeFile{Name: ContainerName(p.Name), Services: map[string]composeService{"vllm": svc}}); err != nil {
		return nil, err
	}
	return b.Bytes(), enc.Close()
}
