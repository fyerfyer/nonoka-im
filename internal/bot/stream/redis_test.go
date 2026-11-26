package stream

import (
	"context"
	"errors"
	"testing"
	"time"

	"nonoka-im/internal/bot/app"
	"nonoka-im/internal/bot/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisStore(client), mr
}

func TestRedisStoreAppendAndFinalize(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	state := app.StreamState{
		ID: "bst_1", BotID: 1, BotUserID: 2, Topic: "p2p_1_2",
		ClientMsgID: "stream-1", State: app.StreamStateOpen, CreatedAt: time.Now(),
	}
	if err := store.Create(ctx, state, time.Minute); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.Create(ctx, state, time.Minute); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate create: %v", err)
	}
	got, err := store.Append(ctx, "bst_1", 1, 1, "hello ", 1024, time.Minute)
	if err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if got.Content != "hello " || got.Seq != 1 {
		t.Fatalf("unexpected state: %+v", got)
	}
	got, err = store.Append(ctx, "bst_1", 1, 2, "world", 1024, time.Minute)
	if err != nil {
		t.Fatalf("append 2: %v", err)
	}
	if got.Content != "hello world" || got.Seq != 2 {
		t.Fatalf("unexpected state: %+v", got)
	}
	// Duplicate/older sequence is a no-op.
	got, err = store.Append(ctx, "bst_1", 1, 2, "!!!", 1024, time.Minute)
	if err != nil {
		t.Fatalf("duplicate append: %v", err)
	}
	if got.Content != "hello world" {
		t.Fatalf("duplicate changed content: %+v", got)
	}
	if _, err := store.Append(ctx, "bst_1", 99, 3, "x", 1024, time.Minute); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("wrong bot error: %v", err)
	}
	final, err := store.MarkFinal(ctx, "bst_1", 1)
	if err != nil {
		t.Fatalf("final: %v", err)
	}
	if final.State != app.StreamStateFinal {
		t.Fatalf("state = %s", final.State)
	}
	if _, err := store.Append(ctx, "bst_1", 1, 3, "late", 1024, time.Minute); !errors.Is(err, domain.ErrStreamExpired) {
		t.Fatalf("append after final: %v", err)
	}
	if deleted, err := store.Delete(ctx, "bst_1", 1); err != nil || deleted.State != app.StreamStateFinal {
		t.Fatalf("delete final stream: state=%+v err=%v", deleted, err)
	}
	if state, err := store.Get(ctx, "bst_1"); err != nil || state.State != app.StreamStateFinal {
		t.Fatalf("final stream should remain until TTL: state=%+v err=%v", state, err)
	}
	open := app.StreamState{
		ID: "bst_open", BotID: 1, BotUserID: 2, Topic: "p2p_1_2",
		ClientMsgID: "stream-open", State: app.StreamStateOpen, CreatedAt: time.Now(),
	}
	if err := store.Create(ctx, open, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Delete(ctx, "bst_open", 1); err != nil {
		t.Fatalf("delete open stream: %v", err)
	}
	if _, err := store.Get(ctx, "bst_open"); !errors.Is(err, domain.ErrStreamExpired) {
		t.Fatalf("get after delete: %v", err)
	}
}

func TestRedisStoreLimitsAndExpiry(t *testing.T) {
	store, mr := newTestStore(t)
	ctx := context.Background()
	state := app.StreamState{
		ID: "bst_2", BotID: 1, BotUserID: 2, Topic: "p2p_1_2",
		ClientMsgID: "stream-2", State: app.StreamStateOpen, CreatedAt: time.Now(),
	}
	if err := store.Create(ctx, state, time.Minute); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		allowed, err := store.AllowChunk(ctx, "bst_2", 2)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 && !allowed {
			t.Fatalf("chunk %d should be allowed", i)
		}
		if i == 2 && allowed {
			t.Fatal("third chunk should be rate limited")
		}
	}
	if _, err := store.Append(ctx, "bst_2", 1, 1, "12345", 4, time.Minute); err == nil {
		t.Fatal("oversized append accepted")
	}
	mr.FastForward(2 * time.Minute)
	if _, err := store.Get(ctx, "bst_2"); !errors.Is(err, domain.ErrStreamExpired) {
		t.Fatalf("expired stream: %v", err)
	}
}
