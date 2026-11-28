package im

import (
	"context"
	"errors"
	"testing"

	"nonoka-im/internal/im/domain"
	"nonoka-im/internal/messaging"
)

type failingQuery struct {
	err error
}

func (q failingQuery) RecentMessages(context.Context, int64, string, int) ([]domain.InboxMessage, error) {
	return nil, q.err
}

func (q failingQuery) MessageForUser(context.Context, int64, string, int64) (domain.InboxMessage, error) {
	return domain.InboxMessage{}, q.err
}

func (q failingQuery) MessageByClientID(context.Context, int64, string, string) (domain.InboxMessage, error) {
	return domain.InboxMessage{}, q.err
}

func (q failingQuery) SearchMessages(context.Context, int64, string, string, domain.SearchFilter, int, int) ([]domain.SearchHit, bool, error) {
	return nil, false, q.err
}

func TestMessageForUserOnlyMapsDomainNotFound(t *testing.T) {
	reader := &Reader{Query: failingQuery{err: domain.ErrNotFound}}
	if _, err := reader.MessageForUser(context.Background(), 1, "p2p_1_2", 9); !errors.Is(err, messaging.ErrNotFound) {
		t.Fatalf("domain not found = %v, want messaging.ErrNotFound", err)
	}

	transient := errors.New("mongo unavailable")
	reader = &Reader{Query: failingQuery{err: transient}}
	if _, err := reader.MessageForUser(context.Background(), 1, "p2p_1_2", 9); !errors.Is(err, transient) {
		t.Fatalf("transient error = %v, want original", err)
	}
}

func TestMessageByClientIDOnlyMapsDomainNotFound(t *testing.T) {
	reader := &Reader{Query: failingQuery{err: domain.ErrNotFound}}
	if _, err := reader.MessageByClientID(context.Background(), 1, "p2p_1_2", "client-1"); !errors.Is(err, messaging.ErrNotFound) {
		t.Fatalf("domain not found = %v, want messaging.ErrNotFound", err)
	}

	transient := errors.New("mongo unavailable")
	reader = &Reader{Query: failingQuery{err: transient}}
	if _, err := reader.MessageByClientID(context.Background(), 1, "p2p_1_2", "client-1"); !errors.Is(err, transient) {
		t.Fatalf("transient error = %v, want original", err)
	}
}
