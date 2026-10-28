package bench

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"

	"github.com/opengfx1030/rdnactl/internal/serving/vllm"
)

//go:embed words.txt
var wordsTxt string

// corpus is the deduplicated embedded word list. Short common English words
// are one token in the usual BPE vocabularies, so word count tracks token
// count closely.
var corpus = func() []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range strings.Fields(wordsTxt) {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}()

// saltOverheadEstimate is the token budget assumed for the salt prefix and
// the BOS token when the server cannot tokenize.
const saltOverheadEstimate = 16

// prompts builds benchmark prompts. Each prompt is a unique salt followed by
// words drawn from the corpus with a generator seeded by (seed, seq). The salt
// is decimal digits only so its token count does not depend on its value.
type prompts struct {
	seed  int64
	nonce uint32 // random per run: salts never repeat across runs
}

func (p prompts) salt(seq int) string {
	return fmt.Sprintf("[%010d.%06d]", p.nonce, seq)
}

// build returns prompt seq with nwords words after the salt.
func (p prompts) build(seq, nwords int) string {
	r := rand.New(rand.NewPCG(uint64(p.seed), uint64(seq)))
	var b strings.Builder
	b.Grow(12 + nwords*7)
	b.WriteString(p.salt(seq))
	for range nwords {
		b.WriteByte(' ')
		b.WriteString(corpus[r.IntN(len(corpus))])
	}
	return b.String()
}

// calibration caches word counts per (url, model, seed, input length):
// tokenizers do not change while a server runs.
var calibration sync.Map

type calKey struct {
	url, model string
	seed       int64
	tokens     int
}

// tokenCounter counts the tokens of a prompt. vllm.Client.Tokenize fits.
type tokenCounter func(ctx context.Context, text string) (int, error)

// calibrate returns the largest word count whose prompt has at most target
// tokens (at least 1 word). calibrated is false when the server has no
// /tokenize endpoint and the count is an estimate.
func calibrate(ctx context.Context, p prompts, target int, count tokenCounter) (words int, calibrated bool, err error) {
	measure := func(w int) (int, error) { return count(ctx, p.build(0, w)) }
	n, err := measure(target)
	if errors.Is(err, vllm.ErrUnsupported) {
		return max(1, target-saltOverheadEstimate), false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("calibrate prompt length: %w", err)
	}
	// Invariant: tokens(lo) <= target < tokens(hi).
	lo, hi := 0, target
	if n <= target {
		lo, hi = target, 2*target
		for {
			if hi > 64*target+64 {
				return 0, false, fmt.Errorf("calibrate prompt length: token count does not grow with prompt length")
			}
			n, err := measure(hi)
			if err != nil {
				return 0, false, fmt.Errorf("calibrate prompt length: %w", err)
			}
			if n > target {
				break
			}
			lo, hi = hi, hi*2
		}
	}
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		n, err := measure(mid)
		if err != nil {
			return 0, false, fmt.Errorf("calibrate prompt length: %w", err)
		}
		if n <= target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return max(1, lo), true, nil
}

// calibrateCached wraps calibrate with the per-server cache. Estimates are
// not cached.
func calibrateCached(ctx context.Context, c *vllm.Client, model string, p prompts, target int) (int, bool, error) {
	key := calKey{c.BaseURL(), model, p.seed, target}
	if v, ok := calibration.Load(key); ok {
		return v.(int), true, nil
	}
	words, ok, err := calibrate(ctx, p, target, func(ctx context.Context, text string) (int, error) {
		return c.Tokenize(ctx, model, text)
	})
	if err == nil && ok {
		calibration.Store(key, words)
	}
	return words, ok, err
}
