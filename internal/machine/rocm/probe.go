package rocm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/support/execx"
	"github.com/opengfx1030/bernie/internal/support/factcache"
	"github.com/opengfx1030/bernie/internal/support/pyenv"
)

// The facts rocm caches under the state directory, and how long they live.
const (
	// VenvProbeTTL is how long the torch and vLLM imports of a venv are
	// trusted. The probe starts the venv's Python, which takes seconds.
	VenvProbeTTL = 10 * time.Minute
	// InstallsTTL is how long the ROCm trees of a root are trusted.
	InstallsTTL = 10 * time.Minute
)

// venvProbeTimeout bounds the probe script of a venv. The import of a
// freshly built vLLM reads hundreds of megabytes of libraries and takes
// seconds. The tests lower it.
var venvProbeTimeout = 60 * time.Second

// Tools probed on PATH (then in the install bin dirs).
var Tools = []string{"rocminfo", "hipcc", "amd-smi", "rocm-smi", "rocm-bandwidth-test"}

// Probe finds ROCm installs, tools, the system rocBLAS, the HIP version and
// the rocminfo GPU agents. External tools run only when root is "/".
func Probe(ctx context.Context, root string) Report {
	if root == "" {
		root = "/"
	}
	r := Report{Installs: FindInstalls(root), Tools: map[string]string{}, OnPath: map[string]bool{}, Agents: []string{}}
	for _, t := range Tools {
		p := ""
		if root == "/" {
			p = execx.Which(t)
			r.OnPath[t] = p != ""
		}
		if p == "" {
			p = installTool(root, r.Installs, t)
		}
		r.Tools[t] = p
	}
	r.ROCBLAS = FindROCBLAS(root, "")
	if root != "/" {
		return r
	}
	var wg sync.WaitGroup
	if hipcc := r.Tools["hipcc"]; hipcc != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := execx.Run(ctx, 15*time.Second, hipcc, "--version"); err == nil {
				r.HIPVersion = ParseHIPVersion(res.Stdout + res.Stderr)
			}
		}()
	}
	if ri := r.Tools["rocminfo"]; ri != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := execx.Run(ctx, 20*time.Second, ri); err == nil {
				r.Agents = ParseRocminfoAgents(res.Stdout)
			}
		}()
	}
	wg.Wait()
	return r
}

// FindTool returns the path of a ROCm tool: on PATH (live host only), else
// in the newest install of the tree under root. It runs no tool.
func FindTool(root, name string) string {
	if root == "" {
		root = "/"
	}
	if root == "/" {
		if p := execx.Which(name); p != "" {
			return p
		}
	}
	return installTool(root, FindInstalls(root), name)
}

var versionDirRe = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)$`)

// treeVersion reads .info/version, then rocm_version.h, then the dir name.
func treeVersion(dir string) string {
	for _, f := range []string{".info/version", ".info/version-rocm"} {
		if v := execx.FirstLine(readFile(filepath.Join(dir, f))); v != "" {
			// "7.2.0-43" -> "7.2.0"
			v, _, _ = strings.Cut(v, "-")
			return v
		}
	}
	if v := headerVersion(filepath.Join(dir, "include/rocm-core/rocm_version.h"), "ROCM_VERSION"); v != "" {
		return v
	}
	if m := versionDirRe.FindStringSubmatch(filepath.Base(dir)); m != nil {
		return m[1]
	}
	return ""
}

// headerVersion reads <PREFIX>_MAJOR/_MINOR/_PATCH defines.
func headerVersion(path, prefix string) string {
	s := readFile(path)
	if s == "" {
		return ""
	}
	parts := make([]string, 0, 3)
	for _, k := range []string{"_MAJOR", "_MINOR", "_PATCH"} {
		re := regexp.MustCompile(`#define\s+` + prefix + k + `\s+(\d+)`)
		m := re.FindStringSubmatch(s)
		if m == nil {
			break
		}
		parts = append(parts, m[1])
	}
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts, ".")
}

// distroInstall detects ROCm from distro packages (libamdhip64 in /usr).
func distroInstall(root string) (string, bool) {
	for _, d := range []string{"usr/lib64", "usr/lib/x86_64-linux-gnu", "usr/lib"} {
		m, _ := filepath.Glob(filepath.Join(root, d, "libamdhip64.so*"))
		for _, lib := range m {
			rel := "/" + filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(lib, filepath.Clean(root)), "/"))
			if strings.HasPrefix(resolveIn(root, rel), "/usr/") {
				return headerVersion(filepath.Join(root, "usr/include/hip/hip_version.h"), "HIP_VERSION"), true
			}
		}
	}
	return "", false
}

var hipVersionRe = regexp.MustCompile(`HIP version:\s*([0-9][0-9.]*)`)

// ParseHIPVersion extracts "7.14.60850" from `hipcc --version` output.
func ParseHIPVersion(out string) string {
	if m := hipVersionRe.FindStringSubmatch(out); m != nil {
		return strings.TrimSuffix(m[1], ".")
	}
	return ""
}

var agentRe = regexp.MustCompile(`(?m)^\s*Name:\s+(gfx[0-9a-f]+)\s*$`)

// ParseRocminfoAgents lists the gfx names of the GPU agents, one per agent.
func ParseRocminfoAgents(out string) []string {
	agents := []string{}
	for _, m := range agentRe.FindAllStringSubmatch(out, -1) {
		agents = append(agents, m[1])
	}
	return agents
}

