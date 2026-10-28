package bench

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrWorkloadDiffers reports that two results do not measure the same thing.
var ErrWorkloadDiffers = errors.New("workload differs")

// Comparison pairs two results that measure the same workload.
type Comparison struct {
	// Workload is the workload both results measured.
	Workload Workload `json:"workload"`
	A        *Result  `json:"a"`
	B        *Result  `json:"b"`
}

// Compare pairs two results that measure the same workload. It returns
// ErrWorkloadDiffers when they do not: the numbers of two different
// workloads cannot be put side by side.
func Compare(a, b *Result) (*Comparison, error) {
	if a == nil || b == nil {
		return nil, errors.New("two results are needed")
	}
	if diff := workloadDiff(a.Workload, b.Workload); len(diff) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrWorkloadDiffers, strings.Join(diff, "; "))
	}
	return &Comparison{Workload: *a.Workload, A: a, B: b}, nil
}

// workloadDiff lists the fields in which two workloads differ.
func workloadDiff(a, b *Workload) []string {
	if a == nil || b == nil {
		return []string{"one result has no workload (saved before the field existed)"}
	}
	var diff []string
	if a.ProtocolID != b.ProtocolID {
		diff = append(diff, fmt.Sprintf("protocol %s vs %s", a.ProtocolID, b.ProtocolID))
	}
	if a.CorpusSHA256 != b.CorpusSHA256 {
		diff = append(diff, "corpus differs")
	}
	if a.InputLen != b.InputLen {
		diff = append(diff, fmt.Sprintf("input %d vs %d tokens", a.InputLen, b.InputLen))
	}
	if a.OutputLen != b.OutputLen {
		diff = append(diff, fmt.Sprintf("output %d vs %d tokens", a.OutputLen, b.OutputLen))
	}
	if !slices.Equal(a.Concurrency, b.Concurrency) {
		diff = append(diff, fmt.Sprintf("concurrency %v vs %v", a.Concurrency, b.Concurrency))
	}
	if a.Recall != b.Recall {
		diff = append(diff, fmt.Sprintf("recall %s vs %s", onOff(a.Recall), onOff(b.Recall)))
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
