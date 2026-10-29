package dashcmd

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/opengfx1030/bernie/dashboard"
	"github.com/opengfx1030/bernie/internal/app/dashsvc"
	"github.com/opengfx1030/bernie/internal/cli"
)

// runStart implements `bernie dash start`: spawn the dashboard as a
// detached service and print the URLs when it is ready.
//
// Flags:
//
//	--listen ADDR    bind address (default 127.0.0.1:7860; non-loopback
//	                 implies token mode)
//	--token T        use this exact token instead of the persisted one
//	--rotate-token   replace the persisted token before starting
//	--no-systemd     skip the systemd --user path even when available
//
// Idempotent: a second start returns the running service in
// `dash_status` and exits 1 with a clear message.
func runStart(c *cli.Context, args []string) error {
	fs := c.Flags(cmdDash)
	listen := fs.String("listen", dashboard.DefaultListen, "listen address; a non-loopback address requires the token")
	token := fs.String("token", "", "explicit token (else use or generate the persisted one)")
	rotate := fs.Bool("rotate-token", false, "replace the persisted token before starting")
	noSystemd := fs.Bool("no-systemd", false, "skip the systemd --user path even when it is available")
	_, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	// Reject a positional argument.
	if len(args) > 0 && !isFlag(args[0]) {
		return cli.Usagef("dash start takes no positional argument (got %q)", args[0])
	}
	// Validate the listen address early.
	if _, _, err := net.SplitHostPort(*listen); err != nil {
		return cli.Usagef("invalid --listen %q: %v", *listen, err)
	}
	// Warn on a non-loopback listen without a token.
	if *token == "" && !dashboard.IsLoopback(*listen) {
		fmt.Fprintf(c.Err, "bernie dash: %s is a non-loopback address; a token will be generated and persisted at %s\n", *listen, dashsvc.ServicePaths().Token)
		fmt.Fprintln(c.Err, "bernie dash: anyone with the URL can drive this host. Keep it private.")
	}
	st, err := dashsvc.Start(c.Ctx, dashsvc.StartOptions{
		Listen:      *listen,
		Token:       *token,
		RotateToken: *rotate,
		Version:     cli.Version,
		NoSystemd:   *noSystemd,
	})
	if err != nil {
		if errors.Is(err, dashsvc.ErrAlreadyRunning) {
			return &cli.ExitError{Code: cli.ExitFailure, Err: err}
		}
		return err
	}
	// Print the URLs and the manager in use.
	if dashsvc.SystemdUserAvailable() && !*noSystemd && st.Manager == "systemd" {
		fmt.Fprintln(c.Err, "bernie dash: managed by systemd --user; the dashboard survives logout only after `loginctl enable-linger $USER` (root).")
	}
	return c.Emit(map[string]any{
		"state":         st.State,
		"pid":           st.PID,
		"listen":        st.Listen,
		"loopback_url":  st.LoopbackURL,
		"lan_urls":      st.LANURLs,
		"token":         st.Token,
		"manager":       st.Manager,
		"token_path":    st.TokenPath,
		"log_path":      st.LogPath,
		"started_at":    st.StartedAt,
		"systemd_hint":  "loginctl enable-linger $USER",
	}, func(w io.Writer) {
		fmt.Fprintf(w, "bernie dash: listening on %s\n", st.Listen)
		if st.LoopbackURL != "" {
			url := st.LoopbackURL
			if st.Token != "" {
				url += "?token=" + st.Token
			}
			fmt.Fprintf(w, "  open:       %s\n", url)
		}
		for _, u := range st.LANURLs {
			lan := u
			if st.Token != "" {
				lan += "?token=" + st.Token
			}
			fmt.Fprintf(w, "  lan:        %s\n", lan)
		}
		hn, _ := os.Hostname()
		if hn != "" {
			fmt.Fprintf(w, "  hostname:   %s\n", hn)
		}
		if st.LoopbackURL != "" && len(st.LANURLs) == 0 {
			fmt.Fprintf(w, "  ssh -L:     ssh -L 7860:%s %s\n", stripScheme(st.LoopbackURL), hnOrLocal(hn))
		}
		fmt.Fprintf(w, "  manager:    %s\n", st.Manager)
		if st.Manager == "systemd" {
			fmt.Fprintf(w, "  hint:       loginctl enable-linger $USER  (run as root, then this dash survives logout)\n")
		}
		fmt.Fprintf(w, "  token:      %s (mode 0600)\n", st.TokenPath)
		fmt.Fprintf(w, "  log:        %s\n", st.LogPath)
		fmt.Fprintf(w, "  status:     bernie dash status    stop: bernie dash stop\n")
	})
}

// isFlag reports whether s looks like a flag (starts with `-`). We use it
// to defend the positional check from being fooled by a leftover token.
func isFlag(s string) bool { return len(s) > 1 && s[0] == '-' }

// hnOrLocal returns the hostname, or "localhost" when it is empty.
func hnOrLocal(hn string) string {
	if hn == "" {
		return "localhost"
	}
	return hn
}
