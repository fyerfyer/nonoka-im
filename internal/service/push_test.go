package service

import (
	"context"
	"io"
	"testing"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/gateway"

	"github.com/go-kratos/kratos/v2/log"
)

// TestBatchPushConcurrent_ContextCanceled reports remaining users as failed.
// This is a regression test for a bug where context cancellation caused the
// remaining user IDs in a chunk to silently disappear instead of being reported
// as failed.
func TestBatchPushConcurrent_ContextCanceled(t *testing.T) {
	mgr := gateway.NewManager(log.NewStdLogger(io.Discard))
	svc := NewPushService(mgr, log.NewStdLogger(io.Discard))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	userIDs := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	resp, err := svc.BatchPushToUsers(ctx, &v1.BatchPushToUsersRequest{
		UserIds: userIDs,
		Message: &v1.MessagePush{
			MsgId:   1,
			Content: []byte("test"),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Every user should be reported as failed because the context was canceled.
	if len(resp.FailedUserIds) != len(userIDs) {
		t.Fatalf("expected %d failed user IDs, got %d", len(userIDs), len(resp.FailedUserIds))
	}

	failedSet := make(map[int64]bool, len(resp.FailedUserIds))
	for _, uid := range resp.FailedUserIds {
		failedSet[uid] = true
	}
	for _, uid := range userIDs {
		if !failedSet[uid] {
			t.Fatalf("expected user %d to be in failed list", uid)
		}
	}
}

// TestBatchPushReceiptsConcurrent_ContextCanceled is the receipt variant of the
// context cancellation regression test.
func TestBatchPushReceiptsConcurrent_ContextCanceled(t *testing.T) {
	mgr := gateway.NewManager(log.NewStdLogger(io.Discard))
	svc := NewPushService(mgr, log.NewStdLogger(io.Discard))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	userIDs := []int64{1, 2, 3, 4, 5}
	resp, err := svc.BatchPushReceiptToUsers(ctx, &v1.BatchPushReceiptToUsersRequest{
		UserIds: userIDs,
		Receipt: &v1.SendReceipt{
			ClientMsgId: "cmid-1",
			MsgId:       1,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.FailedUserIds) != len(userIDs) {
		t.Fatalf("expected %d failed user IDs, got %d", len(userIDs), len(resp.FailedUserIds))
	}
}

// TestBatchPushReceiptsToUsers_Empty verifies that an empty batch request is a
// no-op and returns zero delivered count.
func TestBatchPushReceiptsToUsers_Empty(t *testing.T) {
	mgr := gateway.NewManager(log.NewStdLogger(io.Discard))
	svc := NewPushService(mgr, log.NewStdLogger(io.Discard))

	resp, err := svc.BatchPushReceiptsToUsers(context.Background(), &v1.BatchPushReceiptsToUsersRequest{
		Items: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalDelivered != 0 {
		t.Fatalf("expected total_delivered=0, got %d", resp.TotalDelivered)
	}
	if len(resp.FailedItems) != 0 {
		t.Fatalf("expected no failed items, got %d", len(resp.FailedItems))
	}
}

// TestBatchPushReceiptsToUsers_OfflineUser reports the offline user as failed.
func TestBatchPushReceiptsToUsers_OfflineUser(t *testing.T) {
	mgr := gateway.NewManager(log.NewStdLogger(io.Discard))
	svc := NewPushService(mgr, log.NewStdLogger(io.Discard))

	resp, err := svc.BatchPushReceiptsToUsers(context.Background(), &v1.BatchPushReceiptsToUsersRequest{
		Items: []*v1.ReceiptBatchItem{
			{UserId: 42, Receipt: &v1.SendReceipt{ClientMsgId: "cmid-1", MsgId: 1}},
			{UserId: 43, Receipt: &v1.SendReceipt{ClientMsgId: "cmid-2", MsgId: 2}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalDelivered != 0 {
		t.Fatalf("expected total_delivered=0 for offline users, got %d", resp.TotalDelivered)
	}
	if len(resp.FailedItems) != 2 {
		t.Fatalf("expected 2 failed items, got %d", len(resp.FailedItems))
	}
}
