package bench

import (
	"sort"
	"time"
)

// maxErrorSamples is how many distinct error messages a cell keeps.
const maxErrorSamples = 5

// metrics are the derived numbers of one successful request.
type metrics struct {
	ttft        time.Duration
	itlMS       []float64 // per-token inter-token latencies
	decodeTokS  float64   // 0 when not measurable (single chunk)
	prefillTokS float64   // 0 when the server sent no usage
	prompt      int
	completion  int
	chunks      int
}

// measure derives request metrics from chunk arrival times.
//
// With multi-token chunks (MTP and other speculative decoding) one chunk
// carries several tokens. The per-chunk token count comes from continuous
// usage stats when the server sends them; otherwise the completion tokens
// are spread evenly over the chunks. Each chunk gap is divided by its token
// count, and the per-token latency is recorded once per token, so ITL
// percentiles describe tokens, not chunks.
//
// Decode tok/s counts the tokens after the first chunk over the time between
// the first and the last chunk. Prefill tok/s is prompt tokens over TTFT.
func measure(s sample) metrics {
	m := metrics{ttft: s.chunks[0].at, prompt: s.promptTokens, completion: s.completionTokens, chunks: len(s.chunks)}
	if m.completion == 0 { // no usage: assume one token per chunk
		m.completion = len(s.chunks)
	}
	continuous := true
	for _, c := range s.chunks {
		if c.cum < 0 {
			continuous = false
			break
		}
	}
	first := 1
	if continuous {
		first = max(1, s.chunks[0].cum)
	}
	even := float64(m.completion) / float64(len(s.chunks))
	for i := 1; i < len(s.chunks); i++ {
		gap := float64(s.chunks[i].at-s.chunks[i-1].at) / float64(time.Millisecond)
		if !continuous {
			m.itlMS = append(m.itlMS, gap/even)
			continue
		}
		k := max(1, s.chunks[i].cum-s.chunks[i-1].cum)
		for range k {
			m.itlMS = append(m.itlMS, gap/float64(k))
		}
	}
	span := s.chunks[len(s.chunks)-1].at - s.chunks[0].at
	if rest := m.completion - first; rest > 0 && span > 0 {
		m.decodeTokS = float64(rest) / span.Seconds()
	}
	if m.prompt > 0 && m.ttft > 0 {
		m.prefillTokS = float64(m.prompt) / m.ttft.Seconds()
	}
	return m
}

// aggregate builds a cell from the samples of its measured requests and the
// wall time from the first send to the last completion.
func aggregate(concurrency int, samples []sample, wall time.Duration) Cell {
	c := Cell{Concurrency: concurrency, NumPrompts: len(samples), DurationS: wall.Seconds()}
	var ttft, itl []float64
	var decode, prefill []float64
	var promptSum, complSum, chunkSum int
	seen := map[string]bool{}
	for _, s := range samples {
		if s.err != nil {
			c.Failed++
			msg := s.err.Error()
			if !seen[msg] && len(c.Errors) < maxErrorSamples {
				c.Errors = append(c.Errors, msg)
			}
			seen[msg] = true
			continue
		}
		c.Completed++
		m := measure(s)
		ttft = append(ttft, float64(m.ttft)/float64(time.Millisecond))
		itl = append(itl, m.itlMS...)
		if m.decodeTokS > 0 {
			decode = append(decode, m.decodeTokS)
		}
		if m.prefillTokS > 0 {
			prefill = append(prefill, m.prefillTokS)
		}
		promptSum += m.prompt
		complSum += m.completion
		chunkSum += m.chunks
	}
	if c.Completed == 0 {
		return c
	}
	c.TTFTP50, c.TTFTP95 = percentile(ttft, 50), percentile(ttft, 95)
	c.ITLP50, c.ITLP95 = percentile(itl, 50), percentile(itl, 95)
	c.DecodeTokS = mean(decode)
	c.PrefillTokS = mean(prefill)
	if wall > 0 {
		c.OutputTokS = float64(complSum) / wall.Seconds()
	}
	c.MeanPromptTokens = float64(promptSum) / float64(c.Completed)
	c.MeanOutputTokens = float64(complSum) / float64(c.Completed)
	if chunkSum > 0 {
		c.TokensPerChunk = float64(complSum) / float64(chunkSum)
	}
	return c
}

// percentile uses linear interpolation between closest ranks (numpy's
// default). It sorts xs in place; 0 for an empty slice.
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sort.Float64s(xs)
	rank := p / 100 * float64(len(xs)-1)
	lo := int(rank)
	if lo >= len(xs)-1 {
		return xs[len(xs)-1]
	}
	frac := rank - float64(lo)
	return xs[lo] + frac*(xs[lo+1]-xs[lo])
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var t float64
	for _, x := range xs {
		t += x
	}
	return t / float64(len(xs))
}
