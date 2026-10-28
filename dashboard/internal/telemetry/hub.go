// Package telemetry samples the live host once and shares it with every
// dashboard viewer.
package telemetry

import (
	"context"
	"sync"
	"time"
)

// Hub runs one sampler goroutine for all telemetry subscribers. The sampler
// starts with the first subscriber and stops when the last one leaves, so an
// idle dashboard reads no sysfs and polls no vLLM server.
type Hub struct {
	interval time.Duration
	sample   func(ctx context.Context) []byte // one JSON document

	mu     sync.Mutex
	subs   map[chan []byte]struct{}
	last   []byte
	cancel context.CancelFunc
}

// NewHub returns a shared telemetry hub.
func NewHub(interval time.Duration, sample func(context.Context) []byte) *Hub {
	return &Hub{interval: interval, sample: sample, subs: map[chan []byte]struct{}{}}
}

// Subscribe returns a channel of samples and a function that unsubscribes.
// The channel holds one value; a slow reader skips samples, it never blocks
// the sampler.
func (h *Hub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if h.last != nil {
		ch <- h.last
	}
	if h.cancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		h.cancel = cancel
		go h.run(ctx)
	}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; !ok {
			return
		}
		delete(h.subs, ch)
		if len(h.subs) == 0 && h.cancel != nil {
			h.cancel()
			h.cancel, h.last = nil, nil
		}
	}
}

// Running reports whether the sampler goroutine is active.
func (h *Hub) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cancel != nil
}

func (h *Hub) run(ctx context.Context) {
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		b := h.sample(ctx)
		if ctx.Err() != nil {
			return
		}
		h.mu.Lock()
		if ctx.Err() == nil { // not cancelled while sampling
			h.last = b
			for ch := range h.subs {
				select {
				case <-ch: // drop the stale sample
				default:
				}
				ch <- b
			}
		}
		h.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