// ParseVersion parses "7.2.1" (any trailing text after digits ignored).
func ParseVersion(v string) []int {
	var out []int
	for _, p := range strings.Split(v, ".") {
		end := 0
		for end < len(p) && p[end] >= '0' && p[end] <= '9' {
			end++
		}
		if end == 0 {
			break
		}
		n, _ := strconv.Atoi(p[:end])
		out = append(out, n)
		if end < len(p) {
			break
		}
	}
	return out
}

// CompareVersions compares dotted versions numerically (-1, 0, 1).
func CompareVersions(a, b string) int {
	va, vb := ParseVersion(a), ParseVersion(b)
	for i := range max(len(va), len(vb)) {
		x, y := 0, 0
		if i < len(va) {
			x = va[i]
		}
		if i < len(vb) {
			y = vb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// resolveIn resolves symlinks in the host path p (absolute, "/"-rooted)
// inside root: absolute link targets are re-rooted, so fixture trees with
// /etc/alternatives links resolve like the real host.
func resolveIn(root, p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	cur := "/"
	for hops := 0; len(parts) > 0 && hops < 64; {
		comp := parts[0]
		parts = parts[1:]
		if comp == "" || comp == "." {
			continue
		}
		if comp == ".." {
			cur = filepath.Dir(cur)
			continue
		}
		next := filepath.Join(cur, comp)
		target, err := os.Readlink(filepath.Join(root, next))
		if err != nil {
			cur = next
			continue
		}
		hops++
		if filepath.IsAbs(target) {
			cur = "/"
		}
		parts = append(strings.Split(strings.Trim(target, "/"), "/"), parts...)
	}
	return cur
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// ProbeVenv runs the probe script with the venv's Python and the wheel
// library path (bounded to 30 s). The result is cached under the state
// directory: importing torch and vLLM takes seconds, and the answer only
// changes when the venv changes.
func ProbeVenv(ctx context.Context, venv string) VenvReport {
	rep, _, _, err := factcache.Get(ctx, venvProbeKey(venv), venvFingerprint(venv), VenvProbeTTL,
		func(ctx context.Context) (VenvReport, error) {
			r := probeVenv(ctx, venv)
			if r.Err != "" {
				return r, errors.New(r.Err)
			}
			return r, nil
		})
	if err != nil && rep.Err == "" {
		rep.Err = err.Error()
	}
	// Only a probe that ran and reported is cached, and Probed is not
	// stored with the fact: a hit without an error was probed.
	if err == nil && rep.Err == "" {
		rep.Probed = true
	}
	return rep
}

// probeVenv runs the probe script of a venv. It never caches: a failed
// probe is not a fact of the host.
func probeVenv(ctx context.Context, venv string) VenvReport {
	rep := VenvReport{Venv: venv}
	py := filepath.Join(venv, "bin", "python")
	if !isFile(py) {
		rep.Err = "no python at " + py
		return rep
	}
	rep.Python = pyvenvVersion(venv)
	env := []string{"PYTHONNOUSERSITE=1"}
	if libs := VenvLibPaths(venv); len(libs) > 0 {
		ld := strings.Join(libs, ":")
		if cur := os.Getenv("LD_LIBRARY_PATH"); cur != "" {
			ld += ":" + cur
		}
		env = append(env, "LD_LIBRARY_PATH="+ld)
	}
	res, err := execx.RunEnv(ctx, venvProbeTimeout, env, py, "-I", "-c", VenvProbeScript)
	if err != nil {
		rep.Err = err.Error()
		return rep
	}
	if !parseVenvOutput(res.Stdout, &rep) {
		rep.Err = "probe failed: " + lastLine(res.Stderr+res.Stdout)
		return rep
	}
	rep.Probed = true
	return rep
}

// venvProbeKey is the cache key of the probe of a venv.
func venvProbeKey(venv string) string { return "venv/probe/" + venv }

// venvFingerprint names the state of a venv: the times of the files that
// change when the venv is rebuilt, and the GPU restriction of the
// environment (the probe counts the GPUs that torch sees).
func venvFingerprint(venv string) string {
	paths := append([]string{filepath.Join(venv, "pyvenv.cfg")}, pyenv.SitePackages(venv)...)
	var out []string
	for _, p := range paths {
		out = append(out, stamp(p))
	}
	return strings.Join(out, ",") + "|" + restriction()
}

// restriction names the GPU restriction of the environment: the first
// variable of gpu.RestrictionVars that is set.
func restriction() string {
	for _, v := range gpu.RestrictionVars {
		if s := strings.TrimSpace(os.Getenv(v)); s != "" {
			return v + "=" + s
		}
	}
	return ""
}

// stamp returns the modification time of a path, or "-" when it is gone.
func stamp(p string) string {
	st, err := os.Stat(p)
	if err != nil {
		return "-"
	}
	return st.ModTime().UTC().Format(time.RFC3339Nano)
}

// parseVenvOutput fills rep from the marked JSON line.
func parseVenvOutput(out string, rep *VenvReport) bool {
	for _, line := range strings.Split(out, "\n") {
		js, ok := strings.CutPrefix(strings.TrimSpace(line), "BERNIE_JSON:")
		if !ok {
			continue
		}
		venv := rep.Venv
		if json.Unmarshal([]byte(js), rep) != nil {
			return false
		}
		rep.Venv = venv
		return true
	}
	return false
}

func pyvenvVersion(venv string) string {
	for _, line := range strings.Split(readFile(filepath.Join(venv, "pyvenv.cfg")), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if k = strings.TrimSpace(k); ok && (k == "version" || k == "version_info") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
