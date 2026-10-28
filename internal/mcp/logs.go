package mcp

import (
	"bytes"
	"context"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/support/redact"
)

// logOut is the result of the logs tool. Run logs and model output are
// attacker-controlled data: Untrusted is always true, and the content never
// reaches a tool description or the tool catalog.
type logOut struct {
	Source    string `json:"source"`    // always "run-log"
	Untrusted bool   `json:"untrusted"` // always true
	Run       string `json:"run"`
	Truncated bool   `json:"truncated"`
	Content   string `json:"content"`
}

func runLogs(ctx context.Context, in logsIn) (logOut, error) {
	tail := in.Tail
	if tail == 0 {
		tail = defaultTail
	}
	tb := &tailBuf{max: maxLogBytes}
	if err := runner.Logs(ctx, in.Name, false, tail, tb); err != nil {
		return logOut{}, err
	}
	text, cut := tb.text()
	return logOut{Source: "run-log", Untrusted: true, Run: in.Name, Truncated: cut, Content: redact.Text(text)}, nil
}

// tailBuf keeps the last max bytes written to it.
type tailBuf struct {
	max int
	b   []byte
	cut bool
}

func (t *tailBuf) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > 2*t.max {
		t.b = append([]byte(nil), t.b[len(t.b)-t.max:]...)
		t.cut = true
	}
	return len(p), nil
}

// text returns the kept bytes from the first whole line on, and whether any
// bytes were dropped.
func (t *tailBuf) text() (string, bool) {
	b := t.b
	if len(b) > t.max {
		b, t.cut = b[len(b)-t.max:], true
	}
	if t.cut {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return string(b), t.cut
}
