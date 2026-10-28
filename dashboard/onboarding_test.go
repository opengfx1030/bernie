package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestOnboardingEndpoint(t *testing.T) {
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()

	w := do(t, h, "GET", "/api/onboarding", map[string]string{"Host": "127.0.0.1:7860"})
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/onboarding: %d %s", w.Code, w.Body.String())
	}
	var doc struct {
		Next  string           `json:"next"`
		Steps []map[string]any `json:"steps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the document is not JSON: %v", err)
	}
	if len(doc.Steps) == 0 {
		t.Fatal("the document has no steps")
	}
	if doc.Steps[0]["id"] != "hardware" {
		t.Errorf("the first step is %v, want hardware", doc.Steps[0]["id"])
	}
	if _, ok := doc.Steps[0]["state"]; !ok {
		t.Error("a step has no state")
	}
}

func TestOnboardingRunNeedsTheBernieHeader(t *testing.T) {
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()

	w := do(t, h, "POST", "/api/onboarding/model/run", map[string]string{"Host": "127.0.0.1:7860"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("without X-Bernie: %d, want 403", w.Code)
	}
}

func TestOnboardingRunNeedsTheTokenInTokenMode(t *testing.T) {
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	h := newServer(Options{Listen: "0.0.0.0:7860", Token: testToken}).handler()
	host := map[string]string{"Host": "gpu-box:7860"}

	w := do(t, h, "POST", "/api/onboarding/model/run", host)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d, want 401", w.Code)
	}
	w = do(t, h, "POST", "/api/onboarding/model/run", map[string]string{
		"Host": "gpu-box:7860", "Authorization": "Bearer " + testToken, "X-Bernie": "1",
	})
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Fatalf("with a token and the header: %d", w.Code)
	}
}

func TestAnUnknownStepIsRefused(t *testing.T) {
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()

	w := do(t, h, "POST", "/api/onboarding/nope/run", map[string]string{"Host": "127.0.0.1:7860", "X-Bernie": "1"})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "nope") {
		t.Fatalf("unknown step: %d %s", w.Code, w.Body.String())
	}
}
