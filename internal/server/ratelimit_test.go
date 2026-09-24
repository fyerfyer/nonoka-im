package server

import (
	"context"
	"testing"
	"time"

	pb "nonoka-im/api/im/v1"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
)

func newTestLimiter(t *testing.T) (*AuthRateLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewAuthRateLimiter(client, log.NewStdLogger(nil)), mr
}

func TestAuthRateLimiter_LoginIPWindow(t *testing.T) {
	l, mr := newTestLimiter(t)
	ctx := context.Background()

	// 10 attempts from one IP pass, the 11th is denied.
	for i := 0; i < loginIPLimit; i++ {
		ok, err := l.Allow(ctx, "/api.im.v1.AuthService/Login", "10.0.0.1", "")
		if err != nil || !ok {
			t.Fatalf("attempt %d: expected allowed, got ok=%v err=%v", i+1, ok, err)
		}
	}
	ok, err := l.Allow(ctx, "/api.im.v1.AuthService/Login", "10.0.0.1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected 11th login attempt from same IP to be denied")
	}

	// Window key uses the documented format with a 1-minute TTL.
	counter, err := mr.Get("im:rl:login:ip:10.0.0.1")
	if err != nil {
		t.Fatalf("counter key missing: %v", err)
	}
	if counter != "11" {
		t.Fatalf("expected counter 11, got %q", counter)
	}
	if ttl := mr.TTL("im:rl:login:ip:10.0.0.1"); ttl > rateLimitWindow {
		t.Fatalf("expected TTL <= 1m, got %v", ttl)
	}

	// A different IP is unaffected.
	ok, err = l.Allow(ctx, "/api.im.v1.AuthService/Login", "10.0.0.2", "alice")
	if err != nil || !ok {
		t.Fatalf("expected different IP to be allowed, got ok=%v err=%v", ok, err)
	}
}

func TestAuthRateLimiter_LoginUserWindow(t *testing.T) {
	l, _ := newTestLimiter(t)
	ctx := context.Background()

	// 5 attempts for one account pass (from distinct IPs to avoid tripping
	// the IP window), the 6th is denied.
	for i := 0; i < loginUserLimit; i++ {
		ok, err := l.Allow(ctx, "/api.im.v1.AuthService/Login", ipFor(i), "victim")
		if err != nil || !ok {
			t.Fatalf("attempt %d: expected allowed, got ok=%v err=%v", i+1, ok, err)
		}
	}
	ok, err := l.Allow(ctx, "/api.im.v1.AuthService/Login", ipFor(loginUserLimit), "victim")
	if err != nil || ok {
		t.Fatalf("expected 6th attempt for same user to be denied, got ok=%v err=%v", ok, err)
	}

	// Other accounts from the same IP are still allowed (per-account limit).
	ok, err = l.Allow(ctx, "/api.im.v1.AuthService/Login", ipFor(0), "other-user")
	if err != nil || !ok {
		t.Fatalf("expected other user to be allowed, got ok=%v err=%v", ok, err)
	}
}

func TestAuthRateLimiter_RegisterIPWindow(t *testing.T) {
	l, _ := newTestLimiter(t)
	ctx := context.Background()

	for i := 0; i < registerIPLimit; i++ {
		ok, err := l.Allow(ctx, "/api.im.v1.AuthService/Register", "10.0.0.1", "")
		if err != nil || !ok {
			t.Fatalf("attempt %d: expected allowed, got ok=%v err=%v", i+1, ok, err)
		}
	}
	if ok, _ := l.Allow(ctx, "/api.im.v1.AuthService/Register", "10.0.0.1", ""); ok {
		t.Fatalf("expected 6th register attempt from same IP to be denied")
	}
}

func TestAuthRateLimiter_WindowExpiry(t *testing.T) {
	l, mr := newTestLimiter(t)
	ctx := context.Background()

	for i := 0; i < loginIPLimit; i++ {
		if _, err := l.Allow(ctx, "/api.im.v1.AuthService/Login", "10.0.0.1", ""); err != nil {
			t.Fatalf("attempt %d failed: %v", i+1, err)
		}
	}
	if ok, _ := l.Allow(ctx, "/api.im.v1.AuthService/Login", "10.0.0.1", ""); ok {
		t.Fatalf("expected denied before window expiry")
	}

	// After the window expires the counter resets.
	mr.FastForward(61 * time.Second)
	ok, err := l.Allow(ctx, "/api.im.v1.AuthService/Login", "10.0.0.1", "")
	if err != nil || !ok {
		t.Fatalf("expected allowed after window expiry, got ok=%v err=%v", ok, err)
	}
}

func TestAuthRateLimiter_OtherOperationsUnlimited(t *testing.T) {
	l, _ := newTestLimiter(t)
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		ok, err := l.Allow(ctx, "/api.im.v1.AuthService/RefreshToken", "10.0.0.1", "")
		if err != nil || !ok {
			t.Fatalf("refresh must not be rate limited: ok=%v err=%v", ok, err)
		}
	}
}

func TestAuthRateLimiter_NilRedisFailsOpen(t *testing.T) {
	l := NewAuthRateLimiter(nil, log.NewStdLogger(nil))
	for i := 0; i < 1000; i++ {
		if ok, err := l.Allow(context.Background(), "/api.im.v1.AuthService/Login", "10.0.0.1", "alice"); !ok || err != nil {
			t.Fatalf("nil redis must fail open, got ok=%v err=%v", ok, err)
		}
	}
}

func TestAuthRateLimiter_MiddlewareDenies(t *testing.T) {
	l, _ := newTestLimiter(t)

	nextCalled := 0
	next := func(ctx context.Context, req interface{}) (interface{}, error) {
		nextCalled++
		return &pb.LoginReply{}, nil
	}
	handler := l.Middleware()(next)

	// Drive the middleware without a transport context: the operation will
	// not match login/register, so everything passes (middleware is mounted
	// via selector on real routes).
	if _, err := handler(context.Background(), &pb.LoginRequest{Username: "x"}); err != nil {
		t.Fatalf("unexpected error for unmatched operation: %v", err)
	}
	if nextCalled != 1 {
		t.Fatalf("expected next to be called once, got %d", nextCalled)
	}
}

func ipFor(i int) string {
	return "10.1.0." + string(rune('1'+i%200))
}

func TestParseClientIP(t *testing.T) {
	for in, want := range map[string]string{
		"1.2.3.4:5678":   "1.2.3.4",
		"[::1]:8080":     "::1",
		"1.2.3.4":        "1.2.3.4",
		"bad:addr:thing": "bad:addr:thing",
	} {
		if got := parseClientIP(in); got != want {
			t.Fatalf("parseClientIP(%q)=%q, want %q", in, got, want)
		}
	}
}
