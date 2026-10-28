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
	historyCapacity = 900              // 30 minutes at one point every 2 s
	historyStep     = 2 * time.Second  // the spacing of the points
	historyLongStep = 10 * time.Second // the spacing of the 1 h window
	historyCache    = 2 * time.Second  // how long an answer is reused
	// maxSeriesPoints is the number of points a series of the history
	// holds. A longer window is downsampled further, so the answer stays
	// small enough to paint and to download.
	maxSeriesPoints = 240
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
	at    time.Time
	cards []cardPoint
	tokS  float64
}

// cardPoint is what the history keeps of one card.
type cardPoint struct {
	hip   int
	busy  float64
	power float64
	vram  uint64
}

// HistorySeries is the history of one card: one value per point.
type HistorySeries struct {
	HIPIndex   int       `json:"hip_index"`
	BusyPct    []float64 `json:"busy_pct"`
	PowerW     []float64 `json:"power_w"`
	VRAMUsed   []uint64  `json:"vram_used"`
	TokensPerS []float64 `json:"tokens_per_s"`
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
// run: the rate is a property of the server, so every card carries it.
func (h *History) Add(at time.Time, gpus []gpu.Sample, tokS float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.last.IsZero() && at.Sub(h.last) < historyStep {
		return
	}
	h.last = at
	cards := make([]cardPoint, 0, len(gpus))
	for _, g := range gpus {
		cards = append(cards, cardPoint{
			hip:   g.HIPIndex,
			busy:  round1(g.BusyPct),
			power: round1(g.PowerW),
			vram:  g.VRAMUsed,
		})
	}
	h.pts[h.head] = point{at: at, cards: cards, tokS: round1(tokS)}
	h.head = (h.head + 1) % historyCapacity
	if h.n < historyCapacity {
		h.n++
	}
}

// Window returns the document of a window: "5m", "15m" or "1h". The answer
// is reused for historyCache seconds, so a page that asks often does not
// rebuild it. The 1 h window holds one point every 10 s; the buffer keeps
// 30 minutes, so it holds about 180 points.
func (h *History) Window(name string) (HistoryDocument, error) {
	dur, step, err := windowSpec(name)
	if err != nil {
		return HistoryDocument{}, err
	}
	// No series holds more than maxSeriesPoints points: a long window is
	// downsampled further.
	if least := dur / maxSeriesPoints; step < least {
		step = (least + time.Second - 1) / time.Second * time.Second
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	if h.cached != nil && h.cached.Window == name && now.Sub(h.cachedAt) < historyCache {
		return *h.cached, nil
	}
	doc := h.build(name, dur, step, now)
	h.cached, h.cachedAt = &doc, now
	return doc, nil
}

// windowSpec returns the length and the spacing of a window.
func windowSpec(name string) (time.Duration, time.Duration, error) {
	switch name {
	case Window5m:
		return 5 * time.Minute, historyStep, nil
	case Window15m:
		return 15 * time.Minute, historyStep, nil
	case Window1h:
		return time.Hour, historyLongStep, nil
	}
	return 0, 0, fmt.Errorf("window %q is not one of %s, %s or %s", name, Window5m, Window15m, Window1h)
}

// build assembles the document of a window. The caller holds the lock.
func (h *History) build(name string, dur, step time.Duration, now time.Time) HistoryDocument {
	sel := h.points(now.Add(-dur), step)
	hips := h.hips(sel)
	doc := HistoryDocument{
		Window:    name,
		IntervalS: int(step / time.Second),
		Points:    len(sel),
		Series:    make([]HistorySeries, 0, len(hips)),
	}
	for _, hip := range hips {
		s := HistorySeries{
			HIPIndex:   hip,
			BusyPct:    make([]float64, 0, len(sel)),
			PowerW:     make([]float64, 0, len(sel)),
			VRAMUsed:   make([]uint64, 0, len(sel)),
			TokensPerS: make([]float64, 0, len(sel)),
		}
		var busy, power float64
		var vram uint64
		for _, p := range sel {
			for _, c := range p.cards {
				if c.hip != hip {
					continue
				}
				busy, power, vram = c.busy, c.power, c.vram
				break
			}
			// A card that is gone keeps its last value: the chart shows no
			// dip that never happened.
			s.BusyPct = append(s.BusyPct, busy)
			s.PowerW = append(s.PowerW, power)
			s.VRAMUsed = append(s.VRAMUsed, vram)
			s.TokensPerS = append(s.TokensPerS, p.tokS)
		}
		doc.Series = append(doc.Series, s)
	}
	return doc
}

// points returns the points of the window, oldest first. When the step is
// longer than the spacing of the buffer, one point per bucket is kept: the
// last of every bucket.
func (h *History) points(cut time.Time, step time.Duration) []point {
	sel := make([]point, 0, h.n)
	for i := range h.n {
		p := h.pts[(h.head-h.n+i+historyCapacity)%historyCapacity]
		if p.at.Before(cut) {
			continue
		}
		sel = append(sel, p)
	}
	if step <= historyStep {
		return sel
	}
	out := make([]point, 0, len(sel))
	width := int64(step / time.Second)
	for i := range sel {
		if i+1 < len(sel) && sel[i+1].at.Unix()/width == sel[i].at.Unix()/width {
			continue // a later point of the same bucket wins
		}
		out = append(out, sel[i])
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
