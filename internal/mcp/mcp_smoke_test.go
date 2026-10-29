package mcp

import (
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

func TestSmokeIsReadOnly(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{})
	listed := names(listTools(t, cs))
	if !slices.Contains(listed, "smoke") {
		t.Error("the read-only catalog does not list smoke")
	}
	if IsMutating("smoke") {
		t.Error("smoke should be read-only")
	}
}

func TestSmokeAnswersLocalServer(t *testing.T) {
	isolate(t)
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{}))
	defer srv.Close()
	cs := connect(t, Options{})
	res := callTool(t, cs, "smoke", map[string]any{"url": srv.URL})
	if res.IsError {
		t.Fatalf("smoke: %s", resultText(res))
	}
	doc, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("smoke document: %T", res.StructuredContent)
	}
	if doc["health"] != true || doc["model"] != "fake-model" {
		t.Errorf("smoke document: %+v", doc)
	}
}

func TestSmokeRefusesRemoteURL(t *testing.T) {
	isolate(t)
	cs := connect(t, Options{})
	res := callTool(t, cs, "smoke", map[string]any{"url": "http://10.1.2.3:8000"})
	if !res.IsError {
		t.Error("smoke accepted a remote URL")
	}
}
