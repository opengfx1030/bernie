package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/opengfx1030/bernie/internal/logdoc"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// The state of a job.
const (
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

// ErrJobBusy means another job is running. One job runs at a time.
var ErrJobBusy = errors.New("one setup job runs at a time: wait for the job that runs now to finish")

// Job is one background run of an onboarding step. The state is kept on disk
// (<state>/jobs/<id>), so a page reload finds it again.
type Job struct {
	ID         string         `json:"id"`
	Step       string         `json:"step"`
	State      string         `json:"state"`
	Phase      string         `json:"phase,omitempty"`
	Progress   *JobProgress   `json:"progress,omitempty"`
	Error      string         `json:"error,omitempty"`
	Findings   []logdoc.Match `json:"findings,omitempty"`
	Lines      []string       `json:"lines,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	// PID is the process that runs the job. A job that is running while its
	// process is gone is failed: nothing will finish it.
	PID int `json:"pid,omitempty"`
}

// JobProgress is the progress of a running job: the phase and the numbers
// of the work. The numbers are there when the step counts them; a step that
// only counts items keeps the done and the total of the step.
type JobProgress struct {
	Phase      string `json:"phase"`
	BytesDone  int64  `json:"bytes_done,omitempty"`
	BytesTotal int64  `json:"bytes_total,omitempty"`
	RateBPS    int64  `json:"rate_bps,omitempty"`
	FilesDone  int    `json:"files_done,omitempty"`
	FilesTotal int    `json:"files_total,omitempty"`
	ETAS       int    `json:"eta_s,omitempty"`
}

// jobMu holds the single job slot of this process.
var jobMu sync.Mutex

// maxJobLines is the number of output lines a job keeps.
const maxJobLines = 50

// StartJob runs a prepared onboarding action in the background and returns
// the job. The job outlives the call: it runs until it is done or it fails.
// One job runs at a time; a second call gets ErrJobBusy.
func StartJob(ctx context.Context, prepared *PreparedStep) (*Job, error) {
	if prepared == nil || prepared.Action == nil {
		return nil, ErrNoAction
	}
	return startJob(ctx, prepared.Step, prepared.Action)
}

// startJob runs a prepared action in the background. One job runs at a time.
func startJob(ctx context.Context, step string, action *Action) (*Job, error) {
	if !jobMu.TryLock() {
		return nil, ErrJobBusy
	}
	id := newJobID()
	dir, err := paths.Ensure(jobDir(id))
	if err != nil {
		jobMu.Unlock()
		return nil, err
	}
	logf, err := os.Create(filepath.Join(dir, "job.log"))
	if err != nil {
		jobMu.Unlock()
		return nil, err
	}
	job := &Job{ID: id, Step: step, State: JobRunning, StartedAt: time.Now().UTC(), PID: os.Getpid()}
	if err := saveJob(job); err != nil {
		logf.Close()
		jobMu.Unlock()
		return nil, err
	}
	// The job outlives the request that started it.
	go runJob(context.WithoutCancel(ctx), job, action, logf)
	return job, nil
}

// runJob does the action of a job and records its end.
func runJob(ctx context.Context, job *Job, action *Action, logf *os.File) {
	defer jobMu.Unlock()
	defer logf.Close()
	w := &jobWriter{job: job, f: logf}
	err := action.Run(ctx, w)
	now := time.Now().UTC()
	job.FinishedAt = &now
	switch {
	case err == nil:
		job.State = JobDone
	case errors.Is(err, context.Canceled):
		job.State = JobFailed
		job.Error = "the job was cancelled"
	default:
		job.State = JobFailed
		job.Error = err.Error()
		job.Findings = findingsOf(ctx, action)
	}
	_ = saveJob(job)
}

// findingsOf scans the log of the run a serve job started. It returns the
// known failures of the catalog, or nothing.
func findingsOf(ctx context.Context, action *Action) []logdoc.Match {
	if action.Kind != ActionServe || action.RunName == "" {
		return nil
	}
	rules, err := logdoc.Load()
	if err != nil {
		return nil
	}
	rec, err := runner.Get(action.RunName)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, findingsBudget)
	defer cancel()
	rc, err := runner.OpenLog(ctx, *rec, findingsBudget)
	if err != nil {
		return nil
	}
	defer rc.Close()
	return logdoc.Scan(rc, rules)
}

// findingsBudget bounds the scan of a log.
const findingsBudget = 2 * time.Second

// jobWriter writes the output of a job: to the log file, to the last lines
// of the job and to the phase of the job.
type jobWriter struct {
	mu      sync.Mutex
	job     *Job
	f       *os.File
	lines   []string
	partial string
}

// Write takes the output of the job. A partial line waits for the rest.
func (w *jobWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	text := w.partial + string(p)
	w.partial = ""
	for {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			break
		}
		w.line(strings.TrimRight(text[:i], "\r"))
		text = text[i+1:]
	}
	w.partial = text
	return len(p), nil
}

// line records one output line.
func (w *jobWriter) line(text string) {
	if text == "" {
		return
	}
	_, _ = io.WriteString(w.f, text+"\n")
	w.lines = append(w.lines, text)
	if len(w.lines) > maxJobLines {
		w.lines = w.lines[len(w.lines)-maxJobLines:]
	}
	w.job.Phase = text
	w.job.Lines = append([]string(nil), w.lines...)
	w.progress(text)
	_ = saveJob(w.job)
}

// progress reads the numbers of a line into the progress of the job: the
// phase and, for a download, the bytes and the rate.
func (w *jobWriter) progress(text string) {
	p := w.job.Progress
	if p == nil {
		p = &JobProgress{Phase: w.job.Step}
		w.job.Progress = p
	}
	if !strings.HasPrefix(text, "model.download: ") {
		return
	}
	p.Phase = "model.download"
	done, total, rate, ok := parseDownload(text)
	if !ok {
		return
	}
	p.BytesDone, p.BytesTotal, p.RateBPS = done, total, rate
	if rate > 0 && total > done {
		p.ETAS = int((total - done) / rate)
	}
}

// parseDownload reads the numbers of a download line:
// "model.download: 1.2 GB of 2.3 GB (52%) at 45.2 MB/s".
func parseDownload(line string) (done, total, rate int64, ok bool) {
	rest := strings.TrimPrefix(line, "model.download: ")
	at := strings.Split(rest, " at ")
	if len(at) != 2 {
		return 0, 0, 0, false
	}
	sizes := strings.Split(at[0], " of ")
	if len(sizes) != 2 {
		return 0, 0, 0, false
	}
	done, ok = parseBytes(sizes[0])
	if !ok {
		return 0, 0, 0, false
	}
	total, ok = parseBytes(stripPercent(sizes[1]))
	if !ok {
		return 0, 0, 0, false
	}
	rate, ok = parseBytes(strings.TrimSuffix(strings.TrimSpace(at[1]), "/s"))
	if !ok {
		return 0, 0, 0, false
	}
	return done, total, rate, true
}

// stripPercent removes the share of a size: "2.3 GB (52%)" is "2.3 GB".
func stripPercent(s string) string {
	if i := strings.Index(s, " ("); i >= 0 {
		return s[:i]
	}
	return s
}

// parseBytes reads a size in bytes: "512 B", "45.2 MB". The units are
// decimal, like the ones the download prints.
func parseBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	var mult int64
	switch strings.TrimSpace(s[i:]) {
	case "B", "":
		mult = 1
	case "kB":
		mult = 1000
	case "MB":
		mult = 1000 * 1000
	case "GB":
		mult = 1000 * 1000 * 1000
	case "TB":
		mult = 1000 * 1000 * 1000 * 1000
	default:
		return 0, false
	}
	return int64(n * float64(mult)), true
}

// JobSummary is the list document of a job: the id, the step, the state and
// the progress. The page re-attaches to a job with its id.
type JobSummary struct {
	ID         string       `json:"id"`
	Step       string       `json:"step"`
	State      string       `json:"state"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
	Phase      string       `json:"phase,omitempty"`
	Progress   *JobProgress `json:"progress,omitempty"`
}

// DefaultJobLimit is the number of jobs the list answers.
const DefaultJobLimit = 50

// JobSummaries returns the jobs of this host, newest first, at most limit of
// them. A limit below one means the default.
func JobSummaries(limit int) []JobSummary {
	if limit < 1 {
		limit = DefaultJobLimit
	}
	jobs := RecentJobs()
	out := make([]JobSummary, 0, min(len(jobs), limit))
	for _, j := range jobs {
		if len(out) == limit {
			break
		}
		out = append(out, JobSummary{
			ID: j.ID, Step: j.Step, State: j.State, StartedAt: j.StartedAt,
			FinishedAt: j.FinishedAt, Phase: j.Phase, Progress: j.Progress,
		})
	}
	return out
}

// RecentJobs returns the jobs of this host, newest first. A job that was
// running when its process ended is marked failed. A job of a process that
// is still there keeps running: it is this process or another one.
func RecentJobs() []Job {
	dirs, _ := filepath.Glob(filepath.Join(paths.State(), "jobs", "*"))
	out := []Job{}
	for _, d := range dirs {
		j, err := readJob(filepath.Base(d))
		if err != nil {
			continue
		}
		if j.State == JobRunning && !jobAlive(*j) {
			now := time.Now().UTC()
			j.State = JobFailed
			j.Error = "the process that ran this job ended"
			j.FinishedAt = &now
			_ = saveJob(j)
		}
		out = append(out, *j)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

// jobAlive says whether the process that runs the job is still there. The
// job of this process is alive by definition.
func jobAlive(j Job) bool {
	if j.PID <= 0 {
		return false // a job from before the pid was recorded
	}
	if j.PID == os.Getpid() {
		return true
	}
	return processAlive(j.PID)
}

// processAlive says whether a process with that pid is still there.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// GetJob returns the job with this id.
func GetJob(id string) (*Job, error) { return readJob(id) }

// JobLog returns the output lines of a job, oldest first.
func JobLog(id string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(jobDir(id), "job.log"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// lastJob returns the newest job of a step, or nil.
func lastJob(jobs []Job, step string) *Job {
	var newest *Job
	for i := range jobs {
		if jobs[i].Step != step {
			continue
		}
		if newest == nil || jobs[i].StartedAt.After(newest.StartedAt) {
			newest = &jobs[i]
		}
	}
	return newest
}

// jobDir is <state>/jobs/<id>.
func jobDir(id string) string { return filepath.Join(paths.State(), "jobs", id) }

// saveJob writes the job document.
func saveJob(j *Job) error {
	dir, err := paths.Ensure(jobDir(j.ID))
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "job.json.tmp")
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "job.json"))
}

// readJob reads the job document of an id.
func readJob(id string) (*Job, error) {
	b, err := os.ReadFile(filepath.Join(jobDir(id), "job.json"))
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, fmt.Errorf("job %s: %w", id, err)
	}
	return &j, nil
}

// newJobID returns a sortable id: the time and four random bytes.
func newJobID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102T150405Z")
	}
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}
