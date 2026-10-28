package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/opengfx1030/bernie/internal/logdoc"
)

// WriteMatches prints log findings: one block per rule with its cause, fix,
// doc link and the matching line.
func WriteMatches(w io.Writer, ms []logdoc.Match) {
	for _, m := range ms {
		fmt.Fprintf(w, "\n[%s] %s (line %d)\n", m.Severity, m.ID, m.LineNo)
		fmt.Fprintf(w, "  cause: %s\n", m.Cause)
		fmt.Fprintf(w, "  fix:   %s\n", m.Fix)
		fmt.Fprintf(w, "  doc:   %s\n", m.Doc)
		fmt.Fprintf(w, "  line:  %s\n", strings.TrimSpace(m.Line))
	}
}

// ReportExit reports the known failures found in the log of a run that exited
// (spec §5.3) and returns the exit-3 error of a failed check.
// Text goes to stderr; with --json the document is
// {"run": name, "log_findings": [...]} on stdout.
func (c *Context) ReportExit(run string, ms []logdoc.Match) error {
	if c.JSON {
		if err := c.Emit(map[string]any{"run": run, "log_findings": ms}, nil); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(c.Err, "%s exited; known failures in its log:\n", run)
		WriteMatches(c.Err, ms)
		fmt.Fprintln(c.Err)
	}
	return ChecksFailed("%s exited: %d known failure(s) in its log (bernie logs %s)", run, len(ms), run)
}
