package vllm

import (
	"context"
	"time"
)

// DefaultWindow is the length of the window a watcher reports: the last
// minute of the server.
const DefaultWindow = 60 * time.Second

// Reading is one reading of the metrics of a server: what it has done
// since it started and what it did since the reading that started the
// window.
type Reading struct {
	// Stats are the numbers of the server since it started.
	Stats Stats `json:"stats"`
	// Window is what the server did in the last window.
	Window Window `json:"window"`
}

// Watcher reads the metrics of one server and keeps the readings of the
// last window, so it can say what the server did in that time. A watcher
// belongs to one goroutine.
type Watcher struct {
	client *Client
	window time.Duration
	// points are the readings of the window, oldest first.
	points []point
}

// point is one reading of the metrics with the time it was taken.
type point struct {
	at time.Time
	checkpoint
}

// NewWatcher returns a watcher of the server at baseURL that reports the
// last window of the given length. A window of zero reports the two last
// readings.
func NewWatcher(baseURL string, window time.Duration) *Watcher {
	return &Watcher{client: New(baseURL), window: window}
}

// Sample reads the metrics of the server and returns what it holds now.
// The window of the reading holds what the server did since the reading
// that started the window; it is empty when the server has no such reading
// yet.
func (w *Watcher) Sample(ctx context.Context) (Reading, error) {
	return w.sample(ctx, time.Now())
}

// sample reads the metrics of the server and files the reading under the
// given time.
func (w *Watcher) sample(ctx context.Context, at time.Time) (Reading, error) {
	m, err := w.client.Metrics(ctx)
	if err != nil {
		return Reading{}, err
	}
	w.points = append(w.points, point{at: at, checkpoint: checkpointOf(m)})
	// Drop the readings that left the window; the oldest one left is the
	// start of the window. The two last readings always stay: a window
	// needs two readings to hold anything.
	cut := at.Add(-w.window)
	f := 0
	for f < len(w.points)-2 && w.points[f].at.Before(cut) {
		f++
	}
	w.points = w.points[f:]
	r := Reading{Stats: Summarize(m)}
	if len(w.points) >= 2 {
		first, last := w.points[0], w.points[len(w.points)-1]
		r.Window = windowBetween(first.checkpoint, last.checkpoint, last.at.Sub(first.at).Seconds())
	}
	return r, nil
}
