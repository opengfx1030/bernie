package host

import (
	"context"
	"strings"
	"time"

	"github.com/opengfx1030/rdnactl/internal/support/execx"
)

// runner runs a tool; it is execx.Run in production and a fake in tests.
type runner interface {
	Which(name string) string
	Run(ctx context.Context, name string, args ...string) execx.Result
}

type execRunner struct{}

func (execRunner) Which(name string) string { return execx.Which(name) }

func (execRunner) Run(ctx context.Context, name string, args ...string) execx.Result {
	r, err := execx.Run(ctx, 10*time.Second, name, args...)
	if err != nil {
		r.ExitCode = -1
	}
	return r
}

var defaultRunner runner = execRunner{}

// ProbeEngines finds docker and podman, their versions, whether `info`
// works for this user, rootless mode and the compose command.
func ProbeEngines(ctx context.Context, r runner) []Engine {
	out := []Engine{}
	for _, name := range []string{"docker", "podman"} {
		path := r.Which(name)
		if path == "" {
			continue
		}
		e := Engine{Name: name, Path: path}
		if v := r.Run(ctx, name, "version", "--format", "{{.Client.Version}}"); v.OK() {
			e.Version = execx.FirstLine(v.Stdout)
		}
		switch name {
		case "docker":
			info := r.Run(ctx, name, "info", "--format", "{{json .SecurityOptions}}")
			e.Usable = info.OK()
			e.Rootless = strings.Contains(info.Stdout, "rootless")
		case "podman":
			info := r.Run(ctx, name, "info", "--format", "{{.Host.Security.Rootless}}")
			e.Usable = info.OK()
			e.Rootless = strings.TrimSpace(info.Stdout) == "true"
		}
		e.Compose = composeFor(ctx, r, name)
		out = append(out, e)
	}
	return out
}

// composeFor returns the compose command for an engine: the engine plugin
// ("docker compose", "podman compose") when it works, else the standalone
// binary ("docker-compose", "podman-compose"), else "".
func composeFor(ctx context.Context, r runner, engine string) string {
	if r.Run(ctx, engine, "compose", "version").OK() {
		return engine + " compose"
	}
	standalone := "docker-compose"
	if engine == "podman" {
		standalone = "podman-compose"
	}
	if r.Which(standalone) != "" {
		return standalone
	}
	return ""
}

// Preferred returns the engine to use: name when given and present, else
// the first usable engine (docker before podman), else the first found.
func Preferred(engines []Engine, name string) (Engine, bool) {
	for _, e := range engines {
		if name != "" && e.Name == name {
			return e, true
		}
	}
	if name != "" {
		return Engine{}, false
	}
	for _, e := range engines {
		if e.Usable {
			return e, true
		}
	}
	if len(engines) > 0 {
		return engines[0], true
	}
	return Engine{}, false
}
