package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"nonoka-im/internal/bot/domain"
	"nonoka-im/internal/topic"
)

func (s *DeliveryService) SendTest(ctx context.Context, bot *domain.Bot, webhook *domain.Webhook, secret []byte) (WebhookResponse, error) {
	if bot == nil || webhook == nil {
		return WebhookResponse{}, ErrNotFound
	}
	eventID := "evt_test_" + randomID()
	body, err := json.Marshal(DeliveryEvent{
		SpecVersion: "1", ID: eventID, Type: domain.EventWebhookTest,
		CreatedAt: s.now().UTC().Format(time.RFC3339Nano),
		Bot:       EventBot{ID: fmt.Sprintf("%d", bot.ID), UserID: fmt.Sprintf("%d", bot.BotUserID), Name: bot.Name},
		Data:      DeliveryData{},
	})
	if err != nil {
		return WebhookResponse{}, err
	}
	if len(body) > maxWebhookBody {
		return WebhookResponse{}, fmt.Errorf("%w: payload too large", ErrInvalidInput)
	}
	resp, err := s.Sender.Send(ctx, WebhookTarget{URL: webhook.URL, Secret: secret, Attempt: 1}, eventID, body)
	if err != nil {
		_ = s.Store.MarkWebhookFailure(ctx, bot.ID, false)
		return resp, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = s.Store.MarkWebhookSuccess(ctx, bot.ID)
	} else {
		_ = s.Store.MarkWebhookFailure(ctx, bot.ID, resp.StatusCode == http.StatusGone)
	}
	return resp, nil
}

func (s *DeliveryService) buildBody(ctx context.Context, bot *domain.Bot, delivery *domain.Delivery) ([]byte, error) {
	event := DeliveryEvent{
		SpecVersion: "1",
		ID:          delivery.EventID,
		Type:        delivery.EventType,
		CreatedAt:   delivery.CreatedAt.UTC().Format(time.RFC3339Nano),
		Bot:         EventBot{ID: fmt.Sprintf("%d", bot.ID), UserID: fmt.Sprintf("%d", bot.BotUserID), Name: bot.Name},
	}
	if delivery.EventType == domain.EventWebhookTest {
		return json.Marshal(event)
	}
	if s.Reader == nil {
		return nil, fmt.Errorf("message reader unavailable")
	}
	msg, err := s.Reader.LoadMessage(ctx, bot.BotUserID, delivery.Topic, delivery.MsgID)
	if err != nil {
		return nil, err
	}
	username := "unknown"
	if s.Users != nil {
		if name, err := s.Users.Username(ctx, msg.SenderID); err == nil && name != "" {
			username = name
		}
	}
	conversationType := "group"
	switch topic.ParseType(delivery.Topic) {
	case topic.TypeP2P:
		conversationType = "p2p"
	case topic.TypeSystem:
		conversationType = "system"
	}
	createdAt := time.Unix(msg.Timestamp, 0).UTC().Format(time.RFC3339Nano)
	event.Data = DeliveryData{
		Message: &EventMessage{
			ID: fmt.Sprintf("%d", msg.ID), Topic: msg.Topic, TopicSeq: fmt.Sprintf("%d", msg.TopicSeq),
			Type: messageTypeName(msg.Type), Content: string(msg.Content),
			Sender:    EventSender{ID: fmt.Sprintf("%d", msg.SenderID), Username: username},
			CreatedAt: createdAt,
		},
		Conversation: &EventConversation{Topic: delivery.Topic, Type: conversationType},
	}
	if delivery.EventType == domain.EventMessageMention {
		event.Data.Message.Mentions = []EventMention{{ID: fmt.Sprintf("%d", bot.BotUserID), Username: bot.Username}}
	}
	body, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	if len(body) > maxWebhookBody {
		return nil, fmt.Errorf("%w: payload too large", ErrInvalidInput)
	}
	return body, nil
}

func deliveryEventID(source string, botID int64, eventType string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s:%d:%s", source, botID, eventType))
	return "evt_" + hex.EncodeToString(sum[:16])
}

func messageTypeName(value int32) string {
	switch value {
	case 1:
		return "text"
	case 2:
		return "image"
	case 3:
		return "file"
	case 4:
		return "voice"
	case 5:
		return "agent_card"
	default:
		return "unknown"
	}
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func randomID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func randFloat() float64 {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return 0.5
	}
	var value uint64
	for _, b := range buf {
		value = value<<8 | uint64(b)
	}
	return float64(value%math.MaxUint32) / math.MaxUint32
}
