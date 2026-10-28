package dashboard

import (
	"context"
	"errors"
)

// Starter starts a dashboard in the background of a command: `up` starts it
// while the server loads the model, `serve --dash` starts it next to the run
// (spec §3.1). The dashboard is not listening until Start is called.
type Starter struct {
	o Options
	h *Handle
}

// NewStarter returns a starter for a dashboard with these options.
func NewStarter(o Options) *Starter { return &Starter{o: o} }

// Start starts the dashboard and returns when it listens. A bind error (the
// port is in use) comes back as an error; no dashboard runs then.
func (s *Starter) Start(ctx context.Context) error {
	h, err := Start(ctx, s.o)
	if err != nil {
		return err
	}
	s.h = h
	return nil
}

// Wait blocks until the dashboard stops (Ctrl-C) and returns its error. It
// returns nil when no dashboard runs.
func (s *Starter) Wait() error {
	if s.h == nil {
		return nil
	}
	return s.h.Wait()
}

// Handle is a dashboard that serves in the background.
type Handle struct {
	url  string
	done chan error
}

// Start serves the dashboard in the background and returns when it listens.
// The URL to open goes to Options.Ready (the caller prints it). A bind error
// comes back as an error and no dashboard runs then.
func Start(ctx context.Context, o Options) (*Handle, error) {
	h := &Handle{done: make(chan error, 1)}
	ready := make(chan string, 1)
	caller := o.Ready
	o.Ready = func(url string) {
		ready <- url
		if caller != nil {
			caller(url)
		}
	}
	go func() { h.done <- Serve(ctx, o) }()
	select {
	case url := <-ready:
		h.url = url
		return h, nil
	case err := <-h.done:
		select {
		case url := <-ready: // the server stopped right after it listened
			h.url = url
			return h, nil
		default:
		}
		if err == nil {
			err = errors.New("the dashboard stopped before it was ready")
		}
		return nil, err
	}
}

// URL is the address to open (with the token in token mode).
func (h *Handle) URL() string { return h.url }

// Wait blocks until the dashboard stops and returns its error. A stop by
// Ctrl-C is not an error.
func (h *Handle) Wait() error { return <-h.done }
