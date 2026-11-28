package server

import (
	"context"
	"testing"

	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/metadata"
	"nonoka-im/internal/realtime"
)

func invokeWithClaims(t *testing.T, claims jwt5.MapClaims) error {
	t.Helper()
	next := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, nil
	}
	ctx := jwtMiddleware.NewContext(context.Background(), claims)
	_, err := rejectNonUserScope(next)(ctx, nil)
	return err
}

func TestRejectNonUserScope(t *testing.T) {
	// User access tokens are issued without a scope claim and must pass.
	if err := invokeWithClaims(t, jwt5.MapClaims{"user_id": 1}); err != nil {
		t.Fatalf("token without scope must be allowed: %v", err)
	}
	// Explicit user scope passes.
	if err := invokeWithClaims(t, jwt5.MapClaims{"user_id": 1, "scope": "user"}); err != nil {
		t.Fatalf("user scope must be allowed: %v", err)
	}
	// Bot credentials (scope agent:send) are rejected on gRPC.
	if err := invokeWithClaims(t, jwt5.MapClaims{"user_id": 2, "scope": "agent:send"}); err == nil {
		t.Fatal("bot scope must be rejected on gRPC")
	}
	// A non-string scope is unparseable and keeps existing behavior (allowed).
	if err := invokeWithClaims(t, jwt5.MapClaims{"user_id": 3, "scope": 42}); err != nil {
		t.Fatalf("unparseable scope must keep existing behavior: %v", err)
	}
}

func TestRejectNonUserScopeWithoutClaims(t *testing.T) {
	// Public endpoints skip JWT validation entirely; no claims in context.
	next := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, nil
	}
	if _, err := rejectNonUserScope(next)(context.Background(), nil); err != nil {
		t.Fatalf("requests without claims must pass: %v", err)
	}
}

func TestRequireInternalPushToken(t *testing.T) {
	t.Setenv("INTERNAL_PUSH_TOKEN", "secret")
	next := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}
	if _, err := requireInternalPushToken(next)(context.Background(), nil); err == nil {
		t.Fatal("missing metadata must be rejected")
	}
	wrong := metadata.NewIncomingContext(context.Background(), metadata.Pairs(realtime.InternalPushTokenHeader, "wrong"))
	if _, err := requireInternalPushToken(next)(wrong, nil); err == nil {
		t.Fatal("wrong token must be rejected")
	}
	ok := metadata.NewIncomingContext(context.Background(), metadata.Pairs(realtime.InternalPushTokenHeader, "secret"))
	if _, err := requireInternalPushToken(next)(ok, nil); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}

	// Missing configuration must fail closed; an open PushService lets any
	// caller inject messages/events into arbitrary sessions.
	t.Setenv("INTERNAL_PUSH_TOKEN", "")
	if _, err := requireInternalPushToken(next)(context.Background(), nil); err == nil {
		t.Fatal("empty token must be rejected")
	}
}
