// Package cli is the command router. Each feature registers one top-level
// command from its own cmd_<name>.go file via Register in an init function,
// so features never edit a shared file.
//
// Output contract (agents and scripts rely on it):
//   - `--json` anywhere on the command line switches every command to a single
//     JSON document on stdout; human text otherwise.
//   - Diagnostics and progress go to stderr, never stdout.
//   - Exit codes: 0 ok, 1 runtime failure, 2 usage error, 3 checks failed.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
)

// Exit codes.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
	ExitChecks  = 3
)

// Command is one top-level or nested subcommand.
type Command struct {
	Name    string
	Summary string
	// Usage is the argument synopsis shown after the command path, e.g.
	// "<recipe> [--set KEY=VALUE]...".
	Usage string
	// Run executes the command. Nil when the command only groups Subs.
	Run  func(c *Context, args []string) error
	Subs []*Command
}

// Context carries per-invocation state into commands.
type Context struct {
	Ctx  context.Context
	Out  io.Writer
	Err  io.Writer
	JSON bool
	// Path is the command path, e.g. "bernie recipe show".
	Path string
}

// ExitError carries a specific exit code out of a command.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Usagef reports a usage error (exit 2).
func Usagef(format string, a ...any) error {
	return &ExitError{Code: ExitUsage, Err: fmt.Errorf(format, a...)}
}

// ChecksFailed reports failed checks (exit 3) after the report was printed.
func ChecksFailed(format string, a ...any) error {
	return &ExitError{Code: ExitChecks, Err: fmt.Errorf(format, a...)}
}

var registry = map[string]*Command{}

// Register adds a top-level command. Call from init().
func Register(c *Command) {
	if _, dup := registry[c.Name]; dup {
		panic("cli: duplicate command " + c.Name)
	}
	registry[c.Name] = c
}

// Main runs the CLI and returns the process exit code.
func Main(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	jsonOut := false
	rest := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
			continue
		}
		rest = append(rest, a)
	}
	c := &Context{Ctx: ctx, Out: os.Stdout, Err: os.Stderr, JSON: jsonOut, Path: "bernie"}
	// The product surface is "run the binary": no arguments means status.
	if len(rest) == 0 && registry["status"] != nil {
		rest = []string{"status"}
	}
	root := &Command{Name: "bernie", Subs: topLevel()}
	err := dispatch(c, root, rest)
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, flag.ErrHelp) {
		return ExitOK
	}
	code := ExitFailure
	var ee *ExitError
	if errors.As(err, &ee) {
		code = ee.Code
	}
	switch {
	case err.Error() == "":
		// The command already explained itself.
	case c.JSON && code != ExitChecks:
		enc := json.NewEncoder(c.Out)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(map[string]any{"error": err.Error(), "exit_code": code})
	default:
		fmt.Fprintf(c.Err, "bernie: %v\n", err)
	}
	return code
}

func topLevel() []*Command {
	out := make([]*Command, 0, len(registry))
	for _, c := range registry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func dispatch(c *Context, cmd *Command, args []string) error {
	if len(cmd.Subs) > 0 {
		if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
			if cmd.Run != nil && (len(args) == 0 || args[0] != "help") {
				return cmd.Run(c, args)
			}
			printHelp(c.Err, c.Path, cmd)
			if len(args) == 0 {
				return Usagef("missing subcommand")
			}
			return flag.ErrHelp
		}
		for _, s := range cmd.Subs {
			if s.Name == args[0] {
				sub := *c
				sub.Path = c.Path + " " + s.Name
				return dispatch(&sub, s, args[1:])
			}
		}
		if cmd.Run == nil {
			printHelp(c.Err, c.Path, cmd)
			return Usagef("unknown command %q", args[0])
		}
	}
	if cmd.Run == nil {
		printHelp(c.Err, c.Path, cmd)
		return Usagef("missing subcommand")
	}
	return cmd.Run(c, args)
}

func printHelp(w io.Writer, path string, cmd *Command) {
	if cmd.Summary != "" {
		fmt.Fprintf(w, "%s\n\n", cmd.Summary)
	}
	fmt.Fprintf(w, "usage: %s <command> [args] [--json]\n\ncommands:\n", path)
	for _, s := range cmd.Subs {
		fmt.Fprintf(w, "  %-12s %s\n", s.Name, s.Summary)
	}
}

// Flags returns a FlagSet for a leaf command that prints a consistent usage
// line. Parse errors surface as usage errors.
func (c *Context) Flags(cmd *Command) *flag.FlagSet {
	fs := flag.NewFlagSet(c.Path, flag.ContinueOnError)
	fs.SetOutput(c.Err)
	fs.Usage = func() {
		if cmd.Summary != "" {
			fmt.Fprintf(c.Err, "%s\n\n", cmd.Summary)
		}
		fmt.Fprintf(c.Err, "usage: %s %s [--json]\n", c.Path, cmd.Usage)
		var any bool
		fs.VisitAll(func(*flag.Flag) { any = true })
		if any {
			fmt.Fprintln(c.Err, "\nflags:")
			fs.PrintDefaults()
		}
	}
	return fs
}

// Parse parses flags, allowing flags and positionals to interleave
// (`serve flashnext --set A=B` and `serve --set A=B flashnext` both work).
func Parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, &ExitError{Code: ExitUsage, Err: err}
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		if args[0] == "--" {
			return append(pos, args[1:]...), nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// Emit writes v as JSON when --json is set, otherwise calls human.
func (c *Context) Emit(v any, human func(w io.Writer)) error {
	if c.JSON {
		enc := json.NewEncoder(c.Out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	human(c.Out)
	return nil
}

// Logf writes a progress/diagnostic line to stderr.
func (c *Context) Logf(format string, a ...any) {
	fmt.Fprintf(c.Err, format+"\n", a...)
}

// MultiFlag is a repeatable string flag (e.g. --set A=B --set C=D).
type MultiFlag []string

func (m *MultiFlag) String() string     { return strings.Join(*m, ",") }
func (m *MultiFlag) Set(v string) error { *m = append(*m, v); return nil }

// OrDash renders an empty string as "-" in human tables.
func OrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
