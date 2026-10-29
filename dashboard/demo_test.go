package dashboard

import (
	"io/fs"
	"strings"
	"testing"
)

// The demo mode of the dashboard serves a captured host from a sandbox
// under the user cache and a fake serving run, so that the whole page
// can be tried on a machine with no GPU (ia.md §5 R38). The checks
// below read the source of the page: the page has no build step, so
// the words and the structure of the files are the contract.

func TestTheDemoBannerIsOnThePageAndFollowsTheHost(t *testing.T) {
	src, err := fs.ReadFile(webFS, "web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(src)
	if !strings.Contains(html, `id="demo-banner"`) {
		t.Error("the page has no demo banner")
	}
	if !strings.Contains(html, "Demo: nothing here is real.") {
		t.Error("the banner does not say it is a demo")
	}
	src, err = fs.ReadFile(webFS, "web/js/shell.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "function demoBanner(hw)") {
		t.Error("the shell has no demoBanner function")
	}
	src, err = fs.ReadFile(webFS, "web/js/boot.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "loadHW().then(demoBanner)") {
		t.Error("the boot does not call the demo banner when the host is read")
	}
}
