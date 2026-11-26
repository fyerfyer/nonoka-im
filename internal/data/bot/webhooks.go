package botdata

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"nonoka-im/internal/bot/domain"

	"gorm.io/gorm"
)

func (s *Store) CreateWebhook(ctx context.Context, webhook domain.Webhook, ciphertext []byte) error {
	rawEvents, err := json.Marshal(webhook.Events)
	if err != nil {
		return err
	}
	row := webhookRow{
		BotID: webhook.BotID, URL: webhook.URL, SecretCiphertext: ciphertext,
		SecretPrefix: webhook.SecretPrefix, Events: string(rawEvents), Enabled: webhook.Enabled,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		if duplicateError(err) {
			return domain.ErrConflict
		}
		return err
	}
	return nil
}

func (s *Store) UpdateWebhook(ctx context.Context, botID int64, url string, events []string, enabled bool) error {
	rawEvents, err := json.Marshal(events)
	if err != nil {
		return err
	}
	now := time.Now()
	res := s.db.WithContext(ctx).Table(tableWebhooks).Where("bot_id = ?", botID).Updates(map[string]any{
		"url": url, "events": string(rawEvents), "enabled": enabled,
		"disabled_at": nil, "updated_at": now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrWebhookAbsent
	}
	return nil
}

func (s *Store) GetWebhook(ctx context.Context, botID int64) (*domain.Webhook, []byte, error) {
	var row webhookRow
	err := s.db.WithContext(ctx).Where("bot_id = ?", botID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, domain.ErrWebhookAbsent
	}
	if err != nil {
		return nil, nil, err
	}
	webhook := toDomainWebhook(row)
	return &webhook, row.SecretCiphertext, nil
}

func (s *Store) RotateWebhookSecret(ctx context.Context, botID int64, ciphertext []byte, prefix string) error {
	now := time.Now()
	res := s.db.WithContext(ctx).Table(tableWebhooks).Where("bot_id = ?", botID).Updates(map[string]any{
		"secret_ciphertext": ciphertext, "secret_prefix": prefix, "updated_at": now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrWebhookAbsent
	}
	return nil
}

func (s *Store) DeleteWebhook(ctx context.Context, botID int64) error {
	res := s.db.WithContext(ctx).Where("bot_id = ?", botID).Delete(&webhookRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrWebhookAbsent
	}
	return nil
}

func (s *Store) MarkWebhookSuccess(ctx context.Context, botID int64) error {
	now := time.Now()
	return s.db.WithContext(ctx).Table(tableWebhooks).Where("bot_id = ?", botID).Updates(map[string]any{
		"last_success_at": now, "verified_at": now, "consecutive_failures": 0, "updated_at": now,
	}).Error
}

func (s *Store) MarkWebhookFailure(ctx context.Context, botID int64, disable bool) error {
	now := time.Now()
	updates := map[string]any{
		"consecutive_failures": gorm.Expr("consecutive_failures + 1"), "updated_at": now,
	}
	if disable {
		updates["enabled"] = false
		updates["disabled_at"] = now
	}
	return s.db.WithContext(ctx).Table(tableWebhooks).Where("bot_id = ?", botID).Updates(updates).Error
}

// EnqueueDelivery inserts an idempotent delivery row. It returns false when the
// row already exists.
