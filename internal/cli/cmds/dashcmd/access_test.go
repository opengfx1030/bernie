package dashcmd

import (
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/dashboard"
)

// TestAccessLines: a loopback dashboard tells the operator how to tunnel
// it (the port stays closed), a network dashboard lists the LAN URLs and
// says the token is the only protection.
func TestAccessLines(t *testing.T) {
	cases := []struct {
		name   string
		listen string
		token  string
		want   []string
		absent []string
	}{
		{
			name:   "loopback gives the tunnel",
			listen: "127.0.0.1:7860",
			want:   []string{"ssh -L 7860:127.0.0.1:7860"},
			absent: []string{"lan:"},
		},
		{
			name:   "the network gives the LAN URLs and the warning",
			listen: "0.0.0.0:7860",
			token:  "abc123",
			want:   []string{"can drive this host", "?token=abc123"},
			absent: []string{"ssh -L"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := strings.Join(accessLines(c.listen, c.token), "\n")
			for _, w := range c.want {
				if !strings.Contains(lines, w) {
					t.Errorf("lines = %q, want %q", lines, w)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(lines, a) {
					t.Errorf("lines = %q, want no %q", lines, a)
				}
			}
		})
	}
}

// TestResolveListen: the default stays on loopback; --host opens the
// network; an explicit address always wins.
func TestResolveListen(t *testing.T) {
	cases := []struct {
		listen string
		host   bool
		want   string
	}{
		{"", false, dashboard.DefaultListen},
		{"", true, dashboard.HostListen},
		{"127.0.0.1:9999", false, "127.0.0.1:9999"},
		{"127.0.0.1:9999", true, "127.0.0.1:9999"},
	}
	for _, c := range cases {
		if got := dashboard.ResolveListen(c.listen, c.host); got != c.want {
			t.Errorf("ResolveListen(%q, %v) = %q, want %q", c.listen, c.host, got, c.want)
		}
	}
	if dashboard.DefaultListen == dashboard.HostListen {
		t.Error("the default and the host address are the same: the default would open the network")
	}
}
