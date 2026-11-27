package httpapi

import (
	"context"
	"fmt"
	"sync"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// TestWithRequestIDConcurrent guards against the shared Metadata map race that
// crashed the gateway under concurrent HTTP traffic. Run with -race.
func TestWithRequestIDConcurrent(t *testing.T) {
	shared := kerrors.Forbidden("FORBIDDEN", "access denied")
	shared.Metadata = map[string]string{"reason": "policy"}

	const workers = 64
	const iterations = 200

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				id := fmt.Sprintf("req-%d-%d", worker, i)
				clone := withRequestID(shared, id)
				if clone == shared {
					t.Errorf("withRequestID returned the shared error")
					return
				}
				if got := clone.Metadata["requestId"]; got != id {
					t.Errorf("requestId = %q, want %q", got, id)
					return
				}
				if got := clone.Metadata["reason"]; got != "policy" {
					t.Errorf("reason = %q, want policy", got)
					return
				}
			}
		}(worker)
	}
	wg.Wait()

	if _, ok := shared.Metadata["requestId"]; ok {
		t.Fatalf("shared error metadata was mutated: %v", shared.Metadata)
	}
}

func TestAsErrorDoesNotMutateSharedError(t *testing.T) {
	shared := &Error{Status: 403, Code: "FORBIDDEN", Message: "denied"}
	ctx := WithRequestID(context.Background(), "req-1")

	got := AsError(ctx, shared)
	if got == shared {
		t.Fatalf("AsError returned the shared error")
	}
	if got.RequestID != "req-1" {
		t.Fatalf("request id = %q, want req-1", got.RequestID)
	}
	if shared.RequestID != "" {
		t.Fatalf("shared error was mutated: %+v", shared)
	}
}
