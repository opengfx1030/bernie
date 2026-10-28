package runner

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenLogContainerReadsEngineLogs(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\n[ \"$1 $2\" = \"logs c1\" ] || exit 9\necho out-line\necho 'CUDA out of memory' >&2\n"
	if err := os.WriteFile(filepath.Join(bin, "fakeeng"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	rc, err := OpenLog(context.Background(), Record{Name: "r", Mode: "container", Engine: "fakeeng", Container: "c1"}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if got := string(b); !strings.Contains(got, "out-line") || !strings.Contains(got, "CUDA out of memory") {
		t.Errorf("log = %q", got)
	}
}

func TestOpenLogContainerTimesOut(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "fakeeng"), []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	start := time.Now()
	_, err := OpenLog(context.Background(), Record{Name: "r", Engine: "fakeeng", Container: "c1"}, 200*time.Millisecond)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Errorf("err = %v after %s; want a timeout error", err, time.Since(start))
	}
}

func TestOpenLogHostFallsBackToRunDirAndStopsWithContext(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	if err := os.MkdirAll(RunDir("r"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(RunDir("r"), "serve.log"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rc, err := OpenLog(ctx, Record{Name: "r", Log: "/nonexistent/serve.log"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	buf := make([]byte, 3)
	if _, err := rc.Read(buf); err != nil || string(buf) != "hel" {
		t.Fatalf("read = %q, %v", buf, err)
	}
	cancel()
	if _, err := rc.Read(buf); err != context.Canceled {
		t.Errorf("read after cancel: %v", err)
	}
}
