// Package doctor runs the host checks of spec §5: each check has a status
// (ok, warn, fail, skip), a detail, a fix for the host's package manager
// and a documentation link.
package doctor

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
)

// Status is the result of one check.
type Status string

// Check statuses. fail blocks a launch; warn costs performance or
// stability; skip means the check cannot run here.
const (
	OK   Status = "ok"
	Warn Status = "warn"
	Fail Status = "fail"
	Skip Status = "skip"
)

// Categories in report order.
var Categories = []string{"os", "gpu", "kernel", "topo.p2p", "rocm", "python", "container", "resources", "tuning"}

// Check is one doctor test.
type Check struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Status   Status `json:"status"`
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
	Fix      string `json:"fix,omitempty"`
	Doc      string `json:"doc,omitempty"`
}

// VenvInfo says which host-mode venv doctor checked and where it came from.
type VenvInfo struct {
	Path       string   `json:"path,omitempty"`
	Source     string   `json:"source"`
	Candidates []string `json:"candidates,omitempty"`
}

// Report is the doctor output.
type Report struct {
	Summary map[Status]int `json:"summary"`
	Checks  []Check        `json:"checks"`
	Venv    *VenvInfo      `json:"venv,omitempty"`
}

// Options select the host root, the venv for python checks and the
// categories (all when empty). HFHome overrides the Hugging Face cache root
// ($HF_HOME, else ~/.cache/huggingface). ValidatedArchs are the gfx names or
// prefixes (as in a recipe's requires.arch) of the validated recipes; the caller reads them from the recipe set
// (doctor does not import recipe).
type Options struct {
	Root           string
	Venv           string
	VenvSource     string
	VenvCandidates []string
	Categories     []string
	HFHome         string
	ValidatedArchs []string
	// RecipeEnv is the environment of the recipe bernie would run. It fills
	// the informational recipe item of the topo.p2p category.
	RecipeEnv topo.RecipeEnv
}

// Failed reports whether any check failed.
func (r Report) Failed() bool { return r.Summary[Fail] > 0 }

// inputs is everything the checks look at; gathered once, evaluated purely.
type inputs struct {
	root           string
	host           host.Info
	gpus           []gpu.GPU
	gpuErr         error
	ecc            []gpu.ECC // ECC state of every GPU, in HIP order
	rocm           rocm.Report
	venv           string
	venvSource     string
	venvCandidates []string
	venvRep        rocm.VenvReport
	site           string // venv site-packages ("" without venv)
	rocblas        string // librocblas.so.5 the venv (or system) loads
	libSHA         string
	profiles       []tuning.Profile
	profErr        error
	match          *tuning.Profile
	hfCache        string
	hfFree         uint64 // free bytes on the HF cache filesystem, 0 = unknown
	devices        devices
	amdgpu         bool           // amdgpu kernel module loaded
	validated      []string       // gfx targets with validated recipes
	groupGIDs      map[string]int // gid of video and render; a group missing here was not found
	recipe         topo.RecipeEnv // environment of the recipe bernie would run
}

// devices is the state of the GPU device nodes.
type devices struct {
	kfdExists, kfdRW bool
	render           []string
	renderRW         bool
}

// Run gathers host data and evaluates the selected checks.
func Run(ctx context.Context, o Options) Report {
	want := map[string]bool{}
	for _, c := range o.Categories {
		want[c] = true
	}
	in := gather(ctx, o, want)
	var checks []Check
	for _, c := range evaluate(in) {
		if len(want) == 0 || want[c.Category] {
			checks = append(checks, c)
		}
	}
	r := newReport(checks)
	if o.VenvSource != "" {
		r.Venv = &VenvInfo{Path: o.Venv, Source: o.VenvSource, Candidates: o.VenvCandidates}
	}
	return r
}

func newReport(checks []Check) Report {
	r := Report{Summary: map[Status]int{OK: 0, Warn: 0, Fail: 0, Skip: 0}, Checks: checks}
	if r.Checks == nil {
		r.Checks = []Check{}
	}
	for _, c := range checks {
		r.Summary[c.Status]++
	}
	return r
}

func gather(ctx context.Context, o Options, want map[string]bool) inputs {
	root := o.Root
	if root == "" {
		root = "/"
	}
	need := func(cats ...string) bool {
		if len(want) == 0 {
			return true
		}
		for _, c := range cats {
			if want[c] {
				return true
			}
		}
		return false
	}
	in := inputs{root: root, venv: o.Venv, venvSource: o.VenvSource, venvCandidates: o.VenvCandidates,
		validated: o.ValidatedArchs, recipe: o.RecipeEnv}
	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	run(func() { in.host, _ = host.Probe(ctx, root) }) // best-effort, never fails
	run(func() {
		// Only the GPUs the environment leaves visible are checked
		// (spec §7.7).
		vis, err := gpu.Visible(root)
		if gpu.Restricted(err) {
			in.gpuErr = err // the restriction names a GPU this host does not have
			return
		}
		in.gpus, in.gpuErr = vis.GPUs, err
	})
	if need("rocm", "python", "tuning") {
		run(func() { in.rocm = rocm.Probe(ctx, root) })
	}
	if o.Venv != "" && need("python") {
		run(func() { in.venvRep = rocm.ProbeVenv(ctx, o.Venv) })
	}
	if need("rocm", "tuning") {
		run(func() {
			in.rocblas = rocm.FindROCBLAS(root, o.Venv)
			in.profiles, in.profErr = tuning.Profiles()
			if in.rocblas != "" {
				if sha, err := tuning.HashLib(in.rocblas); err == nil {
					in.libSHA = sha
					in.match, _ = tuning.Select(sha, "", nil)
				}
			}
		})
	}
	if sp := rocm.SitePackages(o.Venv); len(sp) > 0 {
		in.site = sp[0]
	}
	in.devices = probeDevices(root)
	in.amdgpu = amdgpuLoaded(root)
	in.hfCache = hfCache(o.HFHome)
	in.hfFree = freeBytes(in.hfCache)
	wg.Wait()
	if need("gpu") {
		// The ECC counters come from sysfs; amd-smi is the fallback for the
		// cards whose kernel exposes none.
		in.ecc = gpu.ECCAll(root, in.gpus)
		in.ecc = gpu.ECCTool(ctx, in.gpus, in.ecc, rocm.FindTool(root, "amd-smi"))
	}
	if need("gpu") && in.host.User.Name != "" {
		in.groupGIDs = resolveGroups(ctx, root, deviceGroups)
	}
	return in
}

func probeDevices(root string) devices {
	var d devices
	kfd := filepath.Join(root, "dev/kfd")
	_, err := os.Stat(kfd)
	d.kfdExists = err == nil
	d.kfdRW = d.kfdExists && canRW(kfd)
	d.render, _ = filepath.Glob(filepath.Join(root, "dev/dri/renderD*"))
	d.renderRW = len(d.render) > 0
	for _, r := range d.render {
		if !canRW(r) {
			d.renderRW = false
		}
	}
	return d
}

func canRW(p string) bool {
	f, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// hfCache returns the Hugging Face cache root.
func hfCache(override string) string {
	if override != "" {
		return override
	}
	if v := os.Getenv("HF_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "huggingface")
}

// freeBytes is the free space of the filesystem holding p (or its nearest
// existing parent).
func freeBytes(p string) uint64 {
	for p != "" && p != "/" {
		if _, err := os.Stat(p); err == nil {
			break
		}
		p = filepath.Dir(p)
	}
	return statfsFree(p)
}
