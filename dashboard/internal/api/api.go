// Package api serves the dashboard's JSON and server-sent event endpoints.
// Each domain file owns one area of the API from spec §11.
package api

import (
	"io"
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/dashboard/internal/telemetry"
)

// Telemetry is the live sample source used by GET /api/telemetry. It also
// owns the ring buffer of the samples of the last minutes, which
// GET /api/gpus/history reads.
type Telemetry interface {
	Subscribe() (<-chan []byte, func())
	History() *telemetry.History
}

// Options configures the API routes.
type Options struct {
	// Log receives progress from long-running writes. Nil discards.
	Log io.Writer
	// Telemetry supplies the shared live samples.
	Telemetry Telemetry
}

type handler struct {
	log       io.Writer
	telemetry Telemetry
	bench     *benchJobs
}

// Register adds every dashboard API route to mux.
func Register(mux *http.ServeMux, o Options) {
	if o.Log == nil {
		o.Log = io.Discard
	}
	h := &handler{log: o.Log, telemetry: o.Telemetry, bench: newBenchJobs()}

	mux.HandleFunc("GET /api/hw", h.getHW)
	mux.HandleFunc("GET /api/topo", h.getTopo)
	mux.HandleFunc("GET /api/topo/check", h.getTopoCheck)
	mux.HandleFunc("GET /api/doctor", h.getDoctor)
	mux.HandleFunc("GET /api/recipes", h.getRecipes)
	mux.HandleFunc("GET /api/recipes/{id}/plan", h.getPlan)
	mux.HandleFunc("GET /api/runs", h.getRuns)
	mux.HandleFunc("POST /api/runs", h.postRun)
	mux.HandleFunc("DELETE /api/runs/{name}", h.deleteRun)
	mux.HandleFunc("GET /api/runs/{name}/logs", h.getLogs)
	mux.HandleFunc("GET /api/runs/{name}/log", h.getRunLog)
	mux.HandleFunc("GET /api/runs/{name}/findings", h.getRunFindings)
	mux.HandleFunc("GET /api/telemetry", h.getTelemetry)
	mux.HandleFunc("GET /api/gpus/history", h.getHistory)
	mux.HandleFunc("POST /api/smoke", h.postSmoke)
	mux.HandleFunc("GET /api/power", h.getPower)
	mux.HandleFunc("PUT /api/power", h.putPower)
	mux.HandleFunc("GET /api/bench", h.getBench)
	mux.HandleFunc("GET /api/bench/suites", h.getBenchSuites)
	mux.HandleFunc("GET /api/bench/compare", h.getBenchCompare)
	mux.HandleFunc("POST /api/bench", h.postBench)
	mux.HandleFunc("POST /api/bench/run", h.postBenchRun)
	mux.HandleFunc("POST /api/bench/ab", h.postBenchAB)
	mux.HandleFunc("GET /api/bench/{id}", h.getBenchJob)
	mux.HandleFunc("GET /api/bench/{id}/events", h.getBenchEvents)
	mux.HandleFunc("GET /api/onboarding", h.getOnboarding)
	mux.HandleFunc("POST /api/onboarding/{step}/run", h.postOnboardingRun)
	mux.HandleFunc("GET /api/jobs", h.getJobs)
	mux.HandleFunc("GET /api/jobs/{id}", h.getJob)
	mux.HandleFunc("GET /api/jobs/{id}/events", h.getJobEvents)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpjson.Error(w, http.StatusNotFound, "no such endpoint: "+r.Method+" "+r.URL.Path)
	})
}
