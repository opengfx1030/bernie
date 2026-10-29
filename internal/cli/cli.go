// Package cli is the command router. Each feature registers one top-level
// command from its own cmd_<name>.go file via Register in an init function,
// so features never edit a shared file.
//
// Output contract (agents and scripts rely on it):
//   - `--json` anywhere on the command line switches every command to a single
//     JSON document on stdout; human text otherwise.
//   - Diagnostics and progress go to stderr, never stdout. The exception is
//     explicit help (`--help`, `-h`, `help`): it is the requested output, so it
//     goes to stdout and exits 0. Usage errors print their message once on
//     stderr (as `bernie: ...`).
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
	"strconv"
	"strings"
	"syscall"

	"github.com/opengfx1030/bernie/internal/support/hostroot"
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
	// Venv is the global --venv value. It may be set before or after the
	// command.
	Venv string
	// Root is the global --root value: the root of the host tree bernie
	// reads. Empty means the real host ("/").
	Root string
	// Path is the command path, e.g. "bernie recipe show".
	Path string
	// root is the Context Main created; a --json parsed by a command's own
	// FlagSet (see splitJSON) is mirrored there so the error envelope matches.
	root *Context
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
	// The product surface is "run the binary": no arguments means status.
	if len(args) == 0 && registry["status"] != nil {
		args = []string{"status"}
	}
	return run(ctx, os.Stdout, os.Stderr, &Command{Name: "bernie", Subs: topLevel()}, args)
}

// splitJSON removes the global --json flag from args. Rule: scanning stops at
// "--" (everything after is positional and kept verbatim), and a --json that
// directly follows a flag-like token without "=" (-x, --name) is left alone.
// That token is either a boolean flag, so --json is still the global flag, or
// a value flag whose value is literally "--json"; only the command's own
// FlagSet knows which, and Context.Flags registers a boolean json flag there to
// settle it.
func splitJSON(args []string) (rest []string, jsonOut bool) {
	rest = make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			return append(rest, args[i:]...), jsonOut
		}
		if a == "--json" && !(i > 0 && strings.HasPrefix(args[i-1], "-") && args[i-1] != "--json" && !strings.Contains(args[i-1], "=")) {
			jsonOut = true
			continue
		}
		rest = append(rest, a)
	}
	return rest, jsonOut
}

func run(ctx context.Context, out, errw io.Writer, root *Command, args []string) int {
	rest, jsonOut := splitJSON(args)
	c := &Context{Ctx: ctx, Out: out, Err: errw, JSON: jsonOut, Path: "bernie"}
	c.root = c
	err := dispatch(c, root, rest)
	if err == nil {
		return ExitOK
	}
	var he *helpError
	switch {
	case errors.As(err, &he):
		he.fs.SetOutput(out)
		he.fs.Usage()
		return ExitOK
	case errors.Is(err, flag.ErrHelp):
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
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(map[string]any{"error": err.Error(), "exit_code": code})
	default:
		fmt.Fprintf(errw, "bernie: %v\n", err)
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

// takeVenv removes a leading global --venv flag from a group command.
func takeVenv(args []string) (rest []string, venv string, err error) {
	if len(args) == 0 {
		return args, "", nil
	}
	if args[0] == "--venv" {
		if len(args) < 2 || args[1] == "" {
			return nil, "", Usagef("--venv needs a directory")
		}
		return args[2:], args[1], nil
	}
	if v, ok := strings.CutPrefix(args[0], "--venv="); ok {
		if v == "" {
			return nil, "", Usagef("--venv needs a directory")
		}
		return args[1:], v, nil
	}
	return args, "", nil
}

// takeRoot removes a leading global --root flag from a group command.
func takeRoot(args []string) (rest []string, root string, err error) {
	if len(args) == 0 {
		return args, "", nil
	}
	if args[0] == "--root" {
		if len(args) < 2 || args[1] == "" {
			return nil, "", Usagef("--root needs a directory")
		}
		return args[2:], args[1], nil
	}
	if v, ok := strings.CutPrefix(args[0], "--root="); ok {
		if v == "" {
			return nil, "", Usagef("--root needs a directory")
		}
		return args[1:], v, nil
	}
	return args, "", nil
}

func dispatch(c *Context, cmd *Command, args []string) error {
	if len(cmd.Subs) > 0 {
		rest, venv, err := takeVenv(args)
		if err != nil {
			return err
		}
		if venv != "" {
			c.Venv = venv
			args = rest
		}
		rest, root, err := takeRoot(args)
		if err != nil {
			return err
		}
		if root != "" {
			c.Root = root
			hostroot.Set(root)
			args = rest
		}
		if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
			if cmd.Run != nil && (len(args) == 0 || args[0] != "help") {
				return cmd.Run(c, args)
			}
			if len(args) == 0 {
				printHelp(c.Err, c.Path, cmd)
				return Usagef("missing subcommand")
			}
			printHelp(c.Out, c.Path, cmd)
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

// Flags returns a FlagSet for a leaf command with a consistent usage text. It
// also defines the boolean --json flag (see splitJSON). The FlagSet prints
// nothing itself: Parse returns the error, and Main prints a bad flag once on
// stderr or, for -h/--help, the usage on stdout.
func (c *Context) Flags(cmd *Command) *flag.FlagSet {
	fs := flag.NewFlagSet(c.Path, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var(jsonFlag{c}, "json", "print one JSON document on stdout")
	fs.Var(venvFlag{c}, "venv", "host-mode venv directory")
	fs.Var(rootFlag{c}, "root", "root of the host tree to read (default /)")
	fs.Usage = func() {
		w := fs.Output()
		if cmd.Summary != "" {
			fmt.Fprintf(w, "%s\n\n", cmd.Summary)
		}
		fmt.Fprintf(w, "usage: %s %s [--json]\n\nflags:\n", c.Path, cmd.Usage)
		fs.PrintDefaults()
	}
	return fs
}

// jsonFlag is the boolean --json flag of a command's FlagSet.
type jsonFlag struct{ c *Context }

func (jsonFlag) String() string   { return "" }
func (jsonFlag) IsBoolFlag() bool { return true }
func (j jsonFlag) Set(v string) error {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return err
	}
	j.c.JSON = b
	if j.c.root != nil {
		j.c.root.JSON = b
	}
	return nil
}

// venvFlag is the global --venv flag.
type venvFlag struct{ c *Context }

func (venvFlag) String() string   { return "" }
func (venvFlag) IsBoolFlag() bool { return false }
func (v venvFlag) Set(s string) error {
	if s == "" {
		return errors.New("venv path must not be empty")
	}
	v.c.Venv = s
	return nil
}

// rootFlag is the global --root flag: the root of the host tree bernie
// reads. It points bernie at a captured host snapshot.
type rootFlag struct{ c *Context }

func (rootFlag) String() string   { return "" }
func (rootFlag) IsBoolFlag() bool { return false }
func (r rootFlag) Set(s string) error {
	if s == "" {
		return errors.New("root path must not be empty")
	}
	r.c.Root = s
	hostroot.Set(s)
	return nil
}

// helpError is flag.ErrHelp from a command's FlagSet; Main prints its usage to
// stdout.
type helpError struct{ fs *flag.FlagSet }

func (*helpError) Error() string { return flag.ErrHelp.Error() }
func (*helpError) Unwrap() error { return flag.ErrHelp }

// Parse parses flags, allowing flags and positionals to interleave
// (`serve flashnext --set A=B` and `serve --set A=B flashnext` both work).
func Parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, &helpError{fs}
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
