// Package httpjson writes the dashboard's JSON response shapes.
package httpjson

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/opengfx1030/bernie/internal/support/redact"
)

// Write writes one indented JSON document. The document is redacted before it
// leaves the process: everything the browser sees goes through
// internal/support/redact (dashboard/AGENTS.md, hard rule 6). Every handler
// of the API writes with this function, so a route that is added later cannot
// forget the rule.
func Write(w http.ResponseWriter, code int, v any) {
	g, err := redact.JSON(v)
	if err != nil {
		Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, code, g)
}

// write writes the document as it is. It is the one place that touches the
// wire.
func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// Error writes the CLI's --json failure shape: {"error": "...", "exit_code": N}.
func Error(w http.ResponseWriter, code int, msg string) {
	exit := 1
	if code == http.StatusBadRequest {
		exit = 2
	}
	Write(w, code, map[string]any{"error": msg, "exit_code": exit})
}

// Decode reads a JSON body (max 1 MiB) with unknown fields rejected.
func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		Error(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}
