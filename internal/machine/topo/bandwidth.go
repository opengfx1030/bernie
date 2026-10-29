package topo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/support/execx"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// Bandwidth sources.
const (
	BandwidthRBT   = "rocm-bandwidth-test"
	BandwidthTorch = "torch"
)

// Measurement is a saved bandwidth measurement: the GB/s of every pair, the
// time it was taken and the tool that took it. The page reads it so the
// Topology tab can show a measured matrix without measuring again
// (ia.md §5 R32).
type Measurement struct {
	MeasuredAt time.Time          `json:"measured_at"`
	Source     string             `json:"source"`
	GBps       map[string]float64 `json:"gbps"` // "a-b" -> GB/s
}

// measurementPath is the file the measurement lives in: under the state
// directory, next to the runs (nothing is ever written under /tmp).
func measurementPath() string {
	return filepath.Join(paths.State(), "topo", "bandwidth.json")
}

// SaveMeasurement writes the measured bandwidth of the topology to the state
// directory. It is what `bernie topo --bandwidth` leaves behind, so the page
// can show the numbers later.
func SaveMeasurement(t *Topology) error {
	m := Measurement{
		MeasuredAt: time.Now(),
		Source:     t.Bandwidth,
		GBps:       map[string]float64{},
	}
	for _, p := range t.Pairs {
		if p.GBps > 0 {
			m.GBps[fmt.Sprintf("%d-%d", p.A, p.B)] = p.GBps
		}
	}
	if len(m.GBps) == 0 {
		return errors.New("no bandwidth was measured")
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	p := measurementPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// LoadMeasurement reads the last saved bandwidth measurement. It answers a
// nil measurement and no error when there is none: the page then says the
// bandwidth is not measured and names the command.
func LoadMeasurement() (*Measurement, error) {
	b, err := os.ReadFile(measurementPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var m Measurement
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if len(m.GBps) == 0 {
		return nil, nil
	}
	return &m, nil
}

// ApplyMeasurement fills the GB/s of the pairs of the topology from a saved
// measurement and records when it was taken.
func ApplyMeasurement(t *Topology, m *Measurement) {
	if m == nil {
		return
	}
	for i := range t.Pairs {
		p := &t.Pairs[i]
		if v, ok := m.GBps[fmt.Sprintf("%d-%d", p.A, p.B)]; ok {
			p.GBps = v
		}
	}
	t.MeasuredAt = m.MeasuredAt
	t.MeasuredSource = m.Source
}

// MeasureBandwidth fills Pair.GBps with unidirectional GPU-to-GPU copy
// bandwidth: rocm-bandwidth-test when installed, else (with venv) a torch
// peer-copy probe. It runs GPU copies: never use it on a busy host.
func MeasureBandwidth(ctx context.Context, t *Topology, venv string) error {
	if bin := execx.Which(BandwidthRBT); bin != "" {
		// RBT 2.6+ runs TransferBench's p2p preset; older RBT prints its
		// matrices without arguments.
		for _, args := range [][]string{{"run", "tb", "p2p"}, {}} {
			res, err := execx.Run(ctx, 10*time.Minute, bin, args...)
			if err != nil {
				return err
			}
			if bw := ParseRBT(res.Stdout); len(bw) > 0 {
				apply(t, bw)
				t.Bandwidth = BandwidthRBT
				return nil
			}
		}
	}
	if venv == "" {
		return errors.New("rocm-bandwidth-test is not installed; pass --venv for the torch peer-copy probe")
	}
	if os.Getenv("HIP_VISIBLE_DEVICES") != "" || os.Getenv("ROCR_VISIBLE_DEVICES") != "" || os.Getenv("CUDA_VISIBLE_DEVICES") != "" {
		return errors.New("unset HIP_VISIBLE_DEVICES/ROCR_VISIBLE_DEVICES/CUDA_VISIBLE_DEVICES: the probe uses HIP indices")
	}
	var env []string
	if libs := rocm.VenvLibPaths(venv); len(libs) > 0 {
		env = append(env, "LD_LIBRARY_PATH="+strings.Join(libs, ":"))
	}
	res, err := execx.RunEnv(ctx, 10*time.Minute, env, filepath.Join(venv, "bin", "python"), "-I", "-c", torchProbe)
	if err != nil {
		return err
	}
	bw, err := parseTorchProbe(res.Stdout)
	if err != nil {
		return fmt.Errorf("torch peer-copy probe: %w (%s)", err, execx.FirstLine(lastLines(res.Stderr)))
	}
	apply(t, bw)
	t.Bandwidth = BandwidthTorch
	return nil
}

// apply sets GBps per pair: the mean of the measured directions.
func apply(t *Topology, bw map[[2]int]float64) {
	for i := range t.Pairs {
		p := &t.Pairs[i]
		var sum float64
		n := 0
		for _, k := range [][2]int{{p.A, p.B}, {p.B, p.A}} {
			if v, ok := bw[k]; ok && v > 0 {
				sum += v
				n++
			}
		}
		if n > 0 {
			p.GBps = float64(int(sum/float64(n)*100+0.5)) / 100
		}
	}
}

var (
	devLineRe = regexp.MustCompile(`^\s*Device:\s*(\d+),\s*(.*)$`)
	bdfTailRe = regexp.MustCompile(`[0-9a-fA-F]{1,2}:[0-9a-fA-F]{1,2}\.[0-7]\s*$`)
)

// ParseRBT reads the unidirectional copy matrix of rocm-bandwidth-test
// (legacy "D/D" matrix with "Device: N" lines, or TransferBench "GPU NN"
// labels) and returns GB/s keyed by (src, dst) GPU ordinal. GPU ordinals
// follow the HSA agent order, which is the HIP index order.
func ParseRBT(out string) map[[2]int]float64 {
	lines := strings.Split(out, "\n")
	// Legacy device list: GPUs carry a PCI address or "GPU".
	gpuOrd := map[int]int{}
	for _, l := range lines {
		if m := devLineRe.FindStringSubmatch(l); m != nil {
			if bdfTailRe.MatchString(m[2]) || strings.Contains(m[2], "GPU") {
				id, _ := strconv.Atoi(m[1])
				gpuOrd[id] = len(gpuOrd)
			}
		}
	}
	for i := range lines {
		low := strings.ToLower(lines[i])
		if !strings.Contains(low, "unidirectional") || !strings.Contains(low, "bandwidth") {
			continue
		}
		// Header: first non-empty line after the title.
		j := i + 1
		for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
			j++
		}
		if j >= len(lines) {
			return nil
		}
		cols := headerColumns(lines[j], gpuOrd)
		if cols == nil {
			continue
		}
		out := map[[2]int]float64{}
		for k := j + 1; k < len(lines); k++ {
			l := strings.TrimSpace(lines[k])
			if l == "" {
				// Legacy matrices put blank lines between rows; TransferBench
				// separates CPU and GPU rows with one. Stop at the next title.
				if isDataRow(nextNonBlank(lines, k)) {
					continue
				}
				break
			}
			if !isDataRow(l) {
				break
			}
			src := rowLabel(lines[k], gpuOrd)
			if src == nil {
				continue // a CPU row or a "<-"/min/max row
			}
			vals := valueFields(lines[k])
			if len(vals) < len(cols) {
				continue
			}
			vals = vals[len(vals)-len(cols):]
			for c, dst := range cols {
				if dst < 0 || dst == *src {
					continue
				}
				if v, err := strconv.ParseFloat(vals[c], 64); err == nil {
					out[[2]int{*src, dst}] = v
				}
			}
		}
		return out
	}
	return nil
}

// isDataRow reports a matrix row: "<int> ..." or "GPU|CPU NN ...".
func isDataRow(l string) bool {
	f := strings.Fields(l)
	if len(f) < 2 {
		return false
	}
	return isInt(f[0]) || (f[0] == "GPU" || f[0] == "CPU") && isInt(f[1])
}

func nextNonBlank(lines []string, k int) string {
	for k++; k < len(lines); k++ {
		if strings.TrimSpace(lines[k]) != "" {
			return lines[k]
		}
	}
	return ""
}

// headerColumns maps each matrix column to a GPU ordinal (-1 for CPUs).
func headerColumns(line string, gpuOrd map[int]int) []int {
	f := strings.Fields(line)
	if len(f) < 2 {
		return nil
	}
	var cols []int
	switch {
	case f[0] == "D/D":
		for _, s := range f[1:] {
			id, err := strconv.Atoi(s)
			if err != nil {
				return nil
			}
			if o, ok := gpuOrd[id]; ok {
				cols = append(cols, o)
			} else {
				cols = append(cols, -1)
			}
		}
	case strings.Contains(f[0], `\`): // SRC+EXE\DST CPU 00 GPU 00 ...
		rest := f[1:]
		for k := 0; k+1 < len(rest); k += 2 {
			n, err := strconv.Atoi(rest[k+1])
			if err != nil {
				return nil
			}
			if rest[k] == "GPU" {
				cols = append(cols, n)
			} else {
				cols = append(cols, -1)
			}
		}
	default:
		return nil
	}
	return cols
}

// rowLabel returns the GPU ordinal of a unidirectional data row.
func rowLabel(line string, gpuOrd map[int]int) *int {
	f := strings.Fields(line)
	if len(f) == 0 {
		return nil
	}
	if f[0] == "GPU" && len(f) > 2 && f[2] == "->" {
		if n, err := strconv.Atoi(f[1]); err == nil {
			return &n
		}
	}
	if id, err := strconv.Atoi(f[0]); err == nil && len(gpuOrd) > 0 {
		if o, ok := gpuOrd[id]; ok {
			return &o
		}
	}
	return nil
}

// valueFields returns the numeric and N/A fields of a row.
func valueFields(line string) []string {
	var out []string
	f := strings.Fields(line)
	start := 1
	if len(f) > 2 && (f[0] == "GPU" || f[0] == "CPU") {
		start = 3
	}
	for _, s := range f[start:] {
		if s == "N/A" || isFloat(s) {
			out = append(out, s)
		}
	}
	return out
}

func isInt(s string) bool { _, err := strconv.Atoi(s); return err == nil }

func isFloat(s string) bool { _, err := strconv.ParseFloat(s, 64); return err == nil }

// torchProbe copies 256 MiB between every GPU pair (both directions) and
// prints GB/s as one marked JSON line.
const torchProbe = `
import json, time, torch
n = torch.cuda.device_count()
size = 256 << 20
res = {}
bufs = [torch.empty(size, dtype=torch.uint8, device="cuda:%d" % i) for i in range(n)]
for a in range(n):
    for b in range(n):
        if a == b:
            continue
        dst = torch.empty(size, dtype=torch.uint8, device="cuda:%d" % b)
        dst.copy_(bufs[a]); torch.cuda.synchronize(a); torch.cuda.synchronize(b)
        reps = 5
        t0 = time.perf_counter()
        for _ in range(reps):
            dst.copy_(bufs[a])
        torch.cuda.synchronize(a); torch.cuda.synchronize(b)
        dt = time.perf_counter() - t0
        res["%d-%d" % (a, b)] = round(size * reps / dt / 1e9, 2)
        del dst
print("BERNIE_JSON:" + json.dumps(res))
`

func parseTorchProbe(out string) (map[[2]int]float64, error) {
	for _, l := range strings.Split(out, "\n") {
		js, ok := strings.CutPrefix(strings.TrimSpace(l), "BERNIE_JSON:")
		if !ok {
			continue
		}
		var raw map[string]float64
		if err := json.Unmarshal([]byte(js), &raw); err != nil {
			return nil, err
		}
		bw := map[[2]int]float64{}
		for k, v := range raw {
			a, b, ok := strings.Cut(k, "-")
			if !ok {
				continue
			}
			x, err1 := strconv.Atoi(a)
			y, err2 := strconv.Atoi(b)
			if err1 == nil && err2 == nil {
				bw[[2]int{x, y}] = v
			}
		}
		return bw, nil
	}
	return nil, errors.New("no result")
}

func lastLines(s string) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	return l[len(l)-1]
}
