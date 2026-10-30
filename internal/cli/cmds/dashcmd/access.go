package dashcmd

import (
	"fmt"
	"net"
	"os"

	"github.com/opengfx1030/bernie/dashboard"
	"github.com/opengfx1030/bernie/internal/app/dashsvc"
)

// accessLines tells the operator how to reach the dashboard from another
// machine. Two cases:
//
//   - The dashboard listens on the network: the LAN URLs of this host,
//     each with the token, because a non-loopback address runs in token
//     mode. Anyone with the URL can drive the host, so the line says so.
//   - The dashboard listens on loopback (the default): the port stays
//     closed on the host and the operator tunnels it over SSH. That is
//     the safe remote path: the token is not needed and nothing on the
//     network can reach the dashboard.
func accessLines(listen, token string) []string {
	if !dashboard.IsLoopback(listen) {
		out := []string{"  lan:        anyone with this URL can drive this host. Keep it private."}
		for _, u := range dashsvc.LANURLs(listen) {
			out = append(out, "  lan:        "+withToken(u, token))
		}
		return out
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return nil
	}
	hn, _ := os.Hostname()
	if hn == "" {
		hn = "localhost"
	}
	return []string{
		fmt.Sprintf("  ssh -L:     ssh -L %s:%s %s   (then open http://127.0.0.1:%s/)", port, listen, hn, port),
	}
}

// withToken appends the token to a URL that has none.
func withToken(url, token string) string {
	if token == "" || url == "" {
		return url
	}
	if len(url) > 0 && url[len(url)-1] == '/' {
		return url + "?token=" + token
	}
	return url + "/?token=" + token
}
