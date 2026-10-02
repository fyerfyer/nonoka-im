package msgworker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pb "nonoka-im/api/im/v1"

	"github.com/go-kratos/kratos/v2/log"
)

func TestBotIndexRefreshAndLookup(t *testing.T) {
	ctx := context.Background()
	ids := []int64{10, 20}
	idx := NewBotIndex(func(context.Context) ([]int64, error) { return ids, nil }, log.DefaultLogger)
	if idx.Ready() {
		t.Fatal("index must not be ready before the first successful load")
	}
	if err := idx.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !idx.Ready() {
		t.Fatal("index must be ready after a successful load")
	}
	for _, id := range []int64{10, 20} {
		if bot, err := idx.IsBot(ctx, id); err != nil || !bot {
			t.Fatalf("IsBot(%d) = %v, %v; want true, nil", id, bot, err)
		}
	}
	if bot, err := idx.IsBot(ctx, 30); err != nil || bot {
		t.Fatalf("IsBot(30) = %v, %v; want false, nil", bot, err)
	}
	if any, err := idx.AnyBot(ctx, []int64{5, 20}); err != nil || !any {
		t.Fatalf("AnyBot([5 20]) = %v, %v; want true, nil", any, err)
	}
	if any, err := idx.AnyBot(ctx, []int64{5, 6}); err != nil || any {
		t.Fatalf("AnyBot([5 6]) = %v, %v; want false, nil", any, err)
	}
}

func TestBotIndexKeepsLastGoodSetOnFailure(t *testing.T) {
	ctx := context.Background()
	fail := false
	idx := NewBotIndex(func(context.Context) ([]int64, error) {
		if fail {
			return nil, errors.New("db down")
		}
		return []int64{7}, nil
	}, log.DefaultLogger)
	if err := idx.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	fail = true
	if err := idx.Refresh(ctx); err == nil {
		t.Fatal("expected refresh error")
	}
	if bot, err := idx.IsBot(ctx, 7); err != nil || !bot {
		t.Fatalf("failed refresh must keep the last good set, got bot=%v err=%v", bot, err)
	}
}

// TestBotIndexQueryOnMiss verifies that an ID missing from the in-memory set
// is resolved through the query func: positive results are cached in the set
// (no repeat query), negative results are cached only for the short TTL.
func TestBotIndexQueryOnMiss(t *testing.T) {
	ctx := context.Background()
	var queried []int64
	idx := NewBotIndexWithQuery(
		func(context.Context) ([]int64, error) { return []int64{10, 20}, nil },
		func(_ context.Context, id int64) (bool, error) {
			queried = append(queried, id)
			return id == 30, nil // 30 is a bot the periodic load has not seen yet
		},
		log.DefaultLogger,
	)
	if err := idx.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if bot, err := idx.IsBot(ctx, 30); err != nil || !bot {
		t.Fatalf("IsBot(30) = %v, %v; want true, nil", bot, err)
	}
	if len(queried) != 1 {
		t.Fatalf("expected exactly one DB query, got %d", len(queried))
	}
	// A positive result joins the in-memory set: no repeat query.
	if bot, err := idx.IsBot(ctx, 30); err != nil || !bot {
		t.Fatalf("IsBot(30) = %v, %v; want cached true, nil", bot, err)
	}
	if len(queried) != 1 {
		t.Fatalf("expected cached positive to skip the DB, got %d queries", len(queried))
	}

	// A negative result is reused within the TTL instead of querying again.
	if bot, err := idx.IsBot(ctx, 40); err != nil || bot {
		t.Fatalf("IsBot(40) = %v, %v; want false, nil", bot, err)
	}
	if bot, err := idx.IsBot(ctx, 40); err != nil || bot {
		t.Fatalf("IsBot(40) = %v, %v; want cached false, nil", bot, err)
	}
	if len(queried) != 2 {
		t.Fatalf("expected negative cache to skip the DB within the TTL, got %d queries", len(queried))
	}
}

func TestBotIndexQueryErrorPropagates(t *testing.T) {
	idx := NewBotIndexWithQuery(
		func(context.Context) ([]int64, error) { return nil, nil },
		func(context.Context, int64) (bool, error) { return false, errors.New("db down") },
		log.DefaultLogger,
	)
	if _, err := idx.IsBot(context.Background(), 7); err == nil {
		t.Fatal("expected query error to propagate")
	}
}

func TestMessageCanTriggerAgent(t *testing.T) {
	ctx := context.Background()
	ids := []int64{900}
	idx := NewBotIndex(func(context.Context) ([]int64, error) { return ids, nil }, log.DefaultLogger)
	if err := idx.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	w := &MsgWorker{botIndex: idx}

	cases := []struct {
		name    string
		topic   string
		sender  int64
		mention []int64
		want    bool
	}{
		{"human to bot p2p", "p2p_100_900", 100, nil, true},
		{"bot to human p2p", "p2p_100_900", 900, nil, false},
		{"human to human p2p", "p2p_100_200", 100, nil, false},
		{"sender not a participant", "p2p_100_200", 900, nil, false},
		{"group with bot mention", "grp_group", 100, []int64{900}, true},
		{"group with only human mentions", "grp_group", 100, []int64{200, 300}, false},
		{"group without mentions", "grp_group", 100, nil, false},
		{"system topic", "sys_100", 100, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := &pb.UpstreamMessage{Topic: tc.topic, SenderId: tc.sender, MentionedUserIds: tc.mention}
			if got := w.messageCanTriggerAgent(ctx, up); got != tc.want {
				t.Fatalf("messageCanTriggerAgent(%s, sender=%d) = %v, want %v", tc.topic, tc.sender, got, tc.want)
			}
		})
	}
}

func TestBotIndexStartRefreshesPeriodically(t *testing.T) {
	var calls atomic.Int64
	idx := NewBotIndex(func(context.Context) ([]int64, error) {
		return []int64{calls.Add(1)}, nil
	}, log.DefaultLogger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idx.Start(ctx, 20*time.Millisecond)
	time.Sleep(70 * time.Millisecond)
	cancel()
	if n := calls.Load(); n < 2 {
		t.Fatalf("expected periodic refresh, got %d calls", n)
	}
	// The refresh goroutine may still be mid-flight after cancel; poll until
	// the latest loaded ID lands in the set.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if bot, _ := idx.IsBot(context.Background(), calls.Load()); bot {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("index did not pick up refreshed IDs")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
