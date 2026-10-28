package gpu

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/support/execx"
)

// ECCState is the ECC state of a card.
type ECCState string

// The ECC states of a card.
const (
	// ECCEnabled: the kernel counts the memory errors of the card.
	ECCEnabled ECCState = "enabled"
	// ECCDisabled: the card has ECC, the kernel does not count its errors.
	ECCDisabled ECCState = "disabled"
	// ECCSupported: the card has ECC, the host exposes no RAS interface.
	ECCSupported ECCState = "supported"
	// ECCUnsupported: the card has no ECC.
	ECCUnsupported ECCState = "unsupported"
)

// rasDir is the RAS directory of a card, below its device directory.
const rasDir = "ras"

// rasParam is the amdgpu module parameter that turns the RAS counters on.
const rasParam = "sys/module/amdgpu/parameters/ras_enable"

// ECC is the ECC state of one GPU.
type ECC struct {
	State ECCState
	// CE counts the corrected errors, UE the uncorrectable ones and DE the
	// degraded ones.
	CE, UE, DE uint64
	// Counters is true when the counters were read.
	Counters bool
	// Source names where the counters came from: "sysfs" or "amd-smi".
	Source string
	// Mask is the RAS feature mask the kernel published.
	Mask string
	// RASParam is the amdgpu.ras_enable module parameter of the host:
	// "-1" auto, "0" off, "1" on.
	RASParam string
}

// ECCAll reads the ECC state of every GPU under root from sysfs.
func ECCAll(root string, gpus []GPU) []ECC {
	out := make([]ECC, len(gpus))
	for i, g := range gpus {
		out[i] = ECCOf(root, g)
	}
	return out
}

// ECCOf reads the ECC state of one GPU from the RAS files of its device
// directory (/sys/class/drm/cardN/device/ras). The counters exist only when
// the kernel counts the memory errors of the card, which needs ECC memory.
// The device tables say which cards have ECC: without them a card without
// counters has no ECC, with them it has ECC that is off.
func ECCOf(root string, g GPU) ECC {
	e := ECC{State: ECCUnsupported, RASParam: readString(filepath.Join(root, rasParam))}
	if g.SysfsPath == "" {
		return e
	}
	ras := filepath.Join(root, g.SysfsPath, rasDir)
	if _, err := os.Stat(ras); err != nil {
		if hasECC(g) {
			e.State = ECCSupported
		}
		return e
	}
	e.Mask = readString(filepath.Join(ras, "features"))
	body, err := os.ReadFile(filepath.Join(ras, "umc_err_count"))
	if err != nil {
		if hasECC(g) {
			e.State = ECCDisabled
		}
		return e
	}
	ce, ue, de, ok := ParseUMCCounters(string(body))
	if !ok {
		if hasECC(g) {
			e.State = ECCDisabled
		}
		return e
	}
	e.State = ECCEnabled
	e.CE, e.UE, e.DE = ce, ue, de
	e.Counters = true
	e.Source = "sysfs"
	return e
}

// ParseUMCCounters parses the ras/umc_err_count file: one "ue: 0", "ce: 0"
// and "de: 0" line each. It reports the corrected, the uncorrectable and the
// degraded count, and whether it read a counter at all.
func ParseUMCCounters(text string) (ce, ue, de uint64, ok bool) {
	for _, line := range strings.Split(text, "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(name) {
		case "ce":
			ce = v
		case "ue":
			ue = v
		case "de":
			de = v
		default:
			continue
		}
		ok = true
	}
	return ce, ue, de, ok
}

// hasECC reports whether the device tables say the card counts its memory
// errors, that is: it has ECC.
func hasECC(g GPU) bool {
	d, ok := Lookup(g.DeviceID)
	return ok && d.ECC
}

// smiTimeout bounds the amd-smi calls.
const smiTimeout = 15 * time.Second

// ECCTool fills the counters of the cards that sysfs does not cover with
// `amd-smi metric --json`, matched by PCI address. It reads only.
func ECCTool(ctx context.Context, gpus []GPU, ecc []ECC, amdSMI string) []ECC {
	if amdSMI == "" || len(ecc) != len(gpus) {
		return ecc
	}
	need := false
	for _, e := range ecc {
		if !e.Counters {
			need = true
		}
	}
	if !need {
		return ecc
	}
	list, err := execx.Run(ctx, smiTimeout, amdSMI, "list", "--json")
	if err != nil || !list.OK() {
		return ecc
	}
	idx := smiIndex(list.Stdout)
	if len(idx) == 0 {
		return ecc
	}
	res, err := execx.Run(ctx, smiTimeout, amdSMI, "metric", "--json")
	if err != nil || !res.OK() {
		return ecc
	}
	counters := smiCounters(res.Stdout)
	for i, g := range gpus {
		if ecc[i].Counters {
			continue
		}
		j, ok := idx[strings.ToLower(g.PCI)]
		if !ok {
			continue
		}
		c, ok := counters[j]
		if !ok {
			continue
		}
		ecc[i].CE, ecc[i].UE, ecc[i].DE = c.ce, c.ue, c.de
		ecc[i].Counters = true
		ecc[i].State = ECCEnabled
		ecc[i].Source = "amd-smi"
	}
	return ecc
}

// eccCounters are the three ECC counters of a card.
type eccCounters struct {
	ce, ue, de uint64
}

// smiIndex maps the PCI address of every card to its amd-smi index.
func smiIndex(out string) map[string]int {
	var list []struct {
		GPU int    `json:"gpu"`
		BDF string `json:"bdf"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return nil
	}
	idx := map[string]int{}
	for _, e := range list {
		if e.BDF != "" {
			idx[strings.ToLower(e.BDF)] = e.GPU
		}
	}
	return idx
}

// smiCount is a counter that amd-smi prints as a number or as "N/A".
type smiCount uint64

// UnmarshalJSON reads a number, a quoted number or "N/A".
func (c *smiCount) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "N/A" || s == "" {
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return nil
	}
	*c = smiCount(v)
	return nil
}

// smiCounters maps the amd-smi index of every card to its ECC counters.
func smiCounters(out string) map[int]eccCounters {
	var doc struct {
		GPUs []struct {
			GPU int `json:"gpu"`
			ECC struct {
				Correctable   smiCount `json:"total_correctable_count"`
				Uncorrectable smiCount `json:"total_uncorrectable_count"`
				Deferred      smiCount `json:"total_deferred_count"`
			} `json:"ecc"`
		} `json:"gpu_data"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return nil
	}
	out2 := map[int]eccCounters{}
	for _, g := range doc.GPUs {
		out2[g.GPU] = eccCounters{ce: uint64(g.ECC.Correctable), ue: uint64(g.ECC.Uncorrectable), de: uint64(g.ECC.Deferred)}
	}
	return out2
}
