package gpu

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// RestrictionVars are the environment variables that hide GPUs from bernie
// (spec §7.7). The first one that is set wins.
var RestrictionVars = []string{"HIP_VISIBLE_DEVICES", "ROCR_VISIBLE_DEVICES"}

// Restriction is the GPU restriction of the environment: the GPUs bernie
// may use. The indices are the host HIP indices, the numbering the plan
// writes into HIP_VISIBLE_DEVICES of the child process.
type Restriction struct {
	Source  string // HIP_VISIBLE_DEVICES or ROCR_VISIBLE_DEVICES
	Value   string // the raw value
	Indices []int  // the host HIP indices
}

// Visibility is the GPU set bernie may use and the restriction behind it.
type Visibility struct {
	GPUs []GPU
	// Restriction is nil when nothing is restricted.
	Restriction *Restriction
	// Warning says why a restriction value was ignored.
	Warning string
}

// RestrictionOf reads the restriction of the environment. It returns nil
// when nothing is restricted. A value that is not a comma list of indices (a
// GPU UUID, for example) is ignored: the warning says why. The indices are
// the host HIP indices; the order of the GPUs in the KFD topology is not
// guaranteed to be the order the variables use.
func RestrictionOf(getenv func(string) string) (*Restriction, string) {
	for _, name := range RestrictionVars {
		v := strings.TrimSpace(getenv(name))
		if v == "" {
			continue
		}
		idx, err := parseIndices(v)
		if err != nil {
			return nil, fmt.Sprintf("%s=%s is not a list of GPU indices; bernie ignores it", name, v)
		}
		return &Restriction{Source: name, Value: v, Indices: idx}, ""
	}
	return nil, ""
}

// EnvRestriction reads the restriction of the environment of this process.
func EnvRestriction() (*Restriction, string) { return RestrictionOf(os.Getenv) }

// RestrictionError is a restriction the host cannot honor: it names a GPU
// the host does not have.
type RestrictionError struct {
	Source string
	Index  int
}

func (e *RestrictionError) Error() string {
	return fmt.Sprintf("%s names GPU %d, which this host does not have", e.Source, e.Index)
}

// Restricted reports whether err is a restriction the host cannot honor. The
// callers stop on it; a host without a KFD topology is not one.
func Restricted(err error) bool {
	var re *RestrictionError
	return errors.As(err, &re)
}

// Apply keeps the GPUs of the restriction. An index the host does not have
// is an error.
func (r *Restriction) Apply(gpus []GPU) ([]GPU, error) {
	byIdx := map[int]GPU{}
	for _, g := range gpus {
		byIdx[g.HIPIndex] = g
	}
	out := []GPU{}
	for _, i := range r.Indices {
		g, ok := byIdx[i]
		if !ok {
			return nil, &RestrictionError{Source: r.Source, Index: i}
		}
		out = append(out, g)
	}
	return out, nil
}

// Note is the one line that says the restriction is active.
func (r *Restriction) Note() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("GPUs limited to %s by %s", joinInts(r.Indices), r.Source)
}

// Visible returns the GPUs bernie may use on this host: the GPUs of the KFD
// topology minus the ones the environment hides (spec §7.7). A restriction
// that names a GPU the host does not have is an error.
func Visible(root string) (Visibility, error) {
	r, warn := EnvRestriction()
	v := Visibility{Restriction: r, Warning: warn}
	gpus, err := List(root)
	if err != nil {
		return v, err // the restriction of the environment is known anyway
	}
	v.GPUs = gpus
	if r == nil {
		return v, nil
	}
	kept, err := r.Apply(gpus)
	if err != nil {
		return v, err
	}
	v.GPUs = kept
	return v, nil
}

// parseIndices reads a comma list of HIP indices.
func parseIndices(v string) ([]int, error) {
	out := []int{}
	for _, f := range strings.Split(v, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("want comma-separated HIP indices, got %q", v)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no GPU index in %q", v)
	}
	return out, nil
}

// joinInts renders indices as a comma list.
func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ",")
}
