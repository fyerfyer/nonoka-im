package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nonoka-im/internal/bot/domain"
)

type WebhookInput struct {
	URL     string
	Events  []string
	Enabled bool
	Rotate  bool
}

func (s *Service) GetWebhook(ctx context.Context, userID, botID int64) (*domain.Webhook, error) {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	if err := s.requireOwnerOrAdmin(ctx, userID, bot); err != nil {
		return nil, err
	}
	webhook, _, err := s.Store.GetWebhook(ctx, botID)
	if err != nil {
		if errors.Is(err, domain.ErrWebhookAbsent) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return webhook, nil
}

// SetWebhook creates or updates the endpoint. It returns the plaintext secret
// exactly once when a new secret is generated.
func (s *Service) SetWebhook(ctx context.Context, userID, botID int64, input WebhookInput) (*domain.Webhook, string, error) {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, "", mapNotFound(err)
	}
	if err := s.requireOwner(ctx, userID, bot); err != nil {
		return nil, "", err
	}
	if !bot.Active() {
		return nil, "", domain.ErrDisabled
	}
	rawURL := strings.TrimSpace(input.URL)
	if rawURL == "" {
		return nil, "", fmt.Errorf("%w: webhook url is required", ErrInvalidInput)
	}
	if s.Validator == nil {
		return nil, "", fmt.Errorf("%w: webhook validator unavailable", ErrInvalidInput)
	}
	if err := s.Validator.ValidateURL(rawURL); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	events, err := normalizeWebhookEvents(input.Events)
	if err != nil {
		return nil, "", err
	}
	existing, _, getErr := s.Store.GetWebhook(ctx, botID)
	newSecret := ""
	if errors.Is(getErr, domain.ErrWebhookAbsent) {
		secret, err := newWebhookSecret()
		if err != nil {
			return nil, "", err
		}
		sealed, err := s.Sealer.Seal([]byte(secret))
		if err != nil {
			return nil, "", err
		}
		webhook := domain.Webhook{
			BotID: botID, URL: rawURL, SecretPrefix: secretPrefix(secret),
			Events: events, Enabled: input.Enabled,
		}
		if err := s.Store.CreateWebhook(ctx, webhook, sealed); err != nil {
			return nil, "", err
		}
		newSecret = secret
	} else if getErr != nil {
		return nil, "", getErr
	} else {
		if err := s.Store.UpdateWebhook(ctx, botID, rawURL, events, input.Enabled); err != nil {
			return nil, "", err
		}
		if input.Rotate {
			secret, err := newWebhookSecret()
			if err != nil {
				return nil, "", err
			}
			sealed, err := s.Sealer.Seal([]byte(secret))
			if err != nil {
				return nil, "", err
			}
			if err := s.Store.RotateWebhookSecret(ctx, botID, sealed, secretPrefix(secret)); err != nil {
				return nil, "", err
			}
			newSecret = secret
		}
		_ = existing
	}
	webhook, _, err := s.Store.GetWebhook(ctx, botID)
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, userID, "bot.webhook.update", "bot", fmt.Sprintf("%d", botID),
		map[string]any{"url": existingURL(existing), "events": existingEvents(existing)},
		map[string]any{"url": webhook.URL, "events": webhook.Events, "enabled": webhook.Enabled, "secret_rotated": newSecret != ""})
	return webhook, newSecret, nil
}

func (s *Service) RotateWebhookSecret(ctx context.Context, userID, botID int64) (*domain.Webhook, string, error) {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, "", mapNotFound(err)
	}
	if err := s.requireOwner(ctx, userID, bot); err != nil {
		return nil, "", err
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return nil, "", err
	}
	sealed, err := s.Sealer.Seal([]byte(secret))
	if err != nil {
		return nil, "", err
	}
	if err := s.Store.RotateWebhookSecret(ctx, botID, sealed, secretPrefix(secret)); err != nil {
		if errors.Is(err, domain.ErrWebhookAbsent) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	webhook, _, err := s.Store.GetWebhook(ctx, botID)
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, userID, "bot.webhook.rotate", "bot", fmt.Sprintf("%d", botID), nil, map[string]any{"secret_prefix": webhook.SecretPrefix})
	return webhook, secret, nil
}

func (s *Service) DeleteWebhook(ctx context.Context, userID, botID int64) error {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return mapNotFound(err)
	}
	if err := s.requireOwner(ctx, userID, bot); err != nil {
		return err
	}
	if err := s.Store.DeleteWebhook(ctx, botID); err != nil {
		if errors.Is(err, domain.ErrWebhookAbsent) {
			return ErrNotFound
		}
		return err
	}
	s.record(ctx, userID, "bot.webhook.delete", "bot", fmt.Sprintf("%d", botID), nil, nil)
	return nil
}

func (s *Service) WebhookSecret(ctx context.Context, botID int64) (*domain.Webhook, []byte, error) {
	webhook, sealed, err := s.Store.GetWebhook(ctx, botID)
	if err != nil {
		return nil, nil, err
	}
	plain, err := s.Sealer.Open(sealed)
	if err != nil {
		return nil, nil, err
	}
	return webhook, plain, nil
}

func existingURL(webhook *domain.Webhook) string {
	if webhook == nil {
		return ""
	}
	return webhook.URL
}

func existingEvents(webhook *domain.Webhook) []string {
	if webhook == nil {
		return nil
	}
	return webhook.Events
}
