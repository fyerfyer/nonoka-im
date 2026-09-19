package gateway

import "testing"

func TestSessionNodeID_BackwardCompatible(t *testing.T) {
	for value, want := range map[string]string{
		"gateway-a":        "gateway-a",
		"gateway-b|conn-1": "gateway-b",
	} {
		if got := sessionNodeID(value); got != want {
			t.Fatalf("sessionNodeID(%q)=%q, want %q", value, got, want)
		}
	}
}
