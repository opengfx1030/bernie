package dashcmd

import "testing"

func TestSamePort(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"127.0.0.1:7860", "127.0.0.1:7860", true},
		{"0.0.0.0:7860", "127.0.0.1:7860", true}, // LAN service vs loopback foreground
		{":7860", "127.0.0.1:7860", true},
		{"127.0.0.1:7861", "127.0.0.1:7860", false},
		{"127.0.0.1:7860", "127.0.0.1:7861", false},
		{"not-an-address", "127.0.0.1:7860", false},
		{"127.0.0.1:7860", "not-an-address", false},
		{"", "127.0.0.1:7860", false},
	}
	for _, c := range cases {
		if got := samePort(c.a, c.b); got != c.want {
			t.Errorf("samePort(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
