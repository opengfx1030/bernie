package app

import (
	"embed"
	"strings"
	"testing"
)

//go:embed demo.go
var demoFS embed.FS

// The demo mode of the dashboard serves a captured host from a sandbox
// under the user cache and a fake serving run, so that the whole page
// can be tried on a machine with no GPU (ia.md §5 R38). The checks
// below read the source of the demo.
func TestTheDemoSandboxLivesUnderTheUserCache(t *testing.T) {
	src, err := demoFS.ReadFile("demo.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, want := range []string{
		"paths.Cache()",
		"BERNIE_DEMO",
		"fixture.Extract",
		"vllmtest.New",
		"runner.Attach",
		"BERNIE_ROOT",
		"BERNIE_STATE_DIR",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the demo holds no %q", want)
		}
	}
	if strings.Contains(text, "/tmp") {
		t.Error("the demo touches /tmp")
	}
}
