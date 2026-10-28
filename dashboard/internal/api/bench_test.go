package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBenchSuiteIgnoresTheCellFlags checks the contract of the run form: a
// suite decides the cells, so the cell flags of the request do not apply.
func TestBenchSuiteIgnoresTheCellFlags(t *testing.T) {
	in := 4096
	warm := 3
	req := benchRequest{Suite: "pr", InputLen: &in, OutputLen: &in, Warmup: &warm, Recall: true, NumPrompts: 7}
	o, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if o.Suite != "pr" {
		t.Errorf("the options name the suite %q", o.Suite)
	}
	if o.InputLen != 0 || o.OutputLen != 0 || len(o.Concurrency) != 0 {
		t.Errorf("the suite run keeps a lens: %+v", o)
	}
	if o.Recall {
		t.Error("the suite run keeps the recall flag of the request")
	}
	if o.NumPrompts != 7 {
		t.Errorf("the suite run drops the request count: %+v", o)
	}
}

// TestBenchRequestKeepsTheCellFlags checks that a run without a suite keeps
// the flags of the request.
func TestBenchRequestKeepsTheCellFlags(t *testing.T) {
	in := 4096
	warm := 3
	req := benchRequest{InputLen: &in, OutputLen: &in, Warmup: &warm, Recall: true, Concurrency: json.RawMessage(`"1,8"`)}
	o, err := req.options()
	if err != nil {
		t.Fatal(err)
	}
	if o.Suite != "" {
		t.Errorf("the options name the suite %q", o.Suite)
	}
	if o.InputLen != 4096 || o.OutputLen != 4096 || o.Warmup != 3 || !o.Recall {
		t.Errorf("the options lost a flag: %+v", o)
	}
	if len(o.Concurrency) != 2 || o.Concurrency[0] != 1 || o.Concurrency[1] != 8 {
		t.Errorf("the options hold the levels %v", o.Concurrency)
	}
}

// TestParseConcurrency checks the parsing of the concurrency field of the
// run form: a list or a comma string.
func TestParseConcurrency(t *testing.T) {
	for in, want := range map[string]string{"": "[1]", "null": "[1]", "[1,8]": "[1 8]", `"1, 4,16"`: "[1 4 16]", "[]": "[1]"} {
		got, err := parseConcurrency(json.RawMessage(in))
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if s := strings.Join(strings.Fields(strings.Trim(strings.ReplaceAll(fmtInts(got), ",", " "), "[]")), " "); "["+s+"]" != want {
			t.Errorf("%s: got %v, want %s", in, got, want)
		}
	}
	for _, bad := range []string{"[0]", `"1,x"`, "{}", "[-2]"} {
		if _, err := parseConcurrency(json.RawMessage(bad)); err == nil {
			t.Errorf("%s: want error", bad)
		}
	}
}

func fmtInts(v []int) string {
	b, _ := json.Marshal(v)
	return string(b)
}
