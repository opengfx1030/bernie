package bench

import (
	"sort"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// maxErrorSamples is how many distinct error messages a cell keeps.
const maxErrorSamples = 5

// Dist is the distribution of a measured number over the requests of a
// cell. Every new metric has this shape: the mean, the percentiles and the
// range. A field that was not measured is absent and shows as "–".
type Dist struct {
	Mean float64 `json:"mean,omitempty"`
	P50  float64 `json:"p50,omitempty"`
	P95  float64 `json:"p95,omitempty"`
	Min  float64 `json:"min,omitempty"`
	Max  float64 `json:"max,omitempty"`
}

// Rate is a speed: the rate of the whole cell and the distribution of the
// rates of its requests.
type Rate struct {
	// AggTokS is the rate of the whole cell in tokens per second.
	AggTokS float64 `json:"agg_tok_s,omitempty"`
	// WindowS is the time the aggregate rate was measured over.
	WindowS float64 `json:"window_s,omitempty"`
	// Req is the distribution of the per-request rates.
	Req *Dist `json:"req,omitempty"`
}

// ServerSide is what the server itself measured over the same requests. It
// comes from the latency histograms of its /metrics endpoint and is absent
// when the server has no metrics.
type ServerSide struct {
	// Source names the metrics the numbers come from.
	Source string `json:"source,omitempty"`
	// TTFTs is the server's time to first token in seconds.
	TTFTs *Dist `json:"ttft_s,omitempty"`
	// ITLms is the server's inter-token latency in milliseconds.
	ITLms *Dist `json:"itl_ms,omitempty"`
	// Count is the number of requests the server measured.
	Count int `json:"count,omitempty"`
}

// metrics are the derived numbers of one successful request.
type metrics struct {
	ttft        time.Duration
	itlMS       []float64 // per-token inter-token latencies
	decodeTokS  float64   // 0 when not measurable (single chunk)
	prefillTokS float64   // 0 when the server sent no usage
	prompt      int
	completion  int
	chunks      int
	firstTokens int           // tokens of the first chunk
	start       time.Duration // request sent, relative to the cell start
	first       time.Duration // first token, relative to the cell start
	last        time.Duration // last token, relative to the cell start
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
	m.firstTokens = first
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
	m.start = s.started
	m.first = s.started + s.chunks[0].at
	m.last = s.started + s.chunks[len(s.chunks)-1].at
	return m
}

// span is an interval of the cell timeline.
type span struct{ from, to time.Duration }

// union returns the total length of the merged intervals: the time in which
// at least one request was inside the interval.
func union(spans []span) time.Duration {
	if len(spans) == 0 {
		return 0
	}
	s := append([]span(nil), spans...)
	sort.Slice(s, func(i, j int) bool { return s[i].from < s[j].from })
	var total time.Duration
	cur := s[0]
	for _, sp := range s[1:] {
		if sp.from <= cur.to {
			cur.to = max(cur.to, sp.to)
			continue
		}
		total += cur.to - cur.from
		cur = sp
	}
	return total + cur.to - cur.from
}

// aggregate builds a cell from the samples of its measured requests and the
// wall time from the first send to the last completion.
//
// The cell holds five groups of numbers: the prefill speed, the decode
// speed, the time to first token, the inter-token latency and the output
// rate of the cell.
//
// The aggregate prefill rate is the prompt tokens of the cell over the time
// from the first request of the cell to the last first token. The aggregate
// decode rate is the decode tokens of the cell over the time in which any
// request was decoding.
func aggregate(concurrency int, samples []sample, wall time.Duration) Cell {
	c := Cell{Concurrency: concurrency, NumPrompts: len(samples), DurationS: wall.Seconds(), TokensExact: true}
	var ttftS, itlMS, prefill, decode []float64
	var promptSum, complSum, chunkSum, cachedSum, decodeTokens int
	var spans []span
	var firstStart, lastFirst time.Duration
	started := false
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
		ttftS = append(ttftS, m.ttft.Seconds())
		itlMS = append(itlMS, m.itlMS...)
		if m.prefillTokS > 0 {
			prefill = append(prefill, m.prefillTokS)
		}
		if m.decodeTokS > 0 {
			decode = append(decode, m.decodeTokS)
		}
		promptSum += m.prompt
		complSum += m.completion
		chunkSum += m.chunks
		cachedSum += s.cachedTokens
		decodeTokens += max(0, m.completion-m.firstTokens)
		if s.promptTokens == 0 || s.completionTokens == 0 {
			c.TokensExact = false
		}
		spans = append(spans, span{from: m.first, to: m.last})
		if !started {
			firstStart, lastFirst = m.start, m.first
			started = true
			continue
		}
		firstStart = min(firstStart, m.start)
		lastFirst = max(lastFirst, m.first)
	}
	if c.Completed == 0 {
		return c
	}
	c.TTFTs = distOf(ttftS)
	c.ITLms = distOf(itlMS)

	pf := &Rate{Req: distOf(prefill)}
	if lastFirst > firstStart {
		pf.WindowS = (lastFirst - firstStart).Seconds()
		if promptSum > 0 {
			pf.AggTokS = float64(promptSum) / pf.WindowS
		}
	}
	c.Prefill = pf

	dc := &Rate{Req: distOf(decode)}
	if w := union(spans); w > 0 {
		dc.WindowS = w.Seconds()
		if decodeTokens > 0 {
			dc.AggTokS = float64(decodeTokens) / dc.WindowS
		}
	}
	c.Decode = dc

	// The flat fields are the same numbers in the old shape, so a client
	// that knows only the old result still reads them.
	if c.TTFTs != nil {
		c.TTFTP50 = c.TTFTs.P50 * 1000
		c.TTFTP95 = c.TTFTs.P95 * 1000
	}
	if c.ITLms != nil {
		c.ITLP50 = c.ITLms.P50
		c.ITLP95 = c.ITLms.P95
	}
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
	if cachedSum > 0 {
		c.MeanCachedPromptTokens = float64(cachedSum) / float64(c.Completed)
	}
	return c
}

