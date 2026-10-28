package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/logdoc"
)

var oomMatch = logdoc.Match{
	Rule: logdoc.Rule{ID: "memory.gpu-oom", Severity: "error", Cause: "The GPU ran out of VRAM.", Fix: "Lower --max-model-len.", Doc: "https://x/y"},
	Line: "CUDA out of memory", LineNo: 82,
}

func TestReportExitText(t *testing.T) {
	var out, errw bytes.Buffer
	c := &Context{Ctx: context.Background(), Out: &out, Err: &errw}
	err := c.ReportExit("r1", []logdoc.Match{oomMatch})
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitChecks {
		t.Fatalf("err = %v, want exit %d", err, ExitChecks)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
	for _, w := range []string{"r1 exited", "memory.gpu-oom", "line 82", "cause: The GPU ran out of VRAM.", "fix:   Lower --max-model-len."} {
		if !strings.Contains(errw.String(), w) {
			t.Errorf("stderr lacks %q:\n%s", w, errw.String())
		}
	}
}

func TestReportExitJSON(t *testing.T) {
	var out, errw bytes.Buffer
	c := &Context{Ctx: context.Background(), Out: &out, Err: &errw, JSON: true}
	if err := c.ReportExit("r1", []logdoc.Match{oomMatch}); err == nil {
		t.Fatal("want exit-3 error")
	}
	var doc struct {
		Run      string `json:"run"`
		Findings []struct {
			ID, Fix string
			LineNo  int `json:"line_no"`
		} `json:"log_findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	if doc.Run != "r1" || len(doc.Findings) != 1 || doc.Findings[0].ID != "memory.gpu-oom" || doc.Findings[0].LineNo != 82 {
		t.Errorf("doc = %+v", doc)
	}
	if errw.Len() != 0 {
		t.Errorf("stderr = %q, want empty (the exit message is printed by Main)", errw.String())
	}
}
