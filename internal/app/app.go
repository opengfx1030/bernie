// Package app holds the operations that more than one front end (CLI,
// dashboard, MCP server) runs: probe the host, build a report, run a
// benchmark. Each function returns the canonical struct of the matching
// `--json` document and never prints. Front ends parse input and format
// output; domain packages own the host behavior.
package app

import (
	"context"
	"io"

	"github.com/opengfx1030/bernie/internal/onboarding"
)

// root is the host filesystem root the probes read.
const root = "/"

// logf writes one progress line to w. A nil w discards it.
func logf(w io.Writer, line string) {
	if w != nil {
		io.WriteString(w, line+"\n")
	}
}

// OnboardingOptions are the inputs of the onboarding: the flags of `up` and
// of the onboarding endpoints.
type OnboardingOptions = onboarding.Options

// Onboarding returns the onboarding document of this host: the ordered steps
// that lead from a bare machine to a serving model (spec §11). Every front
// end reads this one document, so the page and the CLI cannot disagree.
func Onboarding(ctx context.Context, o OnboardingOptions) (*onboarding.Document, error) {
	f, err := onboarding.Gather(ctx, o)
	if err != nil {
		return nil, err
	}
	return onboarding.Build(f), nil
}

// OnboardingCached returns the onboarding document of this host from the
// cache when it is fresh enough, and refreshes it in the background when it
// is not (spec §3.3). The document says how old it is.
func OnboardingCached(ctx context.Context, o OnboardingOptions) (*onboarding.Document, error) {
	return onboarding.Load(ctx, o)
}

// StartOnboardingJob runs the action of an onboarding step in the background
// and returns the job. One job runs at a time.
func StartOnboardingJob(ctx context.Context, step string, o OnboardingOptions) (*onboarding.Job, error) {
	return onboarding.StartJob(ctx, step, o)
}

// OnboardingJob returns the job with this id.
func OnboardingJob(id string) (*onboarding.Job, error) {
	return onboarding.GetJob(id)
}

// OnboardingJobLog returns the output lines of a job, oldest first.
func OnboardingJobLog(id string) ([]string, error) {
	return onboarding.JobLog(id)
}

// SmokeOptions are the inputs of the smoke check.
type SmokeOptions struct {
	URL   string // base URL of the server; "" = the running run
	Model string // model name; "" = the first id of /v1/models
}

// Smoke asks a server for one token: the smallest request that proves the
// server answers (spec §3.1).
func Smoke(ctx context.Context, o SmokeOptions) (*onboarding.SmokeResult, error) {
	url := o.URL
	if url == "" {
		run, err := onboarding.LiveRun(ctx)
		if err != nil {
			return nil, err
		}
		url = run.URL
	}
	return onboarding.Smoke(ctx, url, o.Model)
}
