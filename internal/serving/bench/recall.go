package bench

import (
	"fmt"
	"strings"
)

// The recall check (spec §10): every prompt hides a short random fact and
// the reply must contain it. A server that does not read the whole prompt
// cannot answer, so the check tells a broken attention apart from a slow one.
const (
	// recallDepth is where the hidden fact sits in the prompt: halfway.
	recallDepth = 0.5
	// passKeySpace is a prime, so the keys of one run never repeat.
	passKeySpace = 999983
	// passKeyPhrase introduces the hidden fact.
	passKeyPhrase = "The pass key is"
	// recallQuestion asks for the hidden fact at the end of the prompt.
	recallQuestion = " Question: what is the pass key mentioned above? Answer with the pass key only."
	// recallOverheadEstimate is the token budget assumed for the hidden
	// fact and the question when the server cannot tokenize.
	recallOverheadEstimate = 32
)

// Methods of the recall check: how the prompt length was measured.
const (
	// recallMethodTokenize means the prompt was sized with /tokenize.
	recallMethodTokenize = "tokenize"
	// recallMethodEstimate means the server has no /tokenize and the
	// prompt length is an estimate.
	recallMethodEstimate = "estimate"
)

// Recall is the pass key check of one cell.
type Recall struct {
	// Asked is the number of replies that carried a pass key.
	Asked int `json:"asked"`
	// Passed is the number of replies that contained their pass key.
	Passed int `json:"passed"`
	// Depth is where the pass key sat in the prompt (0.5 is halfway).
	Depth float64 `json:"depth"`
	// Method is how the prompt length was measured: "tokenize" when the
	// server has /tokenize, "estimate" when it does not.
	Method string `json:"method"`
}

// passKey returns the hidden fact of request seq: six digits taken from the
// run nonce and the request number. The mapping is a bijection over the
// six-digit space, so two requests of a run never share a key and a key
// cannot be guessed from the request number alone.
func (p prompts) passKey(seq int) string {
	step := int64(p.nonce % passKeySpace)
	if step == 0 {
		step = 1
	}
	base := int64(p.nonce/1_000_000) % passKeySpace
	return fmt.Sprintf("%06d", (base+int64(seq)*step)%passKeySpace)
}

// passKeySentence writes the hidden fact into the prompt.
func passKeySentence(key string) string {
	return " " + passKeyPhrase + " " + key + ". Remember it."
}

// recallVerdict counts the replies of a cell that contained their pass key.
// A request that failed has no reply to look at and is not asked.
func recallVerdict(samples []sample) (asked, passed int) {
	for _, s := range samples {
		if s.err != nil || s.expect == "" {
			continue
		}
		asked++
		if strings.Contains(s.reply, s.expect) {
			passed++
		}
	}
	return asked, passed
}

// recallMethod names how the prompt length was measured.
func recallMethod(calibrated bool) string {
	if calibrated {
		return recallMethodTokenize
	}
	return recallMethodEstimate
}
