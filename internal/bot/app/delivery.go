package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"nonoka-im/internal/bot/domain"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	defaultMaxDeliveryAttempts = 8
	defaultDeliveryLock        = 2 * time.Minute
	maxResponseSnippet         = 512
	maxWebhookBody             = 256 << 10
)

var defaultRetryDelays = []time.Duration{
	0,
	5 * time.Second,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	5 * time.Hour,
	10 * time.Hour,
	20 * time.Hour,
}

type DeliveryConfig struct {
	MaxAttempts int
	RetryDelays []time.Duration
	Lock        time.Duration
	Jitter      bool
}

type DeliveryService struct {
	Store  Store
	Reader MessageLoader
	Users  UserDirectory
	Sender WebhookSender
	Sealer Sealer
	Log    *log.Helper
	Config DeliveryConfig
	Now    func() time.Time
}

func NewDeliveryService(store Store, reader MessageLoader, users UserDirectory, sender WebhookSender, sealer Sealer, logger log.Logger) *DeliveryService {
	return &DeliveryService{
		Store: store, Reader: reader, Users: users, Sender: sender, Sealer: sealer,
		Log: log.NewHelper(logger),
	}
}

func (s *DeliveryService) log() *log.Helper {
	if s.Log != nil {
		return s.Log
	}
	return log.NewHelper(log.DefaultLogger)
}

func (s *DeliveryService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *DeliveryService) retryDelays() []time.Duration {
	if len(s.Config.RetryDelays) > 0 {
		return s.Config.RetryDelays
	}
	return defaultRetryDelays
}

func (s *DeliveryService) maxAttempts() int {
	if s.Config.MaxAttempts > 0 {
		return s.Config.MaxAttempts
	}
	return defaultMaxDeliveryAttempts
}

func (s *DeliveryService) lockDuration() time.Duration {
	if s.Config.Lock > 0 {
		return s.Config.Lock
	}
	return defaultDeliveryLock
}

// Dispatch resolves message recipients to external bots and enqueues one
// durable delivery per bot. It only performs short operations so the Kafka
// consumer can commit offsets quickly.
func (s *DeliveryService) DeliverNext(ctx context.Context) (bool, error) {
	delivery, err := s.Store.ClaimDelivery(ctx, s.lockDuration())
	if err != nil {
		return false, err
	}
	if delivery == nil {
		return false, nil
	}
	if err := s.deliver(ctx, delivery); err != nil {
		return true, err
	}
	return true, nil
}

func (s *DeliveryService) deliver(ctx context.Context, delivery *domain.Delivery) error {
	bot, err := s.Store.GetBot(ctx, delivery.BotID)
	if err != nil {
		return s.fail(ctx, delivery, 0, "bot not found", "")
	}
	if !bot.Active() {
		return s.fail(ctx, delivery, 0, "bot disabled", "")
	}
	webhook, sealed, err := s.Store.GetWebhook(ctx, delivery.BotID)
	if err != nil {
		if errors.Is(err, domain.ErrWebhookAbsent) {
			return s.fail(ctx, delivery, 0, "webhook not configured", "")
		}
		return err
	}
	if !webhook.Accepts(delivery.EventType) {
		return s.fail(ctx, delivery, 0, "webhook disabled or event not subscribed", "")
	}
	secret, err := s.Sealer.Open(sealed)
	if err != nil {
		return s.fail(ctx, delivery, 0, "webhook secret unavailable", "")
	}
	body, err := s.buildBody(ctx, bot, delivery)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return s.fail(ctx, delivery, 0, "message not found or not visible", "")
		}
		return s.retry(ctx, delivery, 0, "load message failed", "", 0)
	}
	resp, sendErr := s.Sender.Send(ctx, WebhookTarget{URL: webhook.URL, Secret: secret, Attempt: delivery.Attempts}, delivery.EventID, body)
	if sendErr != nil {
		return s.retry(ctx, delivery, 0, sendErr.Error(), "", 0)
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if err := s.Store.MarkDeliverySucceeded(ctx, delivery.ID, resp.StatusCode, resp.Snippet); err != nil {
			return err
		}
		_ = s.Store.MarkWebhookSuccess(ctx, delivery.BotID)
		return nil
	case resp.StatusCode == http.StatusGone:
		_ = s.Store.MarkWebhookFailure(ctx, delivery.BotID, true)
		return s.fail(ctx, delivery, resp.StatusCode, "endpoint returned 410 gone", resp.Snippet)
	default:
		return s.retry(ctx, delivery, resp.StatusCode, fmt.Sprintf("webhook status %d", resp.StatusCode), resp.Snippet, resp.RetryAfter)
	}
}

func (s *DeliveryService) retry(ctx context.Context, delivery *domain.Delivery, statusCode int, message, snippet string, retryAfter time.Duration) error {
	if delivery.Attempts >= s.maxAttempts() {
		_ = s.Store.MarkWebhookFailure(ctx, delivery.BotID, false)
		return s.fail(ctx, delivery, statusCode, message, snippet)
	}
	delay := retryAfter
	if delay <= 0 {
		delays := s.retryDelays()
		index := delivery.Attempts - 1
		if index < 0 {
			index = 0
		}
		if index >= len(delays) {
			index = len(delays) - 1
		}
		delay = delays[index]
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	if s.Config.Jitter && delay > 0 {
		factor := 0.8 + randFloat()*0.4
		delay = time.Duration(float64(delay) * factor)
	}
	if err := s.Store.MarkDeliveryRetry(ctx, delivery.ID, s.now().Add(delay), statusCode, truncate(message, maxResponseSnippet), snippet); err != nil {
		return err
	}
	s.log().Warnf("bot webhook delivery failed; retrying: bot_id=%d delivery_id=%d attempt=%d status=%d err=%s",
		delivery.BotID, delivery.ID, delivery.Attempts, statusCode, message)
	return nil
}

func (s *DeliveryService) fail(ctx context.Context, delivery *domain.Delivery, statusCode int, message, snippet string) error {
	if err := s.Store.MarkDeliveryFailed(ctx, delivery.ID, statusCode, truncate(message, maxResponseSnippet), snippet); err != nil {
		return err
	}
	_ = s.Store.MarkWebhookFailure(ctx, delivery.BotID, false)
	s.log().Warnf("bot webhook delivery failed permanently: bot_id=%d delivery_id=%d status=%d err=%s",
		delivery.BotID, delivery.ID, statusCode, message)
	return nil
}

// SendTest posts a synthetic webhook.test event and returns the response.
