package tools

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/eino/compose"
)

func TestTimeoutMiddlewareAddsDeadline(t *testing.T) {
	var hasDeadline bool
	var until time.Duration
	endpoint := func(ctx context.Context, _ *compose.ToolInput) (*compose.ToolOutput, error) {
		deadline, ok := ctx.Deadline()
		hasDeadline = ok
		if ok {
			until = time.Until(deadline)
		}
		return &compose.ToolOutput{Result: "ok"}, nil
	}
	wrapped := TimeoutMiddleware(time.Second).Invokable(endpoint)
	if _, err := wrapped(context.Background(), &compose.ToolInput{}); err != nil {
		t.Fatal(err)
	}
	if !hasDeadline {
		t.Fatal("middleware did not add a deadline")
	}
	if until <= 0 || until > time.Second {
		t.Fatalf("unexpected deadline distance: %v", until)
	}
}

func TestTimeoutMiddlewareKeepsEarlierRunDeadline(t *testing.T) {
	blocking := func(ctx context.Context, _ *compose.ToolInput) (*compose.ToolOutput, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	wrapped := TimeoutMiddleware(10 * time.Second).Invokable(blocking)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := wrapped(ctx, &compose.ToolInput{}); err == nil {
		t.Fatal("expected the run deadline to interrupt the tool")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("earlier run deadline was not honored, blocked %v", elapsed)
	}
}
