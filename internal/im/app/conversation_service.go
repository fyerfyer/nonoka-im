package app

import (
	"context"

	"nonoka-im/internal/im/domain"
)

// ConversationService provides conversation summary usecases.
type ConversationService struct {
	Repo ConversationRepository
}

func (s *ConversationService) List(ctx context.Context, userID int64, limit int32, offset int64) ([]*domain.Conversation, error) {
	if userID <= 0 {
		return nil, ErrAuthRequired
	}
	return s.Repo.ListConversations(ctx, userID, limit, offset)
}

func (s *ConversationService) MarkRead(ctx context.Context, userID int64, topic string, upToSeq uint64) error {
	if userID <= 0 {
		return ErrAuthRequired
	}
	normalized, err := domain.NormalizeTopic(topic)
	if err != nil {
		return ErrInvalidTopic
	}
	return s.Repo.MarkRead(ctx, userID, normalized, upToSeq)
}
