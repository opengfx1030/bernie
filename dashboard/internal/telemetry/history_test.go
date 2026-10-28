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
		h.Add(at, []gpu.Sample{{HIPIndex: 0, BusyPct: float64(i), PowerW: 10.5, VRAMUsed: 1024}}, float64(i))
	}
}

// TestHistoryKeepsOnePointPerStep checks the spacing of the ring buffer: a
// sample that comes too soon after the last one is dropped.
func TestHistoryKeepsOnePointPerStep(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	h.Add(now, []gpu.Sample{{HIPIndex: 0}}, 1)
	h.Add(now.Add(time.Second), []gpu.Sample{{HIPIndex: 0}}, 2)
	if h.n != 1 {
		t.Fatalf("the buffer holds %d points, want 1", h.n)
	}
	h.Add(now.Add(2*time.Second), []gpu.Sample{{HIPIndex: 0}}, 3)
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
		}, 5)
	}
	doc, err := h.Window(Window15m)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Window != Window15m || doc.IntervalS != 2 || doc.Points != 3 {
		t.Fatalf("the document is %+v", doc)
	}
	if len(doc.Series) != 2 {
		t.Fatalf("the document holds %d series, want 2", len(doc.Series))
	}
	s := doc.Series[0]
	if s.HIPIndex != 0 {
		t.Fatalf("the first series is the card %d", s.HIPIndex)
	}
	if len(s.BusyPct) != 3 || len(s.PowerW) != 3 || len(s.VRAMUsed) != 3 || len(s.TokensPerS) != 3 {
		t.Fatalf("the series holds %d values, want 3", len(s.BusyPct))
	}
	if s.BusyPct[0] != 10 || s.PowerW[0] != 20.3 || s.VRAMUsed[0] != 100 {
		t.Fatalf("the series is %+v", s)
	}
	if s.TokensPerS[2] != 5 {
		t.Fatalf("the token rate is %v", s.TokensPerS)
	}
	if doc.Series[1].HIPIndex != 1 {
		t.Fatalf("the second series is the card %d", doc.Series[1].HIPIndex)
	}
}

// TestHistoryLongWindowIsDownsampled checks that the 1 h window holds one
// point every 10 s.
func TestHistoryLongWindowIsDownsampled(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	for i := range 30 { // 30 points every 2 s = 60 s
		at := now.Add(-time.Duration(30-i) * historyStep)
		h.Add(at, []gpu.Sample{{HIPIndex: 0}}, float64(i))
	}
	doc, err := h.Window(Window1h)
	if err != nil {
		t.Fatal(err)
	}
	if doc.IntervalS != 10 {
		t.Fatalf("the 1 h window has the spacing %d s, want 10", doc.IntervalS)
	}
	if doc.Points < 6 || doc.Points > 7 { // 60 s at one point every 10 s
		t.Fatalf("the 1 h window holds %d points, want 6 or 7", doc.Points)
	}
	// The last point of every bucket wins.
	if got := doc.Series[0].TokensPerS[len(doc.Series[0].TokensPerS)-1]; got != 29 {
		t.Fatalf("the last token rate is %v, want 29", got)
	}
}

// TestHistoryKeepsTheLastThirtyMinutes checks the retention: the buffer
// holds 900 points and drops the oldest ones.
func TestHistoryKeepsTheLastThirtyMinutes(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	for i := range historyCapacity + 50 {
		at := now.Add(-time.Duration(historyCapacity+50-i) * historyStep)
		h.Add(at, []gpu.Sample{{HIPIndex: 0}}, 0)
	}
	doc, err := h.Window(Window1h)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Points > 181 {
		t.Fatalf("the 1 h window holds %d points, want at most 181", doc.Points)
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

// TestHistoryCachesTheAnswer checks that the same window answers the same
// document for two seconds.
func TestHistoryCachesTheAnswer(t *testing.T) {
	h := NewHistory()
	now := time.Now()
	h.Add(now, []gpu.Sample{{HIPIndex: 0, BusyPct: 5}}, 0)
	first, err := h.Window(Window5m)
	if err != nil {
		t.Fatal(err)
	}
	h.Add(now.Add(historyStep), []gpu.Sample{{HIPIndex: 0, BusyPct: 6}}, 0)
	second, err := h.Window(Window5m)
	if err != nil {
		t.Fatal(err)
	}
	if second.Points != first.Points {
		t.Fatalf("the cached answer holds %d points, want %d", second.Points, first.Points)
	}
}
