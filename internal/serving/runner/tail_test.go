package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TailBytes reads only the end of a big log: the file is never read whole.
func TestTailBytesReadsOnlyTheEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "serve.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20000 {
		if _, err := fmt.Fprintf(f, "line %06d padding padding padding\n", i); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	rec := Record{Name: "big", Log: path}
	b, err := TailBytes(context.Background(), rec, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 4096 {
		t.Errorf("tail is %d bytes, want at most 4096", len(b))
	}
	if !strings.HasSuffix(string(b), "line 019999 padding padding padding\n") {
		t.Errorf("tail does not end with the last line: %q", tail(b))
	}
	if !strings.HasPrefix(string(b), "line ") {
		t.Errorf("tail starts in the middle of a line: %q", tail(b))
	}
}

func TestTailBytesOfASmallLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "serve.log")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := Record{Name: "small", Log: path}
	b, err := TailBytes(context.Background(), rec, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "one\ntwo\n" {
		t.Errorf("tail = %q", b)
	}
}

func TestTailBytesOfAMissingLog(t *testing.T) {
	rec := Record{Name: "gone", Log: filepath.Join(t.TempDir(), "serve.log")}
	if _, err := TailBytes(context.Background(), rec, 4096); err == nil {
		t.Error("missing log: no error")
	}
}

// tail shortens a byte slice for an error message.
func tail(b []byte) string {
	if len(b) > 120 {
		return string(b[:120]) + "..."
	}
	return string(b)
}
