package app

import (
	"errors"
	"fmt"

	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

// ErrBenchResultNotFound means that a comparison names a saved result that
// does not exist.
var ErrBenchResultNotFound = errors.New("bench result not found")

// BenchCompare compares two saved benchmark results and returns the
// verdict. A band of zero takes the noise band of the workload.
func BenchCompare(aID, bID string, band float64) (*suite.Verdict, error) {
	a, err := bench.Get(aID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBenchResultNotFound, err)
	}
	b, err := bench.Get(bID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBenchResultNotFound, err)
	}
	cmp, err := suite.Compare(a, b)
	if err != nil {
		return nil, err
	}
	return suite.Judge(cmp, band), nil
}
