package flow

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// fakeRunner starts a run without a process. The run is "starting" until the
// second poll and "running" after it.
type fakeRunner struct {
	events *[]string
	calls  int
}

func (f *fakeRunner) Start(ctx context.Context, p *recipe.Plan, o runner.StartOptions) (*runner.Record, error) {
	*f.events = append(*f.events, "start")
	return &runner.Record{Name: "test", Port: 18000, PID: 42}, nil
}

func (f *fakeRunner) List(ctx context.Context) ([]runner.Status, error) {
	f.calls++
	switch f.calls {
	case 1:
		return nil, nil
	case 2:
		*f.events = append(*f.events, "starting")
		return []runner.Status{{Record: runner.Record{Name: "test", Port: 18000}, State: runner.StateStarting}}, nil
	default:
		*f.events = append(*f.events, "healthy")
		return []runner.Status{{Record: runner.Record{Name: "test", Port: 18000}, State: runner.StateRunning}}, nil
	}
}

// fakeDashboard is the dashboard of `up`: it records when it starts and when
// it is ready (the Ready callback of the real one).
type fakeDashboard struct {
	events *[]string
	err    error
}

func (f *fakeDashboard) Start(ctx context.Context) error {
	*f.events = append(*f.events, "dashboard")
	if f.err != nil {
		return f.err
	}
	*f.events = append(*f.events, "ready")
	return nil
}

// fastPoll makes the wait loop check the run state every 10 ms.
func fastPoll(t *testing.T) {
	t.Helper()
	old := healthPoll
	healthPoll = 10 * time.Millisecond
	t.Cleanup(func() { healthPoll = old })
}

// The dashboard is ready before the server answers /health: the operator
// watches the model load in the browser.
func TestServeAndWaitStartsTheDashboardBeforeHealth(t *testing.T) {
	fastPoll(t)
	var events []string
	o := UpOptions{
		Wait:      5 * time.Second,
		Runner:    &fakeRunner{events: &events},
		Prepared:  &recipe.Plan{Name: "test"},
		Dashboard: &fakeDashboard{events: &events},
	}
	res := &UpResult{Recipe: "test", Mode: "host"}
	if err := serveAndWait(context.Background(), o, &bytes.Buffer{}, res); err != nil {
		t.Fatalf("serveAndWait: %v", err)
	}
	if !res.Healthy {
		t.Error("the run must be healthy")
	}
	ready, healthy := index(events, "ready"), index(events, "healthy")
	if ready < 0 || healthy < 0 {
		t.Fatalf("events = %v", events)
	}
	if ready > healthy {
		t.Errorf("the dashboard must be ready before the server is healthy: %v", events)
	}
	if events[0] != "start" {
		t.Errorf("the run must start first: %v", events)
	}
}

// A dashboard that cannot bind its address is reported; the run goes on and
// the wait for /health is not cut short.
func TestServeAndWaitKeepsWaitingWhenTheDashboardCannotBind(t *testing.T) {
	fastPoll(t)
	var events []string
	var log bytes.Buffer
	o := UpOptions{
		Wait:     5 * time.Second,
		Runner:   &fakeRunner{events: &events},
		Prepared: &recipe.Plan{Name: "test"},
		Dashboard: &fakeDashboard{events: &events,
			err: errors.New("listen tcp 127.0.0.1:7860: bind: address already in use")},
	}
	res := &UpResult{Recipe: "test", Mode: "host"}
	if err := serveAndWait(context.Background(), o, &log, res); err != nil {
		t.Fatalf("the run must go on: %v", err)
	}
	if !res.Healthy {
		t.Error("the run must be healthy")
	}
	if !strings.Contains(log.String(), "address already in use") {
		t.Errorf("the bind error must be reported: %q", log.String())
	}
	if index(events, "healthy") < 0 {
		t.Errorf("the wait must go on: %v", events)
	}
}

// Without a dashboard the run waits for /health as before.
func TestServeAndWaitWithoutADashboard(t *testing.T) {
	fastPoll(t)
	var events []string
	o := UpOptions{Wait: 5 * time.Second, Runner: &fakeRunner{events: &events}, Prepared: &recipe.Plan{Name: "test"}}
	res := &UpResult{Recipe: "test", Mode: "host"}
	if err := serveAndWait(context.Background(), o, &bytes.Buffer{}, res); err != nil {
		t.Fatalf("serveAndWait: %v", err)
	}
	if index(events, "healthy") < 0 {
		t.Errorf("events = %v", events)
	}
	for _, e := range events {
		if e == "dashboard" {
			t.Errorf("no dashboard must start: %v", events)
		}
	}
}

// index gives the place of an event, or -1.
func index(events []string, want string) int {
	for i, e := range events {
		if e == want {
			return i
		}
	}
	return -1
}
