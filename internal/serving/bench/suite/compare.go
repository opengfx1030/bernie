package suite

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// ErrWorkloadDiffers reports that two results do not measure the same thing.
var ErrWorkloadDiffers = errors.New("workload differs")

// Comparison pairs two results that measure the same workload.
type Comparison struct {
	// Workload is the workload both results measured.
	Workload bench.Workload `json:"workload"`
	A        *bench.Result  `json:"a"`
	B        *bench.Result  `json:"b"`
}

// Compare pairs two results that measure the same workload. It returns
// ErrWorkloadDiffers when they do not: the numbers of two different
// workloads cannot be put side by side. Two results of different protocol
// ids are refused too: they measured different things, so only the
// numbers both of them measured could be compared, and those are not
// worth a table. Two runs of different suites are refused as well: the
// suite digest pins the cells of a run.
func Compare(a, b *bench.Result) (*Comparison, error) {
	if a == nil || b == nil {
		return nil, errors.New("two results are needed")
	}
	if diff := workloadDiff(a.Workload, b.Workload); len(diff) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrWorkloadDiffers, strings.Join(diff, "; "))
	}
	return &Comparison{Workload: *a.Workload, A: a, B: b}, nil
}

// workloadDiff lists the fields in which two workloads differ.
func workloadDiff(a, b *bench.Workload) []string {
	if a == nil || b == nil {
		return []string{"one result has no workload (saved before the field existed)"}
	}
	var diff []string
	if a.ProtocolID != b.ProtocolID {
		diff = append(diff, fmt.Sprintf("protocol %s vs %s (the two results measure different things)", a.ProtocolID, b.ProtocolID))
	}
	if a.CorpusSHA256 != b.CorpusSHA256 {
		diff = append(diff, "corpus differs")
	}
	if a.SuiteID != b.SuiteID {
		diff = append(diff, fmt.Sprintf("suite %s vs %s", a.SuiteID, b.SuiteID))
	}
	if a.SuiteDigest != b.SuiteDigest {
		diff = append(diff, "the suite digest differs (the two runs used different suites)")
	}
	if a.Kind != b.Kind {
		diff = append(diff, fmt.Sprintf("kind %s vs %s", a.Kind, b.Kind))
	}
	if a.Recall != b.Recall {
		diff = append(diff, fmt.Sprintf("recall %s vs %s", onOff(a.Recall), onOff(b.Recall)))
	}
	// A suite run carries the lens of its cells, and the digest pins them.
	if a.SuiteID == "" && b.SuiteID == "" {
		if a.InputLen != b.InputLen {
			diff = append(diff, fmt.Sprintf("input %d vs %d tokens", a.InputLen, b.InputLen))
		}
		if a.OutputLen != b.OutputLen {
			diff = append(diff, fmt.Sprintf("output %d vs %d tokens", a.OutputLen, b.OutputLen))
		}
		if !slices.Equal(a.Concurrency, b.Concurrency) {
			diff = append(diff, fmt.Sprintf("concurrency %v vs %v", a.Concurrency, b.Concurrency))
		}
	}
	return diff
}

// onOff names a flag in words.
func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
