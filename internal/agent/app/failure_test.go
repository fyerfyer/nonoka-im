package app

import "testing"

func TestPublicFailureMessage(t *testing.T) {
	if got := PublicFailureMessage("", ""); got != "" {
		t.Fatalf("empty code should not produce a failure message, got %q", got)
	}
	if got := PublicFailureMessage("cancelled", ""); got != "The agent run was canceled." {
		t.Fatalf("cancelled = %q", got)
	}
	if got := PublicFailureMessage("model_error", "provider stack trace"); got == "provider stack trace" || got == "" {
		t.Fatalf("model error leaked or was empty: %q", got)
	}
	if got := PublicFailureMessage("budget_exceeded", "daily budget exhausted"); got != "daily budget exhausted" {
		t.Fatalf("quota message should be preserved, got %q", got)
	}
}
