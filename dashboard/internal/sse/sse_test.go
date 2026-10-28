package sse

import (
	"net/http/httptest"
	"testing"
)

func TestLineEventsFraming(t *testing.T) {
	rec := httptest.NewRecorder()
	st := New(rec)
	lw := &LineWriter{Stream: st}
	for _, chunk := range []string{"first li", "ne\nsecond\r\n", "bar 10%\rbar 20%\r", "\ntail"} {
		if _, err := lw.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if err := lw.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := st.Event("end", ""); err != nil {
		t.Fatal(err)
	}
	want := "data: first line\n\ndata: second\n\ndata: bar 10%\n\ndata: bar 20%\n\ndata: tail\n\nevent: end\ndata: \n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("framing:\n got %q\nwant %q", got, want)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
}
