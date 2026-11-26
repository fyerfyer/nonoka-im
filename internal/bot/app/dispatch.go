package app

import (
	"context"
	"errors"
	"fmt"

	"nonoka-im/internal/bot/domain"
	"nonoka-im/internal/topic"
)

func (s *DeliveryService) Dispatch(ctx context.Context, ev MessageEvent) error {
	if ev.SenderID <= 0 || ev.Topic == "" || ev.MsgID <= 0 {
		return fmt.Errorf("invalid bot event: sender, topic and message id are required")
	}
	eventType := ""
	var botUserIDs []int64
	switch topic.ParseType(ev.Topic) {
	case topic.TypeP2P:
		first, second, err := topic.ExtractUserIDsFromP2PTopic(ev.Topic)
		if err != nil {
			return err
		}
		switch ev.SenderID {
		case first:
			botUserIDs = []int64{second}
		case second:
			botUserIDs = []int64{first}
		default:
			return nil
		}
		eventType = domain.EventMessageDirect
	case topic.TypeGroup:
		botUserIDs = ev.MentionedAgentIDs
		eventType = domain.EventMessageMention
	default:
		return nil
	}
	if len(botUserIDs) == 0 {
		return nil
	}
	bots, err := s.Store.ListBotsByUserIDs(ctx, botUserIDs)
	if err != nil {
		return fmt.Errorf("list bots for event: %w", err)
	}
	for _, bot := range bots {
		if bot.BotUserID == ev.SenderID {
			continue
		}
		if !bot.Subscribes(eventType) {
			continue
		}
		if s.Reader != nil {
			// Drop events for topics the bot cannot read. This prevents a
			// forged @mention from leaking a private conversation.
			if _, err := s.Reader.LoadMessage(ctx, bot.BotUserID, ev.Topic, ev.MsgID); err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					continue
				}
				return fmt.Errorf("load bot trigger message for bot %d: %w", bot.ID, err)
			}
		}
		eventID := deliveryEventID(ev.EventID, bot.ID, eventType)
		delivery := &domain.Delivery{
			BotID: bot.ID, EventID: eventID, EventType: eventType,
			Topic: ev.Topic, MsgID: ev.MsgID, Status: domain.DeliveryPending,
			NextAttemptAt: s.now(), CreatedAt: s.now(),
		}
		created, err := s.Store.EnqueueDelivery(ctx, delivery)
		if err != nil {
			return fmt.Errorf("enqueue bot delivery for bot %d: %w", bot.ID, err)
		}
		if created {
			s.log().Debugf("enqueued bot delivery: bot_id=%d event_type=%s topic=%s msg_id=%d", bot.ID, eventType, ev.Topic, ev.MsgID)
		}
	}
	return nil
}

// DeliverNext claims and processes one due delivery. It returns false when the
// queue is empty.
