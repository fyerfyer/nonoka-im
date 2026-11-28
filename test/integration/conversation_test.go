package integration

import (
	"context"
	"testing"

	imdata "nonoka-im/internal/data/im"
	"nonoka-im/internal/im/domain"
)

// TestConversationSummaryIsMonotonic ensures a late/duplicate older message
// cannot roll back the conversation preview or increment unread twice.
func TestConversationSummaryIsMonotonic(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()
	ctx := context.Background()
	repo := imdata.NewConversationRepo(ts.data.DB(), testLogger)

	older := &domain.Message{
		ID: 1, Topic: "p2p_1_2", SenderID: 1, Type: domain.MessageTypeText,
		Content: []byte("first"), Timestamp: 100,
	}
	if err := repo.UpsertConversations(ctx, []int64{1, 2}, older, 10); err != nil {
		t.Fatalf("upsert first: %v", err)
	}
	newer := &domain.Message{
		ID: 2, Topic: "p2p_1_2", SenderID: 2, Type: domain.MessageTypeText,
		Content: []byte("second"), Timestamp: 200,
	}
	if err := repo.UpsertConversations(ctx, []int64{1, 2}, newer, 11); err != nil {
		t.Fatalf("upsert newer: %v", err)
	}
	// Simulate a Kafka/DLQ replay of an older seq after the newer one.
	replayed := &domain.Message{
		ID: 3, Topic: "p2p_1_2", SenderID: 2, Type: domain.MessageTypeText,
		Content: []byte("stale"), Timestamp: 50,
	}
	if err := repo.UpsertConversations(ctx, []int64{1, 2}, replayed, 5); err != nil {
		t.Fatalf("upsert replay: %v", err)
	}

	conversations, err := repo.ListConversations(ctx, 1, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("conversations = %d, want 1", len(conversations))
	}
	got := conversations[0]
	if got.LastSeq != 11 {
		t.Fatalf("last_seq = %d, want 11", got.LastSeq)
	}
	if got.LastMsgPreview != "second" {
		t.Fatalf("preview = %q, want second", got.LastMsgPreview)
	}
	if got.UnreadCount != 1 {
		t.Fatalf("unread = %d, want 1 (stale replay must not increment)", got.UnreadCount)
	}
}
