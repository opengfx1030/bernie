package onboarding

// The store of the jobs: the documents on disk, the list of them and the
// liveness of the process that runs one.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/opengfx1030/bernie/internal/support/paths"
)

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
