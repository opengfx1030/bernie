package dashcmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/opengfx1030/bernie/internal/app/dashsvc"
	"github.com/opengfx1030/bernie/internal/cli"
)

// runStatus implements `bernie dash status`: print the service state, the
// URLs and (when --show-token is set or the token file is readable by the
// current user) the token. The token is redacted by default, so a process
// that lacks read access to the 0600 token file never sees it.
func runStatus(c *cli.Context, args []string) error {
	fs := c.Flags(cmdDash)
	showToken := fs.Bool("show-token", false, "reveal the bearer token; defaults to redacting it")
	_, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if len(args) > 0 && !isFlag(args[0]) {
		return cli.Usagef("dash status takes no positional argument (got %q)", args[0])
	}
	st, err := dashsvc.GetStatus()
	if err != nil {
		return err
	}
	tok := redactedToken
	if *showToken {
		// Reveal only when the operator (or root) owns the token file.
		if t, _, err := dashsvc.ReadToken(); err == nil {
			tok = t
		}
	}
	st.Token = tok
	return c.Emit(map[string]any{
		"state":        st.State,
		"pid":          st.PID,
		"listen":       st.Listen,
		"loopback_url": st.LoopbackURL,
		"lan_urls":     st.LANURLs,
		"hostname":     st.Hostname,
		"manager":      st.Manager,
		"started_at":   st.StartedAt,
		"version":      st.Version,
		"token":        tok,
		"token_path":   st.TokenPath,
		"log_path":     st.LogPath,
	}, func(w io.Writer) {
		switch st.State {
		case "running":
			fmt.Fprintf(w, "bernie dash: running (pid %d, manager %s)\n", st.PID, st.Manager)
			if st.Listen != "" {
				fmt.Fprintf(w, "  listen:    %s\n", st.Listen)
			}
			if st.LoopbackURL != "" {
				url := st.LoopbackURL
				if tok != "" && tok != redactedToken {
					url += "?token=" + tok
				}
				fmt.Fprintf(w, "  open:      %s\n", url)
			}
			for _, u := range st.LANURLs {
				lan := u
				if tok != "" && tok != redactedToken {
					lan += "?token=" + tok
				}
				fmt.Fprintf(w, "  lan:       %s\n", lan)
			}
			if hn, _ := os.Hostname(); hn != "" && len(st.LANURLs) == 0 && st.LoopbackURL != "" {
				fmt.Fprintf(w, "  ssh -L:    ssh -L 7860:%s %s\n", stripScheme(st.LoopbackURL), hn)
			}
			if !st.StartedAt.IsZero() {
				fmt.Fprintf(w, "  since:     %s\n", st.StartedAt.Format("2006-01-02 15:04:05 MST"))
			}
			if st.TokenPath != "" {
				fmt.Fprintf(w, "  token:     %s (mode 0600)\n", st.TokenPath)
			}
			if st.LogPath != "" {
				fmt.Fprintf(w, "  log:       %s\n", st.LogPath)
			}
			if tok == redactedToken {
				fmt.Fprintln(w, "  token in URL: redacted; pass --show-token when you own the token file")
			}
		case "stale":
			fmt.Fprintf(w, "bernie dash: stale pidfile (pid %d is gone); run `bernie dash stop` to clear\n", st.PID)
		default:
			fmt.Fprintln(w, "bernie dash: not running")
			fmt.Fprintln(w, "  start: bernie dash start [--listen ADDR]")
		}
	})
}

// redactedToken is the placeholder used when the token is not revealed.
const redactedToken = "<redacted>"

// stripScheme removes the "http://" prefix from a URL.
func stripScheme(u string) string {
	if strings.HasPrefix(u, "http://") {
		return u[len("http://"):]
	}
	if strings.HasPrefix(u, "https://") {
		return u[len("https://"):]
	}
	return u
}
