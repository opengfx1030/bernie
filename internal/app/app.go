// Package app holds the operations that more than one front end (CLI,
// dashboard, MCP server) runs: probe the host, build a report, run a
// benchmark. Each function returns the canonical struct of the matching
// `--json` document and never prints. Front ends parse input and format
// output; domain packages own the host behavior.
package app

import "io"

// root is the host filesystem root the probes read.
const root = "/"

// logf writes one progress line to w. A nil w discards it.
func logf(w io.Writer, line string) {
	if w != nil {
		io.WriteString(w, line+"\n")
	}
}
