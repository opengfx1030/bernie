package health

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/setup"
)

// This file is the report of a setup run: `setup report` prints the record
// of a run and `setup log` prints the log of one of its steps. Both read
// the record the apply function wrote, so the CLI, the MCP server and the
// dashboard show the same run (spec §6.4).

// setupReportCommand returns the `setup report` command: the record of a
// run, with the time of every step and the evidence of the check of the
// venv.
func setupReportCommand() *cli.Command {
	report := &cli.Command{Name: "report", Summary: "The record of a setup run: every step, its time and its log", Usage: "[RUN]"}
	report.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(report)
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) > 1 {
			return cli.Usagef("setup report takes at most one run id")
		}
		run, err := loadRun(first(pos))
		if err != nil {
			return err
		}
		return c.Emit(run, func(w io.Writer) { printRun(w, run) })
	}
	return report
}

// setupLogCommand returns the `setup log` command: the log of one step of
// the newest run.
func setupLogCommand() *cli.Command {
	logCmd := &cli.Command{Name: "log", Summary: "The log of one step of a setup run", Usage: "[STEP] [-f]"}
	logCmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(logCmd)
		follow := fs.Bool("f", false, "keep printing the lines that come")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) > 1 {
			return cli.Usagef("setup log takes at most one step id")
		}
		run, err := loadRun("")
		if err != nil {
			return err
		}
		step := first(pos)
		if step == "" {
			step = lastStepID(run)
		}
		path := stepLogPath(run, step)
		if path == "" {
			return fmt.Errorf("the run %s has no step %q", run.ID, step)
		}
		return tailFile(c.Out, path, *follow)
	}
	return logCmd
}

// first returns the first of the positional arguments, or the empty string.
func first(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// loadRun reads the record of a run: the one with this id, or the newest
// one when the id is empty.
func loadRun(id string) (*setup.Run, error) {
	if id != "" {
		return app.SetupRun(id)
	}
	return app.SetupLatestRun()
}

// lastStepID returns the id of the last step of a run.
func lastStepID(run *setup.Run) string {
	if run == nil || len(run.Steps) == 0 {
		return ""
	}
	return run.Steps[len(run.Steps)-1].ID
}

// stepLogPath returns the log of one step of a run, or the empty string
// when the step has none.
func stepLogPath(run *setup.Run, step string) string {
	if run == nil {
		return ""
	}
	for _, s := range run.Steps {
		if s.ID == step {
			return s.LogPath
		}
	}
	return ""
}

// printRun prints the record of a run: the steps with their state, their
// time and their log, then the evidence of the check of the venv.
func printRun(w io.Writer, run *setup.Run) {
	fmt.Fprintf(w, "run %s  %s", run.ID, run.State)
	if run.Mode != "" {
		fmt.Fprintf(w, "  mode %s", run.Mode)
	}
	if run.Recipe != "" {
		fmt.Fprintf(w, "  recipe %s", run.Recipe)
	}
	if run.Venv != "" {
		fmt.Fprintf(w, "  venv %s", run.Venv)
	}
	fmt.Fprintf(w, "  in %s\n", setup.FormatDuration(run.DurationS))
	if run.Error != "" {
		fmt.Fprintf(w, "error: %s\n", run.Error)
	}
	fmt.Fprintln(w, "\nSTEP\tSTATE\tDURATION\tLOG")
	for _, s := range run.Steps {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, s.State, setup.FormatDuration(s.DurationS), cli.OrDash(s.LogPath))
		if s.Summary != "" && s.Summary != "done" {
			fmt.Fprintf(w, "  %s\n", s.Summary)
		}
	}
	for _, s := range run.Steps {
		if len(s.Evidence) == 0 {
			continue
		}
		fmt.Fprintf(w, "\nevidence of %s:\n", s.ID)
		printEvidence(w, s.Evidence)
	}
}

