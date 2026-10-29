package telemetry

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// The windows the history answers.
const (
	Window5m  = "5m"
	Window15m = "15m"
	Window1h  = "1h"
)

// The retention of the ring buffer and the spacing of its points.
const (
	historyCapacity = 900             // 30 minutes at one point every 2 s
	historyStep     = 2 * time.Second // the spacing of the points
	historyCache    = 2 * time.Second // how long an answer is reused
	// maxSeriesPoints is the number of points a series of the history
	// holds. A longer window is downsampled further, so the answer stays
	// small enough to paint and to download.
	maxSeriesPoints = 240
	// maxBuckets is the number of buckets a downsampled window is split
	// into. Every bucket keeps two points, its lowest and its highest
	// value, so a series never holds more than maxSeriesPoints points.
	maxBuckets = maxSeriesPoints / 2
)

// History is the ring buffer of the samples of the last minutes. The hub
// owns it, the sampler fills it, and a window of it is the answer of
// GET /api/gpus/history.
type History struct {
	mu       sync.Mutex
	pts      []point // the ring, always historyCapacity long
	head     int     // the index of the next write
	n        int     // the number of points in the ring
	last     time.Time
	cached   *HistoryDocument
	cachedAt time.Time
}

// point is one sample of every card.
type point struct {
	at         time.Time
	cards      []cardPoint
	tokS       float64
	promptTokS float64
}

// cardPoint is what the history keeps of one card.
type cardPoint struct {
	hip   int
	busy  float64
	power float64
	vram  uint64
	// The slow fields: the temperature and the clocks. They are nil when
	// the point holds no slow measurement: a gap is a gap, not a value
	// carried forward.
	temp *float64
	sclk *int
	mclk *int
}

// HistorySeries is the history of one card: one value per point.
type HistorySeries struct {
	HIPIndex         int       `json:"hip_index"`
	BusyPct          []float64 `json:"busy_pct"`
	PowerW           []float64 `json:"power_w"`
	VRAMUsed         []uint64  `json:"vram_used"`
	TokensPerS       []float64 `json:"tokens_per_s"`
	PromptTokensPerS []float64 `json:"prompt_tok_s"`
	// TempC is the temperature of the card and SclkMHz, MclkMHz its
	// clocks. They are null at the points that hold no slow measurement.
	TempC   []*float64 `json:"temp_c"`
	SclkMHz []*int     `json:"sclk_mhz"`
	MclkMHz []*int     `json:"mclk_mhz"`
}

// HistoryDocument is the answer of GET /api/gpus/history.
type HistoryDocument struct {
	Window    string          `json:"window"`
	IntervalS int             `json:"interval_s"`
	Points    int             `json:"points"`
	Series    []HistorySeries `json:"series"`
}

// NewHistory returns an empty ring buffer.
func NewHistory() *History {
	return &History{pts: make([]point, historyCapacity)}
}

// Add records a sample of the cards. It keeps one point every historyStep;
// the samples in between are dropped. tokS is the output rate of the live
// run and promptTokS its prefill rate: the rates are properties of the
// server, so every card carries them, and they are zero when no run serves.
//
// slowFresh says that the slow fields of this sample were measured now. A
// point without a slow measurement holds no slow value: the slow series of
// the answer carries a gap there, never a value carried forward.
func (h *History) Add(at time.Time, gpus []gpu.Sample, tokS, promptTokS float64, slowFresh bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.last.IsZero() && at.Sub(h.last) < historyStep {
		return
	}
	h.last = at
	cards := make([]cardPoint, 0, len(gpus))
	for _, g := range gpus {
		c := cardPoint{
			hip:   g.HIPIndex,
			busy:  round1(g.BusyPct),
			power: round1(g.PowerW),
			vram:  g.VRAMUsed,
		}
		if slowFresh {
			if g.TempC != 0 {
				t := g.TempC
				c.temp = &t
			}
			c.sclk = g.SclkMHz
			c.mclk = g.MclkMHz
		}
		cards = append(cards, c)
	}
	h.pts[h.head] = point{
		at: at, cards: cards,
		tokS:       round1(tokS),
		promptTokS: round1(promptTokS),
	}
	h.head = (h.head + 1) % historyCapacity
	if h.n < historyCapacity {
		h.n++
	}
}

