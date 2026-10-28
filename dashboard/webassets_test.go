package dashboard

import (
	"net/http"
	"strings"
	"testing"
)

// The page loads three stylesheets. Every one of them has to be embedded and
// served, or the page loses its styles.
func TestStylesheetsAreServed(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	for _, name := range []string{"tokens.css", "layout.css", "components.css"} {
		w := do(t, h, "GET", "/"+name, map[string]string{"Host": "127.0.0.1:7860"})
		if w.Code != http.StatusOK {
			t.Errorf("GET /%s: %d", name, w.Code)
			continue
		}
		if !strings.Contains(w.Body.String(), "{") {
			t.Errorf("GET /%s: the body holds no rule", name)
		}
	}
}

// The page links the three stylesheets and not the old single file.
func TestThePageLinksTheThreeStylesheets(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	w := do(t, h, "GET", "/", map[string]string{"Host": "127.0.0.1:7860"})
	if w.Code != http.StatusOK {
		t.Fatalf("GET /: %d", w.Code)
	}
	body := w.Body.String()
	for _, name := range []string{"tokens.css", "layout.css", "components.css"} {
		if !strings.Contains(body, `href="`+name+`"`) {
			t.Errorf("the page does not link %s", name)
		}
	}
	if strings.Contains(body, "app.css") {
		t.Error("the page still links app.css")
	}
	if w := do(t, h, "GET", "/app.css", map[string]string{"Host": "127.0.0.1:7860"}); w.Code != http.StatusNotFound {
		t.Errorf("GET /app.css: %d, want 404", w.Code)
	}
}
