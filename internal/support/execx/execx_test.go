package execx

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunCapturesOutput(t *testing.T) {
	res, err := Run(context.Background(), 0, "sh", "-c", "echo out; echo err >&2; exit 3")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "out\n" || res.Stderr != "err\n" || res.ExitCode != 3 || res.OK() || res.Missing {
		t.Errorf("result = %+v", res)
	}
	res, err = Run(context.Background(), 0, "sh", "-c", "printf ok")
	if err != nil || !res.OK() || res.Stdout != "ok" {
		t.Errorf("success: %+v, %v", res, err)
	}
}

func TestRunMissingBinary(t *testing.T) {
	res, err := Run(context.Background(), 0, "bernie-no-such-binary")
	if err != nil || !res.Missing || res.OK() || res.ExitCode != -1 {
		t.Errorf("missing: %+v, %v", res, err)
	}
	if Which("bernie-no-such-binary") != "" || Which("sh") == "" {
		t.Error("Which")
	}
}

func TestRunTimeoutKillsProcess(t *testing.T) {
	start := time.Now()
	res, err := Run(context.Background(), 200*time.Millisecond, "sh", "-c", "echo before; sleep 30")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("returned after %v, the child was not killed", d)
	}
	if res.ExitCode != -1 || res.OK() {
		t.Errorf("result = %+v", res)
	}
}

func TestRunEnv(t *testing.T) {
	res, err := RunEnv(context.Background(), 0, []string{"BERNIE_EXECX_TEST=hello"}, "sh", "-c", "printf %s \"$BERNIE_EXECX_TEST\"")
	if err != nil || res.Stdout != "hello" {
		t.Errorf("RunEnv: %+v, %v", res, err)
	}
}

func TestFirstLine(t *testing.T) {
	for in, want := range map[string]string{
		"":                   "",
		"\n  \n":             "",
		"one":                "one",
		"\n\n  two  \nthree": "two",
		"a\r\nb":             "a",
	} {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q, want %q", in, got, want)
		}
	}
}
