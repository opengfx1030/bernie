package telemetry

import (
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// fill adds n points, one every 2 s, ending now.
func fill(h *History, n int, now time.Time) {
	for i := range n {
		at := now.Add(-time.Duration(n-i) * historyStep)
		h.Add(at, []gpu.Sample{{HIPIndex: 0, BusyPct: float64(i), PowerW: 10.5, VRAMUsed: 1024}}, float64(i), float64(i), false)
	}
}

// TestHistoryKeepsOnePointPerStep checks the spacing of the ring buffer: a
// sample that comes too soon after the last one is dropped.
func TestHistoryKeepsOnePointPerStep(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	h.Add(now, []gpu.Sample{{HIPIndex: 0}}, 1, 1, false)
	h.Add(now.Add(time.Second), []gpu.Sample{{HIPIndex: 0}}, 2, 2, false)
	if h.n != 1 {
		t.Fatalf("the buffer holds %d points, want 1", h.n)
	}
	h.Add(now.Add(2*time.Second), []gpu.Sample{{HIPIndex: 0}}, 3, 3, false)
	if h.n != 2 {
		t.Fatalf("the buffer holds %d points, want 2", h.n)
	}
}

// TestHistoryWindowShape checks the shape of the answer: one series per
// card, one value per point, and the spacing of the window.
func TestHistoryWindowShape(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	for i := range 3 {
		at := now.Add(-time.Duration(3-i) * historyStep)
		h.Add(at, []gpu.Sample{
			{HIPIndex: 0, BusyPct: 10, PowerW: 20.25, VRAMUsed: 100},
			{HIPIndex: 1, BusyPct: 30, PowerW: 40.5, VRAMUsed: 200},
		}, 5, 3, false)
	}
	doc, err := h.Window(Window5m)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Window != Window5m || doc.IntervalS != 2 || doc.Points != 3 {
		t.Fatalf("the document is %+v", doc)
	}
	if len(doc.Series) != 2 {
		t.Fatalf("the document holds %d series, want 2", len(doc.Series))
	}
	s := doc.Series[0]
	if s.HIPIndex != 0 {
		t.Fatalf("the first series is the card %d", s.HIPIndex)
	}
	if len(s.BusyPct) != 3 || len(s.PowerW) != 3 || len(s.VRAMUsed) != 3 || len(s.TokensPerS) != 3 || len(s.PromptTokensPerS) != 3 {
		t.Fatalf("the series holds %d values, want 3", len(s.BusyPct))
	}
	if s.BusyPct[0] != 10 || s.PowerW[0] != 20.3 || s.VRAMUsed[0] != 100 {
		t.Fatalf("the series is %+v", s)
	}
	if s.TokensPerS[2] != 5 {
		t.Fatalf("the token rate is %v", s.TokensPerS)
	}
	if s.PromptTokensPerS[2] != 3 {
		t.Fatalf("the prefill rate is %v", s.PromptTokensPerS)
	}
	if doc.Series[1].HIPIndex != 1 {
		t.Fatalf("the second series is the card %d", doc.Series[1].HIPIndex)
	}
}

// TestHistoryLongWindowIsDownsampled checks that a window that holds more
// points than the cap is downsampled into buckets.
func TestHistoryLongWindowIsDownsampled(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	for i := range historyCapacity { // 30 minutes at one point every 2 s
		at := now.Add(-time.Duration(historyCapacity-i) * historyStep)
		h.Add(at, []gpu.Sample{{HIPIndex: 0}}, float64(i), float64(i), false)
	}
	doc, err := h.Window(Window1h)
	if err != nil {
		t.Fatal(err)
	}
	if doc.IntervalS != 15 {
		t.Fatalf("the 1 h window has the spacing %d s, want 15", doc.IntervalS)
	}
	if doc.Points != maxSeriesPoints {
		t.Fatalf("the 1 h window holds %d points, want %d", doc.Points, maxSeriesPoints)
	}
	if len(doc.Series[0].TokensPerS) != doc.Points {
		t.Fatalf("the series holds %d points, want %d", len(doc.Series[0].TokensPerS), doc.Points)
	}
	// The highest value of the window survives the downsampling.
	top := 0.0
	for _, v := range doc.Series[0].TokensPerS {
		if v > top {
			top = v
		}
	}
	if top != float64(historyCapacity-1) {
		t.Fatalf("the highest token rate is %v, want %d", top, historyCapacity-1)
	}
}

// TestHistoryKeepsThePeakOfOneSample checks the downsampling: a peak that
// lasted one sample is still in the answer. The old rule kept the last
// point of every bucket and lost it.
func TestHistoryKeepsThePeakOfOneSample(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	for i := range historyCapacity { // 30 minutes at one point every 2 s
		at := now.Add(-time.Duration(historyCapacity-i) * historyStep)
		busy := 10.0
		if i == historyCapacity/2 {
			busy = 100 // one sample of a peak
		}
		h.Add(at, []gpu.Sample{{HIPIndex: 0, BusyPct: busy}}, 0, 0, false)
	}
	doc, err := h.Window(Window1h)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Points > maxSeriesPoints {
		t.Fatalf("the window holds %d points, want at most %d", doc.Points, maxSeriesPoints)
	}
	if len(doc.Series) != 1 {
		t.Fatalf("the window holds %d series, want 1", len(doc.Series))
	}
	s := doc.Series[0]
	if len(s.BusyPct) != doc.Points {
		t.Fatalf("the series holds %d points, want %d", len(s.BusyPct), doc.Points)
	}
	top := 0.0
	for _, v := range s.BusyPct {
		if v > top {
			top = v
		}
	}
	if top != 100 {
		t.Fatalf("the highest load of the answer is %v, want 100: the peak was lost", top)
	}
}

// TestHistoryKeepsTheGapOfTheSlowFields checks that a point without a slow
// measurement stays a gap after the downsampling.
func TestHistoryKeepsTheGapOfTheSlowFields(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	for i := range historyCapacity {
		at := now.Add(-time.Duration(historyCapacity-i) * historyStep)
		h.Add(at, []gpu.Sample{{HIPIndex: 0}}, 0, 0, false)
	}
	doc, err := h.Window(Window1h)
	if err != nil {
		t.Fatal(err)
	}
	s := doc.Series[0]
	if len(s.TempC) != doc.Points {
		t.Fatalf("the slow series holds %d points, want %d", len(s.TempC), doc.Points)
	}
	for i, v := range s.TempC {
		if v != nil {
			t.Fatalf("the point %d holds the temperature %v, want a gap", i, *v)
		}
	}
}

// TestHistoryKeepsTheLastThirtyMinutes checks the retention: the buffer
// holds 900 points and drops the oldest ones.
func TestHistoryKeepsTheLastThirtyMinutes(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	for i := range historyCapacity + 50 {
		at := now.Add(-time.Duration(historyCapacity+50-i) * historyStep)
		h.Add(at, []gpu.Sample{{HIPIndex: 0}}, 0, 0, false)
	}
	if h.n != historyCapacity {
		t.Fatalf("the buffer holds %d points, want %d", h.n, historyCapacity)
	}
	doc, err := h.Window(Window1h)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Points > maxSeriesPoints {
		t.Fatalf("the 1 h window holds %d points, want at most %d", doc.Points, maxSeriesPoints)
	}
	if doc.Points == 0 {
		t.Fatal("the window is empty")
	}
}

// TestHistoryRejectsAnUnknownWindow checks that only the three windows of
// the design are answered.
func TestHistoryRejectsAnUnknownWindow(t *testing.T) {
	h := NewHistory()
	for _, name := range []string{"", "2m", "1d", "15"} {
		if _, err := h.Window(name); err == nil {
			t.Errorf("the window %q was accepted", name)
		}
	}
	for _, name := range []string{Window5m, Window15m, Window1h} {
		if _, err := h.Window(name); err != nil {
			t.Errorf("the window %q was refused: %v", name, err)
		}
	}
}

// TestHistoryCapsThePointsOfASeries checks that no window holds more than
// maxSeriesPoints points per series: a long window is downsampled further.
func TestHistoryCapsThePointsOfASeries(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	for i := range historyCapacity { // 30 minutes at one point every 2 s
		at := now.Add(-time.Duration(historyCapacity-i) * historyStep)
		h.Add(at, []gpu.Sample{{HIPIndex: 0}}, 0, 0, false)
	}
	for _, name := range []string{Window5m, Window15m, Window1h} {
		doc, err := h.Window(name)
		if err != nil {
			t.Fatal(err)
		}
		if doc.Points > maxSeriesPoints {
			t.Errorf("the window %s holds %d points, want at most %d", name, doc.Points, maxSeriesPoints)
		}
		if len(doc.Series) != 1 || len(doc.Series[0].BusyPct) > maxSeriesPoints {
			t.Errorf("the series of the window %s holds %d points", name, len(doc.Series[0].BusyPct))
		}
	}
}

// TestHistoryCachesTheAnswer checks that the same window answers the same
// document for two seconds.
func TestHistoryCachesTheAnswer(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	h.Add(now, []gpu.Sample{{HIPIndex: 0, BusyPct: 5}}, 0, 0, false)
	first, err := h.Window(Window5m)
	if err != nil {
		t.Fatal(err)
	}
	h.Add(now.Add(historyStep), []gpu.Sample{{HIPIndex: 0, BusyPct: 6}}, 0, 0, false)
	second, err := h.Window(Window5m)
	if err != nil {
		t.Fatal(err)
	}
	if second.Points != first.Points {
		t.Fatalf("the cached answer holds %d points, want %d", second.Points, first.Points)
	}
}
