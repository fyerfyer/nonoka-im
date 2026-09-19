package server

import "testing"

func TestIsLoopbackOrigin(t *testing.T) {
	tests := map[string]bool{
		"http://localhost:3000":    true,
		"https://localhost:3000":   true,
		"http://127.0.0.1:3000":    true,
		"http://[::1]:3000":        true,
		"https://example.com":      false,
		"http://192.168.1.10:3000": false,
		"not-an-origin":            false,
	}
	for origin, want := range tests {
		if got := isLoopbackOrigin(origin); got != want {
			t.Errorf("isLoopbackOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}
