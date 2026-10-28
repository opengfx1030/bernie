package doctor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/opengfx1030/rdnactl/internal/machine/gpu"
	"github.com/opengfx1030/rdnactl/internal/serving/tuning"
)

func tuningChecks(in inputs) []Check {
	const cat = "tuning"
	var out []Check
	switch {
	case in.profErr != nil:
		out = append(out, check(cat, "tunableop", Fail, "TunableOp profile matches the loaded rocBLAS", "profile registry: "+in.profErr.Error()))
	case in.rocblas == "":
		out = append(out, check(cat, "tunableop", Skip, "TunableOp profile matches the loaded rocBLAS", "no librocblas.so.5 found").with("", docTunable))
	case in.libSHA == "":
		out = append(out, check(cat, "tunableop", Skip, "TunableOp profile matches the loaded rocBLAS", "cannot read "+in.rocblas).with("", docTunable))
	case in.match != nil:
		out = append(out, check(cat, "tunableop", OK, "TunableOp profile matches the loaded rocBLAS",
			fmt.Sprintf("%s (rocBLAS %s, sha %s) for %s", in.match.Name, in.match.ROCBLAS, in.libSHA[:tuning.SHAPrefix], in.rocblas)).with("", docTunable))
	default:
		out = append(out, check(cat, "tunableop", Warn, "TunableOp profile matches the loaded rocBLAS",
			fmt.Sprintf("no registered profile for %s (sha %s, %d profiles known); rows go to %s", in.rocblas, in.libSHA[:tuning.SHAPrefix], len(in.profiles), tuning.PerUserFile())).
			with("rdnactl tune tunableop env --tuning  # tune rows once for this rocBLAS build", docMismatch))
	}
	out = append(out, powerCheck(in))
	return out
}

// powerCheck compares each GPU's power cap with the advice of its device
// table row (only rows with a sourced advice have one).
func powerCheck(in inputs) Check {
	const title = "Power cap per GPU"
	if len(in.gpus) == 0 {
		return check("tuning", "power", Skip, title, "no GPUs")
	}
	var caps, high, order []string
	idx := map[int][]string{}
	for _, g := range in.gpus {
		caps = append(caps, fmt.Sprintf("%d:%.0fW", g.HIPIndex, g.PowerCapW))
		d, ok := gpu.Lookup(g.DeviceID)
		if !ok || d.PowerCapAdviceW == 0 || g.PowerCapW <= float64(d.PowerCapAdviceW) {
			continue
		}
		high = append(high, fmt.Sprintf("%s above %d W: GPU %d %.0f W", d.Name, d.PowerCapAdviceW, g.HIPIndex, g.PowerCapW))
		if len(idx[d.PowerCapAdviceW]) == 0 {
			order = append(order, fmt.Sprint(d.PowerCapAdviceW))
		}
		idx[d.PowerCapAdviceW] = append(idx[d.PowerCapAdviceW], fmt.Sprint(g.HIPIndex))
	}
	if len(high) == 0 {
		return check("tuning", "power", OK, title, strings.Join(caps, " ")).with("", docPower)
	}
	var fixes []string
	for _, w := range order {
		n, _ := strconv.Atoi(w)
		fixes = append(fixes, fmt.Sprintf("rdnactl tune power set %d --gpu %s", n, strings.Join(idx[n], " --gpu ")))
	}
	return check("tuning", "power", Warn, title, strings.Join(high, "; ")).with(strings.Join(fixes, " && "), docPower)
}
