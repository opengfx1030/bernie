package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
)

func plan(entry string) *recipe.Plan {
	return &recipe.Plan{
		Recipe: "r", Name: "run1", Mode: recipe.ModeContainer, Entrypoint: entry,
		Image: "img:1", ShmSize: "16g",
		Env:        map[string]string{"B": "2", "A": "1", "PRICE": "$5"},
		Unset:      []string{"FLASH_ATTENTION_TRITON_AMD_ENABLE"},
		Args:       []string{"org/m", "--port", "18096", "--x", `{"a":"$b"}`},
		PathInputs: []string{"/data/ple"},
		Mounts:     []recipe.Mount{{Host: "/host/tm", Container: "/bernie/tuned-moe", ReadOnly: true}},
	}
}

func TestHostArgv(t *testing.T) {
	tests := []struct {
		entry string
		want  []string
	}{
		{"cli", []string{"/v/bin/python", "-m", "vllm.entrypoints.cli.main", "serve", "org/m", "--port", "18096", "--x", `{"a":"$b"}`}},
		{"api", []string{"/v/bin/python", "-m", "vllm.entrypoints.openai.api_server", "--model", "org/m", "--port", "18096", "--x", `{"a":"$b"}`}},
	}
	for _, tt := range tests {
		if got := hostArgv(plan(tt.entry), "/v/bin/python"); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s:\n got %q\nwant %q", tt.entry, got, tt.want)
		}
	}
}

