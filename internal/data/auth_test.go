package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) (*Data, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &Data{Redis: client}, mr
}

func TestAuthRepo_RefreshTokenRoundTrip(t *testing.T) {
	d, mr := setupTestRedis(t)
	repo := &authRepo{data: d}

	ctx := context.Background()
	tokenHash := sha256Hex("some-refresh-token")

	if err := repo.SaveRefreshTokenHash(ctx, 42, "dev-1", tokenHash, time.Hour); err != nil {
		t.Fatalf("SaveRefreshTokenHash failed: %v", err)
	}

	// Key format and TTL are part of the contract with the biz layer.
	key := "im:refresh:42:dev-1"
	stored, err := mr.Get(key)
	if err != nil {
		t.Fatalf("key not found in miniredis: %v", err)
	}
	if stored != tokenHash {
		t.Fatalf("stored value mismatch: got %q", stored)
	}
	if mr.TTL(key) > time.Hour {
		t.Fatalf("expected TTL <= 1h, got %v", mr.TTL(key))
	}

	got, err := repo.GetRefreshTokenHash(ctx, 42, "dev-1")
	if err != nil {
		t.Fatalf("GetRefreshTokenHash failed: %v", err)
	}
	if got != tokenHash {
		t.Fatalf("expected %q, got %q", tokenHash, got)
	}

	if err := repo.DeleteRefreshToken(ctx, 42, "dev-1"); err != nil {
		t.Fatalf("DeleteRefreshToken failed: %v", err)
	}
	if _, err := repo.GetRefreshTokenHash(ctx, 42, "dev-1"); !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound after delete, got %v", err)
	}
}

func TestAuthRepo_RefreshTokenDevicesAreIndependent(t *testing.T) {
	d, _ := setupTestRedis(t)
	repo := &authRepo{data: d}
	ctx := context.Background()

	h1 := sha256Hex("token-1")
	h2 := sha256Hex("token-2")
	if err := repo.SaveRefreshTokenHash(ctx, 7, "dev-1", h1, time.Hour); err != nil {
		t.Fatalf("save dev-1: %v", err)
	}
	if err := repo.SaveRefreshTokenHash(ctx, 7, "dev-2", h2, time.Hour); err != nil {
		t.Fatalf("save dev-2: %v", err)
	}

	got, err := repo.GetRefreshTokenHash(ctx, 7, "dev-2")
	if err != nil {
		t.Fatalf("get dev-2: %v", err)
	}
	if got != h2 {
		t.Fatalf("devices share refresh token storage")
	}

	if err := repo.DeleteRefreshToken(ctx, 7, "dev-1"); err != nil {
		t.Fatalf("delete dev-1: %v", err)
	}
	if _, err := repo.GetRefreshTokenHash(ctx, 7, "dev-1"); !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound for dev-1, got %v", err)
	}
	if got, err := repo.GetRefreshTokenHash(ctx, 7, "dev-2"); err != nil || got != h2 {
		t.Fatalf("dev-2 token must survive dev-1 logout: %q %v", got, err)
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
