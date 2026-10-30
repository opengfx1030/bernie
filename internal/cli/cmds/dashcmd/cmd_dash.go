package dashcmd

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/opengfx1030/bernie/dashboard"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/app/dashsvc"
	"github.com/opengfx1030/bernie/internal/cli"
)

// init registers `bernie dash` and dispatches its subcommands. The same
// `dash` command serves four forms:
//
//	bernie dash                        foreground (legacy; spec §3.2)
//	bernie dash start [--listen ...]   spawn the dashboard as a service
//	bernie dash stop                   stop the service
//	bernie dash status [--show-token]  print URLs and state
//	bernie dash --child                internal: run as a service child
//
// One file per command, per the project rules. This file is the
// dispatcher; the subcommand bodies live in cmd_dash_start.go,
// cmd_dash_stop.go and cmd_dash_status.go. The dash child is an internal
// mode and shares this file with the foreground case.
func init() {
	cmd := &cli.Command{
		Name:    "dash",
		Summary: "Start, stop or check the web dashboard",
		Usage: "[start|stop|status] [flags]\n" +
			"\n" +
			"  bernie dash                       (foreground, Ctrl-C to stop)\n" +
			"  bernie dash start [--listen ADDR] [--token T] [--rotate-token]\n" +
			"  bernie dash stop\n" +
			"  bernie dash status [--show-token]\n" +
			"\n" +
			"  --child is internal: the parent `dash start` spawns a detached\n" +
			"  bernie process that runs the dashboard as a service. End users\n" +
			"  never pass --child.",
	}
	cmd.Run = func(c *cli.Context, args []string) error {
		// Internal flag: the parent (dash start) spawned us as a
		// service child. Run the dashboard until ctx ends; do not
		// parse subcommands.
		for _, a := range args {
			if a == "--child" {
				return runChildMode(c, args)
			}
		}
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return runForeground(c, args)
		}
		switch args[0] {
		case "start":
			return runStart(c, args[1:])
		case "stop":
			return runStop(c, args[1:])
		case "status":
			return runStatus(c, args[1:])
		case "help", "-h", "--help":
			return cli.Usagef("use --help for the dash usage")
		default:
			return cli.Usagef("unknown dash subcommand %q (want start, stop or status)", args[0])
		}
	}
	cli.Register(cmd)
}

// cmdDash is the command record. The foreground runner reads the flag set
// from it via c.Flags; we expose the struct so the subcommand files can
// build on it.
var cmdDash = &cli.Command{
	Name:  "dash",
	Usage: "[start|stop|status] [flags]",
}

// runForeground is the legacy `bernie dash` mode: serve until Ctrl-C. The
// service form is `bernie dash start`; this stays for backwards
// compatibility with the existing CLI users.
func runForeground(c *cli.Context, args []string) error {
	fs := c.Flags(cmdDash)
	listen := fs.String("listen", dashboard.DefaultListen, "listen address; a non-loopback address requires the token")
	token := fs.String("token", "", "access token (default $BERNIE_DASH_TOKEN; random when the address is not loopback)")
	demo := fs.Bool("demo", false, "serve the demo: a captured host and a fake run, with no GPU")
	_, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if *token == "" {
		*token = os.Getenv("BERNIE_DASH_TOKEN")
	}
	if *token == "" && !dashboard.IsLoopback(*listen) {
		*token = dashboard.NewToken()
	}
	// A dashboard service already owns this address: surface it instead of
	// failing to bind. Compare the port so 0.0.0.0:7860 and 127.0.0.1:7860
	// both match a service listening on the same port.
	if st, err := dashsvc.GetStatus(); err == nil && st.State == "running" && samePort(st.Listen, *listen) {
		url := st.LoopbackURL
		if tok, ok := dashsvc.ReadableToken(); ok && tok != "" {
			url += "?token=" + tok
		}
		if url == "" {
			url = "http://" + *listen + "/"
		}
		return c.Emit(map[string]any{"url": url, "listen": st.Listen, "already_running": true, "pid": st.PID},
			func(w io.Writer) {
				fmt.Fprintf(w, "bernie dashboard: %s\n", url)
				fmt.Fprintf(w, "It is already running (pid %d). Stop it with `bernie dash stop`.\n", st.PID)
			})
	}
	if *demo {
		if os.Getenv(app.DemoEnv) == "" {
			env, err := app.PrepareDemo()
			if err != nil {
				return err
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			fmt.Fprintln(c.Err, "bernie demo: the captured host is unpacked under "+app.DemoDir()+".")
			return syscall.Exec(exe, os.Args, append(os.Environ(), env...))
		}
		d, err := app.StartDemo(c.Ctx)
		if err != nil {
			return err
		}
		defer d.Stop()
	}
	return dashboard.Serve(c.Ctx, dashboard.Options{
		Listen:  *listen,
		Token:   *token,
		Log:     c.Err,
		Version: cli.Version,
		Ready: func(url string) {
			_ = c.Emit(map[string]any{"url": url, "listen": *listen, "token_required": *token != "", "demo": *demo}, func(w io.Writer) {
				fmt.Fprintf(w, "bernie dashboard: %s\n", url)
				if *token != "" {
					fmt.Fprintln(w, "API clients: send the header \"Authorization: Bearer <token>\".")
				}
				if *demo {
					fmt.Fprintln(w, "Demo: nothing here is real.")
				}
				fmt.Fprintln(w, "Press Ctrl-C to stop.")
			})
		},
	})
}

// samePort reports whether two listen addresses ("host:port" or ":port")
// resolve to the same TCP port. An unparsable address returns false.
func samePort(a, b string) bool {
	_, pa, err := net.SplitHostPort(a)
	if err != nil {
		return false
	}
	_, pb, err := net.SplitHostPort(b)
	if err != nil {
		return false
	}
	return pa != "" && pa == pb
}

// runChildMode is `bernie dash --child`: serve as the detached service
// child that `bernie dash start` spawned. The PID file and the startup
// document are written from here.
func runChildMode(c *cli.Context, args []string) error {
	listen := dashboard.DefaultListen
	token := ""
	for i, a := range args {
		switch a {
		case "--listen":
			if i+1 < len(args) {
				listen = args[i+1]
			}
		case "--token":
			if i+1 < len(args) {
				token = args[i+1]
			}
		}
	}
	// Translate SIGTERM/SIGINT to a context cancel so `dash stop` and
	// Ctrl-C can stop the child cleanly. The child has its own context;
	// the parent's CancelFunc does not reach here.
	ctx, cancel := signal.NotifyContext(c.Ctx, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	defer dashsvc.RunChildCleanup()
	if err := dashsvc.RunChild(ctx, dashsvc.ChildOptions{
		Listen:  listen,
		Token:   token,
		Version: cli.Version,
	}); err != nil {
		return err
	}
	return dashboard.Serve(ctx, dashboard.Options{
		Listen:  listen,
		Token:   token,
		Log:     c.Err,
		Version: cli.Version,
		Ready: func(url string) {
			// The parent polls for this document to learn the URL the
			// dashboard bound to.
			_ = dashsvc.WriteChildStartup(listen, url, cli.Version)
		},
	})
}
