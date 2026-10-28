// Package doctor runs the host checks of spec §5: each check has a status
// (ok, warn, fail, skip), a detail, a fix for the host's package manager
// and a documentation link.
package doctor

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/opengfx1030/rdnactl/internal/machine/gpu"
	"github.com/opengfx1030/rdnactl/internal/machine/host"
	"github.com/opengfx1030/rdnactl/internal/machine/rocm"
	"github.com/opengfx1030/rdnactl/internal/serving/tuning"
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
var Categories = []string{"os", "gpu", "kernel", "rocm", "python", "container", "resources", "tuning"}

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

// Report is the doctor output.
type Report struct {
	Summary map[Status]int `json:"summary"`
	Checks  []Check        `json:"checks"`
}

// Options select the host root, the venv for python checks and the
// categories (all when empty). HFHome overrides the Hugging Face cache root
// ($HF_HOME, else ~/.cache/huggingface). ValidatedArchs are the gfx names or
// prefixes (as in a recipe's requires.arch) of the validated recipes; the caller reads them from the recipe set
// (doctor does not import recipe).
type Options struct {
	Root           string
	Venv           string
	Categories     []string
	HFHome         string
	ValidatedArchs []string
}

// Failed reports whether any check failed.
func (r Report) Failed() bool { return r.Summary[Fail] > 0 }

// inputs is everything the checks look at; gathered once, evaluated purely.
type inputs struct {
	root      string
	host      host.Info
	gpus      []gpu.GPU
	gpuErr    error
	rocm      rocm.Report
	venv      string
	venvRep   rocm.VenvReport
	site      string // venv site-packages ("" without venv)
	rocblas   string // librocblas.so.5 the venv (or system) loads
	libSHA    string
	profiles  []tuning.Profile
	profErr   error
	match     *tuning.Profile
	hfCache   string
	hfFree    uint64 // free bytes on the HF cache filesystem, 0 = unknown
	devices   devices
	amdgpu    bool     // amdgpu kernel module loaded
	validated []string // gfx targets with validated recipes
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
	return newReport(checks)
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
	in := inputs{root: root, venv: o.Venv, validated: o.ValidatedArchs}
	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	run(func() { in.host, _ = host.Probe(ctx, root) }) // best-effort, never fails
	run(func() { in.gpus, in.gpuErr = gpu.List(root) })
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