// serverSide is the server's own measurement of the cell, from the latency
// histograms of its /metrics endpoint. It is nil when the server has no
// metrics or when it measured nothing during the cell.
func serverSide(before, after map[string]float64) *ServerSide {
	if before == nil || after == nil {
		return nil
	}
	names := []string{vllm.MetricTimeToFirstToken, vllm.MetricInterTokenLatency}
	bh := vllm.Histograms(before, names...)
	ah := vllm.Histograms(after, names...)
	ss := &ServerSide{Source: names[0] + ", " + names[1]}
	if d, ok := histDelta(bh[names[0]], ah[names[0]]); ok {
		ss.TTFTs = &Dist{Mean: d.Mean(), P50: d.Quantile(0.5), P95: d.Quantile(0.95)}
		ss.Count = int(d.Count)
	}
	if d, ok := histDelta(bh[names[1]], ah[names[1]]); ok {
		ss.ITLms = &Dist{Mean: d.Mean() * 1000, P50: d.Quantile(0.5) * 1000, P95: d.Quantile(0.95) * 1000}
	}
	if ss.TTFTs == nil && ss.ITLms == nil {
		return nil
	}
	return ss
}

// histDelta returns the observations a server recorded between two reads of
// a histogram. It reports false when there were none.
func histDelta(before, after vllm.Histogram) (vllm.Histogram, bool) {
	if after.Count <= 0 {
		return vllm.Histogram{}, false
	}
	d, ok := before.Delta(after)
	if !ok || d.Count <= 0 {
		return vllm.Histogram{}, false
	}
	return d, true
}

// upgrade fills the metrics of a cell saved before the new shape: the old
// millisecond fields become the new distributions, and the old flat rates
// become the mean of the per-request rates. What the old result did not
// measure stays absent and shows as "–".
func (c *Cell) upgrade() {
	if c.TTFTs == nil && (c.TTFTP50 > 0 || c.TTFTP95 > 0) {
		c.TTFTs = &Dist{P50: c.TTFTP50 / 1000, P95: c.TTFTP95 / 1000}
	}
	if c.ITLms == nil && (c.ITLP50 > 0 || c.ITLP95 > 0) {
		c.ITLms = &Dist{P50: c.ITLP50, P95: c.ITLP95}
	}
	if c.Prefill == nil && c.PrefillTokS > 0 {
		c.Prefill = &Rate{Req: &Dist{Mean: c.PrefillTokS}}
	}
	if c.Decode == nil && c.DecodeTokS > 0 {
		c.Decode = &Rate{Req: &Dist{Mean: c.DecodeTokS}}
	}
}

// upgrade fills the metrics of the cells of a result saved before the new
// shape.
func (r *Result) upgrade() {
	for i := range r.Cells {
		r.Cells[i].upgrade()
	}
}

// distOf returns the distribution of xs, or nil when there is nothing to
// summarize: an absent distribution shows as "–".
func distOf(xs []float64) *Dist {
	if len(xs) == 0 {
		return nil
	}
	d := Summarize(xs)
	return &d
}

// Summarize returns the distribution of xs. It is the only place that
// computes a mean or a percentile, so the CLI, the JSON and the dashboard
// show the same numbers.
func Summarize(xs []float64) Dist {
	if len(xs) == 0 {
		return Dist{}
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return Dist{
		Mean: mean(s),
		P50:  quantile(s, 0.5),
		P95:  quantile(s, 0.95),
		Min:  s[0],
		Max:  s[len(s)-1],
	}
}

// quantile returns the q-quantile of a sorted slice with linear
// interpolation between the closest ranks (numpy's default).
func quantile(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	rank := q * float64(n-1)
	lo := int(rank)
	if lo >= n-1 {
		return sorted[n-1]
	}
	frac := rank - float64(lo)
	return sorted[lo] + frac*(sorted[lo+1]-sorted[lo])
}

// mean returns the mean of xs.
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
