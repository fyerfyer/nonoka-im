package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/bot/domain"
)

func (s *StreamService) publish(ctx context.Context, event StreamEvent) {
	if s.Publisher == nil || s.Recipients == nil {
		return
	}
	recipients, err := s.Recipients.Recipients(ctx, event.Topic, event.BotUserID)
	if err != nil {
		s.log().Warnf("resolve bot stream recipients failed: stream_id=%s topic=%s err=%v", event.StreamID, event.Topic, err)
		return
	}
	if len(recipients) == 0 {
		return
	}
	if err := s.Publisher.PublishBotStream(ctx, event, recipients); err != nil {
		s.log().Warnf("publish bot stream event failed: stream_id=%s type=%s err=%v", event.StreamID, event.Type, err)
	}
}

func randomStreamID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func mapStreamError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domain.ErrStreamExpired):
		return domain.ErrStreamExpired
	case errors.Is(err, domain.ErrForbidden):
		return ErrForbidden
	default:
		return err
	}
}
