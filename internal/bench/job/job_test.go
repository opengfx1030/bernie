package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// waitFor waits until the condition holds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for range 400 {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the %s did not happen", what)
}

// TestJobStateMachine checks that the typed events move the state of a job:
// the counters, the phase and the last event.
func TestJobStateMachine(t *testing.T) {
	s := NewStore()
	j, err := s.Start(context.Background(), Request{Kind: KindRun, Total: 2,
		Run: func(ctx context.Context, p *Progress) (string, error) {
			p.Event(bench.Event{Kind: bench.EventCell, Index: 1, Total: 2, Level: 4})
			_, _ = p.Write([]byte("bench: c=4 done in 1.0s: 4 ok, 0 failed\n"))
			p.Event(bench.Event{Kind: bench.EventCell, Index: 2, Total: 2, Level: 8})
			return "r1", nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the job", func() bool { return j.Status() == StatusDone })
	doc := j.Doc()
	if doc.Done != 2 || doc.Total != 2 {
		t.Errorf("the job counts %d of %d, want 2 of 2", doc.Done, doc.Total)
	}
	if doc.ResultID != "r1" {
		t.Errorf("the result is %q, want r1", doc.ResultID)
	}
	if doc.Event.Kind != bench.EventCell || doc.Event.Level != 8 {
		t.Errorf("the last event is %+v", doc.Event)
	}
	if doc.Phase != "the run is done" {
		t.Errorf("the phase is %q", doc.Phase)
	}
	lines, _, _ := j.State()
	if len(lines) != 1 {
		t.Errorf("the job holds %d lines, want 1", len(lines))
	}
}

// TestJobPhaseFollowsTheEvents checks that the phase of a job comes from the
// typed events and not from the text of the log.
func TestJobPhaseFollowsTheEvents(t *testing.T) {
	s := NewStore()
	release := make(chan struct{})
	defer close(release)
	j, err := s.Start(context.Background(), Request{Kind: KindRun, Total: 3,
		Run: func(ctx context.Context, p *Progress) (string, error) {
			p.Event(bench.Event{Kind: bench.EventGroup, Group: "1k", Index: 1, Total: 1})
			p.Event(bench.Event{Kind: bench.EventCell, Index: 1, Total: 3, Level: 4})
			_, _ = p.Write([]byte("bench: visit 9 of 9: nonsense\n"))
			<-release
			return "", nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the cell", func() bool { return j.Doc().Done == 1 })
	doc := j.Doc()
	if doc.Phase != "cell 2 of 3: 1k at concurrency 4" {
		t.Errorf("the phase is %q", doc.Phase)
	}
	if doc.Event.Kind != bench.EventCell {
		t.Errorf("the last event is %+v", doc.Event)
	}
}

// TestVisitEventNamesTheArm checks the phase of an A/B job.
func TestVisitEventNamesTheArm(t *testing.T) {
	s := NewStore()
	release := make(chan struct{})
	defer close(release)
	j, err := s.Start(context.Background(), Request{Kind: KindAB,
		Run: func(ctx context.Context, p *Progress) (string, error) {
			p.Event(bench.Event{Kind: bench.EventVisit, Arm: "base", Index: 1, Total: 4})
			<-release
			return "", nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the visit", func() bool { return j.Doc().Phase != "" })
	doc := j.Doc()
	if doc.Phase != "visit 1 of 4: base" {
		t.Errorf("the phase is %q", doc.Phase)
	}
	if doc.Total != 4 {
		t.Errorf("the job counts %d visits, want 4", doc.Total)
	}
}

// TestStoreAnswersBusy checks that a second job gets the busy answer.
func TestStoreAnswersBusy(t *testing.T) {
	s := NewStore()
	release := make(chan struct{})
	defer close(release)
	_, err := s.Start(context.Background(), Request{Kind: KindRun,
		Run: func(ctx context.Context, p *Progress) (string, error) {
			<-release
			return "", nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Start(context.Background(), Request{Kind: KindRun})
	if !errors.Is(err, app.ErrBenchBusy) {
		t.Fatalf("a second job: %v", err)
	}
}

// TestStoreKeepsTheFinishedJobsBounded checks that the store keeps at most
// MaxFinished finished jobs and drops the oldest one.
func TestStoreKeepsTheFinishedJobsBounded(t *testing.T) {
	s := NewStore()
	for range MaxFinished + 3 {
		j, err := s.Start(context.Background(), Request{Kind: KindRun,
			Run: func(ctx context.Context, p *Progress) (string, error) {
				return "", nil
			}})
		if err != nil {
			t.Fatal(err)
		}
		waitFor(t, "the job", func() bool { return j.Status() == StatusDone })
	}
	waitFor(t, "the eviction", func() bool { return len(s.List()) == MaxFinished })
	jobs := s.List()
	if len(jobs) != MaxFinished {
		t.Fatalf("the store holds %d jobs, want %d", len(jobs), MaxFinished)
	}
	for _, j := range jobs {
		if j.Status() == StatusRunning {
			t.Errorf("the job %s runs", j.ID())
		}
	}
}

// TestStoreForgetsTheOldestJob checks that the job that fell out of the
// bound is gone.
func TestStoreForgetsTheOldestJob(t *testing.T) {
	s := NewStore()
	var first *Job
	for i := range MaxFinished + 1 {
		j, err := s.Start(context.Background(), Request{Kind: KindRun,
			Run: func(ctx context.Context, p *Progress) (string, error) {
				return "", nil
			}})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = j
		}
		waitFor(t, "the job", func() bool { return j.Status() == StatusDone })
	}
	waitFor(t, "the eviction", func() bool { return len(s.List()) == MaxFinished })
	if _, ok := s.Get(first.ID()); ok {
		t.Errorf("the store still holds the job %s", first.ID())
	}
}
