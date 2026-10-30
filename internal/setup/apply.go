package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Apply runs, in order, the steps that are not done and do not need root,
// streaming their output to log. Root steps are printed, not run. only
// limits the run to the named step ids. It stops at the first failure.
func Apply(ctx context.Context, steps []Step, only []string, log io.Writer) error {
	_, err := ApplyRun(ctx, steps, only, log, ApplyOptions{Mirror: true})
	return err
}

// ApplyOptions say how a run is recorded and how its output is streamed.
type ApplyOptions struct {
	// Mode, Recipe and Venv describe the run.
	Mode, Recipe, Venv string
	// Store keeps the record of the run. A nil store keeps no record:
	// the steps run as they always did.
	Store *RunStore
	// Mirror writes the raw output of the steps to log, next to the log
	// of the step. The dashboard and `up` show the output; the CLI shows
	// the progress and keeps the output in the log of the step.
	Mirror bool
	// Quiet leaves the markers ("== step: title") out of log.
	Quiet bool
	// Event receives the events of the run. The CLI streams them.
	Event func(Event)
	// Clock is the clock of the run. The tests replace it.
	Clock func() time.Time
}

// Event is one thing that happened in a run. The CLI streams the events
// as NDJSON, the record of the run is the last one.
type Event struct {
	// Type is the kind of event: run_started, step_started, progress,
	// step_done or run_done.
	Type  string    `json:"type"`
	RunID string    `json:"run_id"`
	At    time.Time `json:"at"`
	// Index and Total place the step in the run: "[3/7]".
	Index int      `json:"index,omitempty"`
	Total int      `json:"total,omitempty"`
	Step  *RunStep `json:"step,omitempty"`
	Run   *Run     `json:"run,omitempty"`
	Error string   `json:"error,omitempty"`
}

// ApplyRun runs the steps of the plan like Apply and records the run: one
// record per run, one log per step, the state, the duration and the
// evidence of every step. The record is written after every step and
// every two seconds while a step runs, so a crash or a reload still shows
// the truth.
func ApplyRun(ctx context.Context, steps []Step, only []string, log io.Writer, o ApplyOptions) (*Run, error) {
	sel, err := selectSteps(steps, only)
	if err != nil {
		return nil, err
	}
	a := &applier{opt: o, log: log}
	a.run = &Run{ID: NewRunID(), Mode: o.Mode, Recipe: o.Recipe, Venv: o.Venv, State: RunRunning, StartedAt: a.now()}
	if o.Store != nil {
		if err := o.Store.Create(a.run); err != nil {
			return nil, err
		}
	}
	a.emit(Event{Type: "run_started", RunID: a.run.ID, At: a.run.StartedAt})
	var rootSteps []Step
	var runErr error
	total := runnableCount(steps, sel)
	index := 0
loop:
	for _, s := range steps {
		if !sel[s.ID] {
			continue
		}
		switch {
		case s.Done:
			a.skip(s, "already done")
		case s.Root:
			a.block(s, "needs root: run the commands yourself")
			rootSteps = append(rootSteps, s)
		default:
			index++
			if err := a.step(ctx, s, index, total); err != nil {
				runErr = err
				break loop
			}
		}
	}
	if len(rootSteps) > 0 {
		a.marker("\nRun these root steps yourself, then run setup apply again:")
		for _, s := range rootSteps {
			a.marker("# " + s.ID + ": " + s.Title)
			for _, c := range s.Commands {
				a.marker(c)
			}
		}
	}
	end := a.now()
	a.run.EndedAt = &end
	switch {
	case runErr != nil:
		a.run.State = RunFailed
		a.run.Error = runErr.Error()
	case total == 0 && len(rootSteps) > 0:
		a.run.State = RunBlocked
	default:
		a.run.State = RunDone
	}
	a.save()
	a.emit(Event{Type: "run_done", RunID: a.run.ID, At: end, Run: a.run, Error: a.run.Error})
	return a.run, runErr
}

