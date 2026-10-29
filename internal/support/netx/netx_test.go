package netx

import "testing"

func TestLoopback(t *testing.T) {
	tests := map[string]bool{
		"127.0.0.1":                   true,
		"127.0.0.2":                   true,
		"localhost":                   true,
		"LOCALHOST":                   true,
		"[::1]":                       true,
		"127.0.0.1:8000":              true,
		"http://127.0.0.1:8000":       true,
		"https://localhost:8000/path": true,
		"":                            false,
		"example.com":                 false,
		"192.168.1.5":                 false,
		"0.0.0.0":                     false,
		"http://192.168.1.5:8000":     false,
		"ftp://127.0.0.1":             false,
		"http://127.0.0.1.evil.test":  false,
	}
	for target, want := range tests {
		if got := Loopback(target); got != want {
			t.Errorf("Loopback(%q) = %v, want %v", target, got, want)
		}
	}
}