func TestHostEnv(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("BERNIE_CACHE_DIR", cache)
	venv := t.TempDir()
	sp := filepath.Join(venv, "lib", "python3.12", "site-packages")
	for _, d := range []string{"_rocm_sdk_libraries/lib", "_rocm_sdk_core/lib", "torch/lib"} {
		if err := os.MkdirAll(filepath.Join(sp, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wheel := filepath.Join(sp, "_rocm_sdk_libraries/lib") + ":" + filepath.Join(sp, "_rocm_sdk_core/lib") + ":" + filepath.Join(sp, "torch/lib")

	tests := []struct {
		name     string
		planLD   string
		inherit  string
		wantLD   string
		planVLLM string
	}{
		{"wheel libs first, inherited after", "", "/opt/rocm/lib", wheel + ":/opt/rocm/lib", ""},
		{"recipe LD_LIBRARY_PATH after wheel libs", "/custom", "/opt/rocm/lib", wheel + ":/custom", ""},
		{"recipe cache root kept", "", "", wheel, "/my/cache"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LD_LIBRARY_PATH", tt.inherit)
			p := plan("cli")
			if tt.planLD != "" {
				p.Env["LD_LIBRARY_PATH"] = tt.planLD
			}
			if tt.planVLLM != "" {
				p.Env["VLLM_CACHE_ROOT"] = tt.planVLLM
			}
			env := map[string]string{}
			keys := []string{}
			for _, kv := range hostEnv(p, venv) {
				k, v, _ := strings.Cut(kv, "=")
				env[k] = v
				keys = append(keys, k)
			}
			if env["LD_LIBRARY_PATH"] != tt.wantLD {
				t.Errorf("LD_LIBRARY_PATH = %q\nwant %q", env["LD_LIBRARY_PATH"], tt.wantLD)
			}
			wantCache := filepath.Join(cache, "runs", "run1", "vllm")
			if tt.planVLLM != "" {
				wantCache = tt.planVLLM
			}
			if env["VLLM_CACHE_ROOT"] != wantCache || env["TRITON_CACHE_DIR"] != filepath.Join(cache, "runs", "run1", "triton") {
				t.Errorf("caches: %v", env)
			}
			if env["A"] != "1" || !sortedStrings(keys) {
				t.Errorf("env not complete or not sorted: %v", keys)
			}
		})
	}
}

func sortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}

func TestContainerCommand(t *testing.T) {
	common := []string{"run", "-d", "--name", "bernie-run1", "--label", "bernie.run=run1",
		"--network", "host", "--ipc", "host", "--shm-size", "16g", "--device", "/dev/kfd", "--device", "/dev/dri"}
	sec := []string{"--security-opt", "seccomp=unconfined", "--security-opt", "label=disable",
		"-v", "/hf:/root/.cache/huggingface", "-v", "/rows:/bernie/tunableop:ro",
		"-v", "/host/tm:/bernie/tuned-moe:ro", "-v", "/data/ple:/data/ple",
		"-e", "A=1", "-e", "B=2", "-e", "PRICE=$5"}
	cat := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	tests := []struct {
		name  string
		eng   host.Engine
		entry string
		want  []string
	}{
		{"docker cli", host.Engine{Name: "docker"}, "cli", cat([]string{"docker"}, common,
			[]string{"--group-add", "video", "--group-add", "render"}, sec,
			[]string{"-e", "FLASH_ATTENTION_TRITON_AMD_ENABLE=", "img:1", "serve", "org/m", "--port", "18096", "--x", `{"a":"$b"}`})},
		{"rootless podman api", host.Engine{Name: "podman", Rootless: true}, "api", cat([]string{"podman"}, common,
			[]string{"--group-add", "keep-groups"}, sec,
			[]string{"--unsetenv", "FLASH_ATTENTION_TRITON_AMD_ENABLE", "--entrypoint", "python3", "img:1",
				"-m", "vllm.entrypoints.openai.api_server", "--model", "org/m", "--port", "18096", "--x", `{"a":"$b"}`})},
		{"rootful podman keeps numeric groups", host.Engine{Name: "podman"}, "cli", cat([]string{"podman"}, common,
			[]string{"--group-add", "video", "--group-add", "render"}, sec,
			[]string{"--unsetenv", "FLASH_ATTENTION_TRITON_AMD_ENABLE", "img:1", "serve", "org/m", "--port", "18096", "--x", `{"a":"$b"}`})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainerCommand(plan(tt.entry), tt.eng, "/rows", "/hf")
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestCompose(t *testing.T) {
	for _, entry := range []string{"cli", "api"} {
		t.Run(entry, func(t *testing.T) {
			b, err := Compose(plan(entry), "/rows", "/hf")
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Name     string `yaml:"name"`
				Services map[string]struct {
					Image       string            `yaml:"image"`
					NetworkMode string            `yaml:"network_mode"`
					IPC         string            `yaml:"ipc"`
					ShmSize     string            `yaml:"shm_size"`
					Devices     []string          `yaml:"devices"`
					GroupAdd    []string          `yaml:"group_add"`
					SecurityOpt []string          `yaml:"security_opt"`
					Volumes     []string          `yaml:"volumes"`
					Environment map[string]string `yaml:"environment"`
					Entrypoint  []string          `yaml:"entrypoint"`
					Command     []string          `yaml:"command"`
				} `yaml:"services"`
			}
			if err := yaml.Unmarshal(b, &doc); err != nil {
				t.Fatalf("%v\n%s", err, b)
			}
			s, ok := doc.Services["vllm"]
			if !ok || doc.Name != "bernie-run1" {
				t.Fatalf("doc: %+v", doc)
			}
			if s.Image != "img:1" || s.NetworkMode != "host" || s.IPC != "host" || s.ShmSize != "16g" {
				t.Errorf("service: %+v", s)
			}
			if !reflect.DeepEqual(s.Devices, []string{"/dev/kfd", "/dev/dri"}) ||
				!reflect.DeepEqual(s.GroupAdd, []string{"video", "render"}) ||
				!reflect.DeepEqual(s.SecurityOpt, []string{"seccomp=unconfined", "label=disable"}) {
				t.Errorf("devices/groups/security: %+v", s)
			}
			wantVol := []string{"/hf:/root/.cache/huggingface", "/rows:/bernie/tunableop:ro", "/host/tm:/bernie/tuned-moe:ro", "/data/ple:/data/ple"}
			if !reflect.DeepEqual(s.Volumes, wantVol) {
				t.Errorf("volumes = %q", s.Volumes)
			}
			// "$" is escaped for Compose interpolation; unset becomes empty.
			if s.Environment["PRICE"] != "$$5" || s.Environment["FLASH_ATTENTION_TRITON_AMD_ENABLE"] != "" {
				t.Errorf("environment = %v", s.Environment)
			}
			wantCmd := []string{"serve", "org/m", "--port", "18096", "--x", `{"a":"$$b"}`}
			var wantEP []string
			if entry == "api" {
				wantCmd = []string{"--model", "org/m", "--port", "18096", "--x", `{"a":"$$b"}`}
				wantEP = []string{"python3", "-m", "vllm.entrypoints.openai.api_server"}
			}
			if !reflect.DeepEqual(s.Command, wantCmd) || !reflect.DeepEqual(s.Entrypoint, wantEP) {
				t.Errorf("entrypoint %q command %q", s.Entrypoint, s.Command)
			}
		})
	}
	p := plan("cli")
	p.Image = ""
	if _, err := Compose(p, "", ""); err == nil {
		t.Error("compose of a host-only plan must fail")
	}
}

func TestMergeEnv(t *testing.T) {
	got := mergeEnv([]string{"PATH=/bin", "A=old", "GONE=1", "KEEP=x"}, []string{"A=new", "B=2"}, []string{"GONE"})
	want := []string{"PATH=/bin", "KEEP=x", "A=new", "B=2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTailLines(t *testing.T) {
	in := "1\n2\n3\n4\n5\n"
	tests := []struct {
		n    int
		want string
	}{
		{2, "4\n5\n"},
		{10, in},
		{0, in},
	}
	for _, tt := range tests {
		var b bytes.Buffer
		if err := tailLines(strings.NewReader(in), tt.n, &b); err != nil {
			t.Fatal(err)
		}
		if b.String() != tt.want {
			t.Errorf("tail %d = %q, want %q", tt.n, b.String(), tt.want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"plain/path-1.0":  "plain/path-1.0",
		"":                "''",
		`{"a":1}`:         `'{"a":1}'`,
		"it's":            `'it'\''s'`,
		"a b":             "'a b'",
		"KEY=v,w:x@y%z+1": "KEY=v,w:x@y%z+1",
		"$HOME":           "'$HOME'",
	}
	for in, want := range tests {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRecordRoundTripAndURL(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	r := &Record{Name: "run1", Recipe: "r", Mode: recipe.ModeHost, PID: 42, Host: "", Port: 18096}
	if err := writeRecord(r); err != nil {
		t.Fatal(err)
	}
	back, err := readRecord("run1")
	if err != nil {
		t.Fatal(err)
	}
	if back.PID != 42 || back.Port != 18096 || back.Recipe != "r" {
		t.Errorf("record = %+v", back)
	}
	if got := back.URL(); got != "http://127.0.0.1:18096" {
		t.Errorf("URL = %q", got)
	}
	back.Host = "0.0.0.0"
	if got := back.URL(); got != "http://127.0.0.1:18096" {
		t.Errorf("wildcard host URL = %q", got)
	}
	if _, err := readRecord("nope"); err == nil {
		t.Error("missing record read")
	}
}
