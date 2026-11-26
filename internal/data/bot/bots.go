package botdata

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"nonoka-im/internal/bot/domain"

	"gorm.io/gorm"
)

func (s *Store) CreateBot(ctx context.Context, ownerID int64, username, name, description string, events []string) (*domain.Bot, error) {
	rawEvents, err := json.Marshal(events)
	if err != nil {
		return nil, err
	}
	var created *domain.Bot
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user := userRow{Username: username, Password: "!", IsBot: true}
		if err := tx.Create(&user).Error; err != nil {
			if duplicateError(err) {
				return domain.ErrConflict
			}
			return err
		}
		row := botRow{
			BotUserID: user.ID, OwnerUserID: ownerID, Name: name, Description: description,
			Status: domain.StatusActive, EventSubscriptions: string(rawEvents),
		}
		if err := tx.Create(&row).Error; err != nil {
			if duplicateError(err) {
				return domain.ErrConflict
			}
			return err
		}
		var loaded botRow
		if err := tx.Table(tableBots).
			Select("bots.*, users.username").
			Joins("JOIN users ON users.id = bots.bot_user_id").
			Where("bots.id = ?", row.ID).
			First(&loaded).Error; err != nil {
			return err
		}
		bot := toDomainBot(loaded)
		created = &bot
		return nil
	})
	return created, err
}

func (s *Store) ListBots(ctx context.Context, ownerID int64, limit, offset int) ([]domain.Bot, int64, error) {
	q := s.db.WithContext(ctx).Table(tableBots).Where("bots.owner_user_id = ?", ownerID)
	return s.listBots(ctx, q, limit, offset)
}

func (s *Store) ListAllBots(ctx context.Context, limit, offset int) ([]domain.Bot, int64, error) {
	q := s.db.WithContext(ctx).Table(tableBots)
	return s.listBots(ctx, q, limit, offset)
}

func (s *Store) listBots(ctx context.Context, q *gorm.DB, limit, offset int) ([]domain.Bot, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []botRow
	if err := q.Select("bots.*, users.username").
		Joins("JOIN users ON users.id = bots.bot_user_id").
		Order("bots.created_at DESC").
		Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Bot, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainBot(row))
	}
	return out, total, nil
}

func (s *Store) GetBot(ctx context.Context, id int64) (*domain.Bot, error) {
	var row botRow
	err := s.db.WithContext(ctx).Table(tableBots).
		Select("bots.*, users.username").
		Joins("JOIN users ON users.id = bots.bot_user_id").
		Where("bots.id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	bot := toDomainBot(row)
	return &bot, nil
}

func (s *Store) GetBotByUserID(ctx context.Context, userID int64) (*domain.Bot, error) {
	var row botRow
	err := s.db.WithContext(ctx).Table(tableBots).
		Select("bots.*, users.username").
		Joins("JOIN users ON users.id = bots.bot_user_id").
		Where("bots.bot_user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	bot := toDomainBot(row)
	return &bot, nil
}

func (s *Store) ListBotsByUserIDs(ctx context.Context, ids []int64) ([]domain.Bot, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []botRow
	err := s.db.WithContext(ctx).Table(tableBots).
		Select("bots.*, users.username").
		Joins("JOIN users ON users.id = bots.bot_user_id").
		Where("bots.bot_user_id IN ? AND bots.status = ?", ids, domain.StatusActive).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Bot, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainBot(row))
	}
	return out, nil
}

func (s *Store) UpdateBot(ctx context.Context, id int64, name, description, status string, events []string) error {
	rawEvents, err := json.Marshal(events)
	if err != nil {
		return err
	}
	res := s.db.WithContext(ctx).Table(tableBots).Where("id = ?", id).Updates(map[string]any{
		"name": name, "description": description, "status": status,
		"event_subscriptions": string(rawEvents), "updated_at": time.Now(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) DisableBot(ctx context.Context, id int64) error {
	now := time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Table(tableBots).Where("id = ?", id).Updates(map[string]any{
			"status": domain.StatusDisabled, "updated_at": now,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		if err := tx.Table(tableTokens).Where("bot_id = ? AND revoked_at IS NULL", id).
			Update("revoked_at", now).Error; err != nil {
			return err
		}
		if err := tx.Table(tableWebhooks).Where("bot_id = ?", id).
			Updates(map[string]any{"enabled": false, "disabled_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Table(tableDeliveries).
			Where("bot_id = ? AND status IN ?", id, []string{domain.DeliveryPending, domain.DeliveryDelivering}).
			Updates(map[string]any{"status": domain.DeliveryFailed, "locked_until": nil, "last_error": "bot disabled"}).Error
	})
}
