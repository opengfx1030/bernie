package dashboard

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// linkRE finds the stylesheets the page links. The list of the names is not
// fixed: a new stylesheet is a new link, and every link must answer.
var linkRE = regexp.MustCompile(`<link[^>]+rel="stylesheet"[^>]*href="([^"]+)"`)

// scriptRE finds the scripts the page loads.
var scriptRE = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)

// The page links its stylesheets; every one of them is served and holds a
// rule. A stylesheet that is not served leaves the page without its styles.
func TestTheLinkedStylesheetsAreServed(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	w := do(t, h, "GET", "/", map[string]string{"Host": "127.0.0.1:7860"})
	if w.Code != http.StatusOK {
		t.Fatalf("GET /: %d", w.Code)
	}
	body := w.Body.String()
	links := linkRE.FindAllStringSubmatch(body, -1)
	if len(links) == 0 {
		t.Fatal("the page links no stylesheet")
	}
	for _, m := range links {
		name := m[1]
		r := do(t, h, "GET", "/"+name, map[string]string{"Host": "127.0.0.1:7860"})
		if r.Code != http.StatusOK {
			t.Errorf("GET /%s: %d", name, r.Code)
			continue
		}
		if !strings.Contains(r.Body.String(), "{") {
			t.Errorf("GET /%s: the body holds no rule", name)
		}
	}
}

// Every script the page loads is served. A script that is not served leaves
// the page without a part of itself.
func TestTheLinkedScriptsAreServed(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	w := do(t, h, "GET", "/", map[string]string{"Host": "127.0.0.1:7860"})
	if w.Code != http.StatusOK {
		t.Fatalf("GET /: %d", w.Code)
	}
	body := w.Body.String()
	links := scriptRE.FindAllStringSubmatch(body, -1)
	if len(links) == 0 {
		t.Fatal("the page loads no script")
	}
	for _, m := range links {
		name := m[1]
		r := do(t, h, "GET", "/"+name, map[string]string{"Host": "127.0.0.1:7860"})
		if r.Code != http.StatusOK {
			t.Errorf("GET /%s: %d", name, r.Code)
		}
	}
}