// selectSteps resolves the step ids the run covers. An unknown id is an
// error.
func selectSteps(steps []Step, only []string) (map[string]bool, error) {
	sel := map[string]bool{}
	if len(only) == 0 {
		for _, s := range steps {
			sel[s.ID] = true
		}
		return sel, nil
	}
	for _, id := range only {
		found := false
		for _, s := range steps {
			if s.ID == id {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown step %q", id)
		}
		sel[id] = true
	}
	return sel, nil
}

// runnableCount counts the steps of the run that will run: the selected
// ones that are not done and need no root.
func runnableCount(steps []Step, sel map[string]bool) int {
	n := 0
	for _, s := range steps {
		if sel[s.ID] && !s.Done && !s.Root {
			n++
		}
	}
	return n
}

// applier runs the steps of a plan and keeps the record of the run.
type applier struct {
	opt ApplyOptions
	log io.Writer
	run *Run
	// mu guards the record: the ticker and the step write it.
	mu sync.Mutex
}

// now is the clock of the run.
func (a *applier) now() time.Time {
	if a.opt.Clock != nil {
		return a.opt.Clock()
	}
	return time.Now().UTC()
}

// save writes the record of the run.
func (a *applier) save() {
	if a.opt.Store == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.run.DurationS = a.run.Duration(a.now())
	_ = a.opt.Store.Save(a.run)
}

// emit hands an event to the caller.
func (a *applier) emit(ev Event) {
	if a.opt.Event == nil {
		return
	}
	a.opt.Event(ev)
}

// marker writes one line of the log of the caller.
func (a *applier) marker(line string) {
	if a.opt.Quiet || a.log == nil {
		return
	}
	fmt.Fprintln(a.log, line)
}

// skip records a step that needs nothing.
func (a *applier) skip(s Step, why string) {
	rs := RunStep{ID: s.ID, Title: s.Title, State: StepSkipped, Summary: why, ExitCode: -1}
	a.record(rs)
	a.emit(Event{Type: "step_skipped", RunID: a.run.ID, At: a.now(), Step: &a.run.Steps[len(a.run.Steps)-1]})
	a.marker("== " + s.ID + ": done")
}

// block records a step that needs root.
func (a *applier) block(s Step, why string) {
	rs := RunStep{ID: s.ID, Title: s.Title, State: StepBlocked, Summary: why, ExitCode: -1, Commands: s.Commands}
	a.record(rs)
	a.emit(Event{Type: "step_blocked", RunID: a.run.ID, At: a.now(), Step: &a.run.Steps[len(a.run.Steps)-1]})
	a.marker("== " + s.ID + ": needs root, not run")
}

// record adds a step to the record of the run and writes it.
func (a *applier) record(rs RunStep) {
	a.run.Steps = append(a.run.Steps, rs)
	a.save()
}

// step runs one step of the plan and records it: its log, its duration,
// its exit code and its evidence.
func (a *applier) step(ctx context.Context, s Step, index, total int) error {
	start := a.now()
	rs := RunStep{ID: s.ID, Title: s.Title, State: StepRunning, Commands: s.Commands, StartedAt: &start, ExitCode: -1}
	if a.opt.Store != nil {
		rs.LogPath = a.opt.Store.StepLog(a.run.ID, s.ID)
	}
	a.run.Steps = append(a.run.Steps, rs)
	i := len(a.run.Steps) - 1
	a.save()
	a.emit(Event{Type: "step_started", RunID: a.run.ID, At: start, Index: index, Total: total, Step: &a.run.Steps[i]})
	a.marker("== " + s.ID + ": " + s.Title)

	// The record is written every two seconds while the step runs, so a
	// reader sees the progress of a long step.
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				a.save()
			}
		}
	}()
	defer close(stop)

	var lf *os.File
	if rs.LogPath != "" {
		f, err := os.Create(rs.LogPath)
		if err != nil {
			return a.fail(i, start, fmt.Errorf("step %s: %w", s.ID, err), -1)
		}
		lf = f
		defer lf.Close()
		fmt.Fprintf(lf, "# bernie setup run %s\n# step %s: %s\n# started %s\n",
			a.run.ID, s.ID, s.Title, start.Format(time.RFC3339))
	}
	w := &stepWriter{file: lf, log: a.log, mirror: a.opt.Mirror}
	w.onLine = func(line string) { a.progress(i, index, total, line) }

	exit := -1
	if s.Download != nil {
		err := downloadModel(ctx, newHubClient(), s.Download, w)
		if err != nil {
			return a.fail(i, start, fmt.Errorf("step %s: %s", s.ID, downloadFailure(err, s.ID)), exit)
		}
		exit = 0
	} else {
		for _, c := range s.Commands {
			if strings.HasPrefix(strings.TrimSpace(c), "#") {
				a.writeLog(lf, "%s\n", c)
				continue
			}
			a.writeLog(lf, "$ %s\n", c)
			a.marker("$ " + c)
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c", c)
			cmd.Stdout, cmd.Stderr = w, w
			if err := cmd.Run(); err != nil {
				exit = exitCodeOf(err)
				return a.fail(i, start, fmt.Errorf("step %s: %q: %w", s.ID, c, err), exit)
			}
			exit = 0
		}
	}
	end := a.now()
	step := &a.run.Steps[i]
	step.State = StepDone
	step.EndedAt = &end
	step.DurationS = end.Sub(start).Seconds()
	step.ExitCode = exit
	step.Evidence = evidenceOf(step.LogPath)
	step.Summary = summaryOf(step)
	a.writeLog(lf, "# ended %s exit=%d\n", end.Format(time.RFC3339), exit)
	a.save()
	a.emit(Event{Type: "step_done", RunID: a.run.ID, At: end, Index: index, Total: total, Step: step})
	return nil
}

// fail records a step that failed and returns the error.
func (a *applier) fail(i int, start time.Time, err error, exit int) error {
	end := a.now()
	step := &a.run.Steps[i]
	step.State = StepFailed
	step.EndedAt = &end
	step.DurationS = end.Sub(start).Seconds()
	step.ExitCode = exit
	step.Summary = "failed: " + err.Error()
	step.Evidence = evidenceOf(step.LogPath)
	a.writeLog(nil, "# ended %s exit=%d\n", end.Format(time.RFC3339), exit)
	a.save()
	a.emit(Event{Type: "step_done", RunID: a.run.ID, At: end, Step: step, Error: err.Error()})
	return err
}