// printEvidence prints the verification JSON of a step.
func printEvidence(w io.Writer, raw json.RawMessage) {
	var v struct {
		Python       string      `json:"python"`
		Torch        string      `json:"torch"`
		HIP          string      `json:"hip"`
		VLLM         string      `json:"vllm"`
		DeviceCount  int         `json:"device_count"`
		ROCMOps      int         `json:"rocm_ops"`
		RDNAOps      int         `json:"rdna_ops"`
		AllReduce    bool        `json:"all_reduce"`
		ROCMC        string      `json:"rocm_c"`
		ROCBLAS      string      `json:"rocblas"`
		ROCBLASWheel bool        `json:"rocblas_wheel"`
		MissingLibs  []string    `json:"missing_libs"`
		VerifyErrors []string    `json:"verify_errors"`
		Components   []Component `json:"components"`
		Error        string      `json:"error"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		fmt.Fprintf(w, "  %s\n", raw)
		return
	}
	fmt.Fprintf(w, "  python %s\n", cli.OrDash(v.Python))
	fmt.Fprintf(w, "  torch %s, hip %s, %d devices\n", cli.OrDash(v.Torch), cli.OrDash(v.HIP), v.DeviceCount)
	fmt.Fprintf(w, "  vllm %s\n", cli.OrDash(v.VLLM))
	fmt.Fprintf(w, "  ops: %d RDNA of %d, all_reduce %t\n", v.RDNAOps, v.ROCMOps, v.AllReduce)
	fmt.Fprintf(w, "  extension %s\n", cli.OrDash(v.ROCMC))
	fmt.Fprintf(w, "  rocBLAS %s%s\n", cli.OrDash(v.ROCBLAS), wheelNote(v.ROCBLASWheel))
	if len(v.Components) > 0 {
		fmt.Fprintln(w, "  components:")
		for _, c := range v.Components {
			line := "    " + c.Name + " " + cli.OrDash(c.Version)
			if c.Note != "" {
				line += "  (" + c.Note + ")"
			}
			fmt.Fprintln(w, line)
		}
	}
	if len(v.MissingLibs) > 0 {
		fmt.Fprintf(w, "  missing libraries: %s\n", strings.Join(v.MissingLibs, ", "))
	}
	if len(v.VerifyErrors) > 0 {
		fmt.Fprintf(w, "  failed checks: %s\n", strings.Join(v.VerifyErrors, "; "))
	}
	if v.Error != "" {
		fmt.Fprintf(w, "  error: %s\n", v.Error)
	}
}

// Component is one row of the venv components table: the MOST IMPORTANT
// parts of the venv (spec §6.3). A missing part has Version == "absent"
// and is never omitted.
type Component struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Note    string `json:"note,omitempty"`
}

// venvComponents reads the components list out of the venv.verify step
// of a setup run (spec §6.4). It returns nil when the run has no
// venv.verify step or the evidence has no components: the caller shows
// nothing rather than a half-empty table.
func venvComponents(run *setup.Run) []Component {
	if run == nil {
		return nil
	}
	for _, s := range run.Steps {
		if s.ID != "venv.verify" {
			continue
		}
		if len(s.Evidence) == 0 {
			return nil
		}
		var v struct {
			Components []Component `json:"components"`
		}
		if err := json.Unmarshal(s.Evidence, &v); err != nil {
			return nil
		}
		if len(v.Components) == 0 {
			return nil
		}
		return v.Components
	}
	return nil
}

// wheelNote says where the rocBLAS comes from.
func wheelNote(wheel bool) string {
	if wheel {
		return " (from the wheel of the venv)"
	}
	return ""
}

// printPythonEnv prints the python-env detail of a setup run: the MOST
// IMPORTANT parts of the venv (spec §6.3). It reads the same record the
// dashboard and the MCP server read, so the three surfaces cannot
// disagree. A missing component is "absent", never omitted.
func printPythonEnv(w io.Writer, run *setup.Run) {
	comps := venvComponents(run)
	if len(comps) == 0 {
		return
	}
	fmt.Fprintln(w, "python-env:")
	for _, c := range comps {
		line := "  " + c.Name + " " + cli.OrDash(c.Version)
		if c.Note != "" {
			line += "  (" + c.Note + ")"
		}
		fmt.Fprintln(w, line)
	}
}

// tailFile prints a file and, when follow is set, the lines that come
// after it.
func tailFile(w io.Writer, path string, follow bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if !follow {
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// runPrinter prints the progress of a setup run: one line per step, the
// line updated while the step runs.
type runPrinter struct {
	w io.Writer
	// events receives the events of the run when the output is JSON.
	events func(setup.Event)
	mu     sync.Mutex
	// width is the length of the line on the screen: the next line pads
	// it, so the text does not leave pieces behind.
	width int
	// start is when the step that runs now started.
	start time.Time
	// phase is the progress sentence of the step that runs now.
	phase string
	// index and total place the step in the run.
	index, total int
	// current is the id of the step that runs now.
	current string
	// active says that a step runs now.
	active bool
	// now is the clock of the printer. The tests fix it.
	now func() time.Time
}

// clock is the clock of the printer.
func (p *runPrinter) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// event prints one event of a run.
func (p *runPrinter) event(ev setup.Event) {
	if p.events != nil {
		p.events(ev)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch ev.Type {
	case "step_started":
		p.start = ev.At
		p.phase = ""
		p.index, p.total = ev.Index, ev.Total
		if ev.Step != nil {
			p.current = ev.Step.ID
		}
		p.active = true
		p.line(p.running())
	case "progress":
		if !p.active || ev.Step == nil || ev.Step.Progress == nil {
			return
		}
		p.phase = ev.Step.Progress.Sentence
		p.line(p.running())
	case "step_done":
		p.active = false
		p.done(ev)
	case "step_skipped":
		p.line(fmt.Sprintf("       %s: skipped (%s)", ev.Step.ID, ev.Step.Summary))
	case "step_blocked":
		p.line(fmt.Sprintf("       %s: blocked (%s)", ev.Step.ID, ev.Step.Summary))
	}
}

// running is the line of the step that runs now.
func (p *runPrinter) running() string {
	line := fmt.Sprintf("[%d/%d] %s: running... %s", p.index, p.total, p.stepID(), setup.FormatDuration(p.clock().Sub(p.start).Seconds()))
	if p.phase != "" {
		line += " (" + p.phase + ")"
	}
	return line
}

// stepID is the id of the step that runs now.
func (p *runPrinter) stepID() string {
	if p.current == "" {
		return "step"
	}
	return p.current
}

// done prints the last line of a step.
func (p *runPrinter) done(ev setup.Event) {
	if ev.Step == nil {
		return
	}
	took := setup.FormatDuration(ev.Step.DurationS)
	switch ev.Step.State {
	case setup.StepFailed:
		p.line(fmt.Sprintf("[%d/%d] %s: failed in %s: %s, log: %s",
			ev.Index, ev.Total, ev.Step.ID, took, ev.Step.Summary, cli.OrDash(ev.Step.LogPath)))
	default:
		p.line(fmt.Sprintf("[%d/%d] %s: done in %s, log: %s",
			ev.Index, ev.Total, ev.Step.ID, took, cli.OrDash(ev.Step.LogPath)))
	}
}

// line writes a line and pads it, so the next line covers it.
func (p *runPrinter) line(text string) {
	if p.width > len(text) {
		text += strings.Repeat(" ", p.width-len(text))
	}
	fmt.Fprintf(p.w, "\r%s", text)
	p.width = len(text)
}

// finish ends the last line of the run.
func (p *runPrinter) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.width > 0 {
		fmt.Fprintln(p.w)
		p.width = 0
	}
}
