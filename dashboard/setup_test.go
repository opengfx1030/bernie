package dashboard

import (
	"io/fs"
	"strings"
	"testing"
)

// The Setup tab has three words and one block that the reference's reader
// knows (ia.md §5 R39, R40, R42). The checks below read the source of the
// tab: the page has no build step and no JS test runner, so the wording
// and the structure of the file are the contract.

// The blocked state says "A FEW THINGS FIRST." and names the next move.
func TestTheBlockedSetupStateHasItsHeadlineAndItsLede(t *testing.T) {
	src, err := fs.ReadFile(webFS, "web/views/setup/headline.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "A FEW THINGS FIRST.") {
		t.Error("the blocked state has no headline")
	}
	if !strings.Contains(text, "Fix the items below first, then press Re-check.") {
		t.Error("the blocked state does not tell the reader what to do next")
	}
	// The blocked branch must come before the generic "steps left" one, or
	// the reader never sees it.
	blocked := strings.Index(text, "A FEW THINGS FIRST.")
	generic := strings.Index(text, "STEPS LEFT")
	if blocked < 0 || generic < 0 || blocked > generic {
		t.Errorf("the blocked headline comes at %d, the generic one at %d", blocked, generic)
	}
}

// The "Needs you" block holds the items only the operator can do: a
// blocked step and a step with a root command.
func TestTheNeedsYouBlockHoldsTheOperatorItems(t *testing.T) {
	src, err := fs.ReadFile(webFS, "web/views/setup/needs-you.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "Needs you") {
		t.Error("the section has no name")
	}
	if !strings.Contains(text, `s.state === "blocked"`) {
		t.Error("the section does not hold the blocked steps")
	}
	if !strings.Contains(text, "root_commands") {
		t.Error("the section does not hold the steps that need root")
	}
	if !strings.Contains(text, "Copy the command") {
		t.Error("the section carries no copy button")
	}
	if !strings.Contains(text, "Re-check") {
		t.Error("the section has no Re-check")
	}
}

// The launch command and the compose file live in one disclosure, and the
// compose file only in container mode.
func TestTheLaunchDisclosureShowsTheComposeFileOnlyInContainerMode(t *testing.T) {
	src, err := fs.ReadFile(webFS, "web/views/setup/launch-disclosure.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "Show the exact launch command") {
		t.Error("the disclosure has no name")
	}
	if !strings.Contains(text, "The generated compose.yml") {
		t.Error("the disclosure holds no compose file")
	}
	// The compose file is behind the container-mode test.
	if !strings.Contains(text, `plan.mode !== "container"`) {
		t.Error("the compose file is not behind the mode test")
	}
	if !strings.Contains(text, "plan.compose") {
		t.Error("the disclosure does not read the compose field of the plan")
	}
	// The host command is always there.
	if !strings.Contains(text, "setupPlanCommand(plan)") {
		t.Error("the disclosure holds no launch command")
	}
}

// The python-env detail of the setup path comes from the venv.verify
// step of the run record (spec §6.3). The page reads the same JSON the
// CLI prints and the MCP server returns, so the three surfaces cannot
// disagree.
func TestSetupPathShowsPythonEnvFromTheRunRecord(t *testing.T) {
	src, err := fs.ReadFile(webFS, "web/views/setup/path.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, want := range []string{
		"setupPythonEnv",
		"venv.verify",
		"evidence.components",
		// The table is absent when the run has no venv.verify step or no
		// components in its evidence.
		"if (!step || !step.evidence || !step.evidence.components) return null",
		// The table is rendered with the path paint: the path module owns
		// the section, the steps module owns the load.
		"setupPaintPath",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("setup/path.js does not carry %q", want)
		}
	}
}