// progress reads the progress out of a line of a step and keeps it in the
// record.
func (a *applier) progress(i, index, total int, line string) {
	step := &a.run.Steps[i]
	p, ok := progressOf(step.ID, line)
	if !ok {
		return
	}
	if step.Progress != nil && step.Progress.Sentence == p.Sentence && step.Progress.Fraction == p.Fraction {
		return
	}
	step.Progress = &p
	a.save()
	a.emit(Event{Type: "progress", RunID: a.run.ID, At: a.now(), Index: index, Total: total, Step: step})
}

// writeLog writes a line to the log of a step.
func (a *applier) writeLog(f *os.File, format string, args ...any) {
	if f == nil {
		return
	}
	fmt.Fprintf(f, format, args...)
}

// stepWriter takes the output of a step: it writes it to the log of the
// step with a timestamp, reads the progress out of it and, when the
// caller mirrors, writes it to the log of the caller.
type stepWriter struct {
	mu     sync.Mutex
	file   *os.File
	log    io.Writer
	mirror bool
	part   string
	onLine func(line string)
}

// Write takes the output of a step. A partial line waits for the rest.
func (w *stepWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	text := w.part + string(p)
	w.part = ""
	for {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			break
		}
		w.line(strings.TrimRight(text[:i], "\r"))
		text = text[i+1:]
	}
	w.part = text
	return len(p), nil
}

// line records one output line.
func (w *stepWriter) line(text string) {
	if w.file != nil {
		fmt.Fprintf(w.file, "[%s] %s\n", time.Now().UTC().Format(time.RFC3339), text)
	}
	if w.onLine != nil {
		w.onLine(text)
	}
	if w.mirror && w.log != nil {
		fmt.Fprintln(w.log, text)
	}
}

// exitCodeOf reads the exit code of a failed command. A command that
// could not start has the code -1.
func exitCodeOf(err error) int {
	var ee *exec.ExitError
	if ok := asExitError(err, &ee); ok {
		return ee.ExitCode()
	}
	return -1
}

// asExitError is errors.As for an exit error, without the import of
// errors in this file.
func asExitError(err error, target **exec.ExitError) bool {
	for err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			*target = ee
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// evidenceOf reads the evidence of a step out of its log: the check of
// the venv prints one marked JSON line.
func evidenceOf(logPath string) json.RawMessage {
	if logPath == "" {
		return nil
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	const marker = "BERNIE_JSON:"
	for _, line := range strings.Split(string(b), "\n") {
		i := strings.Index(line, marker)
		if i < 0 {
			continue
		}
		raw := strings.TrimSpace(line[i+len(marker):])
		if json.Valid([]byte(raw)) {
			return json.RawMessage(raw)
		}
	}
	return nil
}

// summaryOf writes the one sentence that says what a step did.
func summaryOf(step *RunStep) string {
	if len(step.Evidence) > 0 {
		if s := verifySummary(step.Evidence); s != "" {
			return s
		}
	}
	return "done"
}

// verifySummary reads the verification JSON of the venv check and writes
// the sentence that says what it found.
func verifySummary(raw json.RawMessage) string {
	var v struct {
		Python       string   `json:"python"`
		Torch        string   `json:"torch"`
		HIP          string   `json:"hip"`
		VLLM         string   `json:"vllm"`
		RDNAOps      int      `json:"rdna_ops"`
		ROCMOps      int      `json:"rocm_ops"`
		AllReduce    bool     `json:"all_reduce"`
		ROCBLAS      string   `json:"rocblas"`
		ROCBLASWheel bool     `json:"rocblas_wheel"`
		MissingLibs  []string `json:"missing_libs"`
		VerifyErrors []string `json:"verify_errors"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	var parts []string
	if v.Torch != "" {
		parts = append(parts, "torch "+v.Torch)
	}
	if v.HIP != "" {
		parts = append(parts, "hip "+v.HIP)
	}
	if v.VLLM != "" {
		parts = append(parts, "vllm "+v.VLLM)
	}
	if v.RDNAOps > 0 {
		parts = append(parts, fmt.Sprintf("%d RDNA ops", v.RDNAOps))
	}
	if v.AllReduce {
		parts = append(parts, "all_reduce registered")
	}
	if v.ROCBLAS != "" {
		parts = append(parts, "rocBLAS "+v.ROCBLAS)
	}
	if v.ROCBLASWheel {
		parts = append(parts, "from the wheel")
	}
	if len(v.MissingLibs) > 0 {
		parts = append(parts, "missing: "+strings.Join(v.MissingLibs, ", "))
	}
	if len(v.VerifyErrors) > 0 {
		parts = append(parts, "failed: "+strings.Join(v.VerifyErrors, "; "))
	}
	return strings.Join(parts, ", ")
}