// Window returns the document of a window: "5m", "15m" or "1h". The answer
// is reused for historyCache seconds, so a page that asks often does not
// rebuild it. A window that holds more points than the cap is downsampled
// with MinMax: every bucket of the window keeps its lowest and its highest
// value, so a peak of one sample is never lost.
func (h *History) Window(name string) (HistoryDocument, error) {
	dur, err := windowSpec(name)
	if err != nil {
		return HistoryDocument{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	if h.cached != nil && h.cached.Window == name && now.Sub(h.cachedAt) < historyCache {
		return *h.cached, nil
	}
	doc := h.build(name, dur, now)
	h.cached, h.cachedAt = &doc, now
	return doc, nil
}

// windowSpec returns the length of a window.
func windowSpec(name string) (time.Duration, error) {
	switch name {
	case Window5m:
		return 5 * time.Minute, nil
	case Window15m:
		return 15 * time.Minute, nil
	case Window1h:
		return time.Hour, nil
	}
	return 0, fmt.Errorf("window %q is not one of %s, %s or %s", name, Window5m, Window15m, Window1h)
}

// build assembles the document of a window. The caller holds the lock.
func (h *History) build(name string, dur time.Duration, now time.Time) HistoryDocument {
	sel := h.points(now.Add(-dur))
	hips := h.hips(sel)
	buckets, width := bucketPlan(sel)
	var bucketOf []int
	if width > 0 {
		bucketOf = bucketIndexes(sel, width)
	}
	points := len(sel)
	if bucketOf != nil {
		points = 2 * buckets
	}
	// The interval of the answer is the width of a bucket, or the spacing
	// of the ring when the window is not downsampled.
	interval := width
	if interval == 0 {
		interval = int(historyStep / time.Second)
	}
	doc := HistoryDocument{
		Window:    name,
		IntervalS: interval,
		Points:    points,
		Series:    make([]HistorySeries, 0, len(hips)),
	}
	for _, hip := range hips {
		s := HistorySeries{
			HIPIndex:         hip,
			BusyPct:          make([]float64, 0, points),
			PowerW:           make([]float64, 0, points),
			VRAMUsed:         make([]uint64, 0, points),
			TokensPerS:       make([]float64, 0, points),
			PromptTokensPerS: make([]float64, 0, points),
			TempC:            make([]*float64, 0, points),
			SclkMHz:          make([]*int, 0, points),
			MclkMHz:          make([]*int, 0, points),
		}
		busy := make([]sampleValue, 0, len(sel))
		power := make([]sampleValue, 0, len(sel))
		vram := make([]sampleValue, 0, len(sel))
		tokens := make([]sampleValue, 0, len(sel))
		prompt := make([]sampleValue, 0, len(sel))
		temp := make([]sampleValue, 0, len(sel))
		sclk := make([]sampleValue, 0, len(sel))
		mclk := make([]sampleValue, 0, len(sel))
		var lastBusy, lastPower float64
		var lastVram uint64
		for _, p := range sel {
			var card *cardPoint
			for i := range p.cards {
				if p.cards[i].hip == hip {
					card = &p.cards[i]
					break
				}
			}
			if card != nil {
				lastBusy, lastPower, lastVram = card.busy, card.power, card.vram
			}
			// A card that is gone keeps its last value: the chart shows no
			// dip that never happened.
			busy = append(busy, measured(lastBusy))
			power = append(power, measured(lastPower))
			vram = append(vram, measured(float64(lastVram)))
			tokens = append(tokens, measured(p.tokS))
			prompt = append(prompt, measured(p.promptTokS))
			// The slow fields are never carried forward: a point without a
			// slow measurement is a gap.
			if card == nil {
				temp = append(temp, sampleValue{})
				sclk = append(sclk, sampleValue{})
				mclk = append(mclk, sampleValue{})
				continue
			}
			temp = append(temp, optional(card.temp))
			sclk = append(sclk, optionalInt(card.sclk))
			mclk = append(mclk, optionalInt(card.mclk))
		}
		s.BusyPct = floats(minMax(busy, bucketOf, buckets))
		s.PowerW = floats(minMax(power, bucketOf, buckets))
		s.VRAMUsed = uints(minMax(vram, bucketOf, buckets))
		s.TokensPerS = floats(minMax(tokens, bucketOf, buckets))
		s.PromptTokensPerS = floats(minMax(prompt, bucketOf, buckets))
		s.TempC = temps(minMax(temp, bucketOf, buckets))
		s.SclkMHz = clocks(minMax(sclk, bucketOf, buckets))
		s.MclkMHz = clocks(minMax(mclk, bucketOf, buckets))
		doc.Series = append(doc.Series, s)
	}
	return doc
}

// points returns the points of the window, oldest first.
func (h *History) points(cut time.Time) []point {
	sel := make([]point, 0, h.n)
	for i := range h.n {
		p := h.pts[(h.head-h.n+i+historyCapacity)%historyCapacity]
		if p.at.Before(cut) {
			continue
		}
		sel = append(sel, p)
	}
	return sel
}

// bucketPlan splits the points of a window into the buckets of the
// downsampling. It returns the number of the buckets and the width of a
// bucket in seconds. A window that holds no more points than the cap is
// kept as it is: the width is 0 and there is no bucket.
func bucketPlan(sel []point) (int, int) {
	if len(sel) <= maxSeriesPoints {
		return len(sel), 0
	}
	span := int64(sel[len(sel)-1].at.Sub(sel[0].at) / time.Second)
	width := span/int64(maxBuckets) + 1
	for {
		count := int(span/width) + 1
		if count <= maxBuckets {
			return count, int(width)
		}
		width++
	}
}

// bucketIndexes returns the bucket of every point of a window. The buckets
// are counted from the first point of the window.
func bucketIndexes(sel []point, width int) []int {
	base := sel[0].at.Unix()
	out := make([]int, len(sel))
	for i, p := range sel {
		out[i] = int((p.at.Unix() - base) / int64(width))
	}
	return out
}

// sampleValue is one value of a series and whether it was measured.
type sampleValue struct {
	v  float64
	ok bool
}

// measured is a value that was measured.
func measured(v float64) sampleValue { return sampleValue{v: v, ok: true} }

// optional is a value that may be missing.
func optional(v *float64) sampleValue {
	if v == nil {
		return sampleValue{}
	}
	return measured(*v)
}

// optionalInt is an integer that may be missing.
func optionalInt(v *int) sampleValue {
	if v == nil {
		return sampleValue{}
	}
	return measured(float64(*v))
}

// minMax keeps the lowest and the highest measured value of every bucket,
// in the order of the window: the two values of a bucket are written in the
// order of the samples, so the line never steps back in time. A bucket
// without a measurement keeps two gaps, so a gap stays a gap. A window that
// is not downsampled keeps its values as they are.
func minMax(vals []sampleValue, bucketOf []int, buckets int) []sampleValue {
	if bucketOf == nil {
		out := make([]sampleValue, len(vals))
		copy(out, vals)
		return out
	}
	lo := make([]int, buckets)
	hi := make([]int, buckets)
	for i := range lo {
		lo[i] = -1
		hi[i] = -1
	}
	for i, v := range vals {
		if !v.ok {
			continue
		}
		b := bucketOf[i]
		if lo[b] < 0 || v.v < vals[lo[b]].v {
			lo[b] = i
		}
		if hi[b] < 0 || v.v > vals[hi[b]].v {
			hi[b] = i
		}
	}
	out := make([]sampleValue, 0, 2*buckets)
	for b := range buckets {
		switch {
		case lo[b] < 0:
			out = append(out, sampleValue{}, sampleValue{})
		case lo[b] == hi[b]:
			out = append(out, vals[lo[b]], vals[lo[b]])
		case lo[b] < hi[b]:
			out = append(out, vals[lo[b]], vals[hi[b]])
		default:
			out = append(out, vals[hi[b]], vals[lo[b]])
		}
	}
	return out
}

// floats writes the values of a series that are always measured.
func floats(vals []sampleValue) []float64 {
	out := make([]float64, len(vals))
	for i, v := range vals {
		out[i] = v.v
	}
	return out
}

// uints writes the values of a series that are always measured.
func uints(vals []sampleValue) []uint64 {
	out := make([]uint64, len(vals))
	for i, v := range vals {
		out[i] = uint64(v.v)
	}
	return out
}

// temps writes the values of a series that may be missing.
func temps(vals []sampleValue) []*float64 {
	out := make([]*float64, len(vals))
	for i, v := range vals {
		if !v.ok {
			continue
		}
		f := v.v
		out[i] = &f
	}
	return out
}

// clocks writes the integers of a series that may be missing.
func clocks(vals []sampleValue) []*int {
	out := make([]*int, len(vals))
	for i, v := range vals {
		if !v.ok {
			continue
		}
		n := int(v.v)
		out[i] = &n
	}
	return out
}

// hips returns the indexes of the cards of the points, in order.
func (h *History) hips(sel []point) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, p := range sel {
		for _, c := range p.cards {
			if seen[c.hip] {
				continue
			}
			seen[c.hip] = true
			out = append(out, c.hip)
		}
	}
	sort.Ints(out)
	return out
}

// round1 rounds a value to one decimal: the history holds hundreds of
// points and the reader does not need more.
func round1(v float64) float64 {
	return float64(int64(v*10+0.5)) / 10
}
