package botdata

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"nonoka-im/internal/bot/domain"

	"gorm.io/gorm"
)

func (s *Store) CreateToken(ctx context.Context, token domain.Token, hash []byte) error {
	rawScopes, err := json.Marshal(token.Scopes)
	if err != nil {
		return err
	}
	row := tokenRow{
		ID: token.ID, BotID: token.BotID, Name: token.Name, Prefix: token.Prefix,
		TokenHash: hash, Scopes: string(rawScopes), ExpiresAt: token.ExpiresAt,
		CreatedBy: token.CreatedBy,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		if duplicateError(err) {
			return domain.ErrConflict
		}
		return err
	}
	return nil
}

func (s *Store) ListTokens(ctx context.Context, botID int64) ([]domain.Token, error) {
	var rows []tokenRow
	if err := s.db.WithContext(ctx).Where("bot_id = ?", botID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Token, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomainToken(row))
	}
	return out, nil
}

func (s *Store) RevokeToken(ctx context.Context, botID int64, tokenID string) error {
	now := time.Now()
	res := s.db.WithContext(ctx).Table(tableTokens).
		Where("id = ? AND bot_id = ? AND revoked_at IS NULL", tokenID, botID).
		Update("revoked_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrTokenNotFound
	}
	return nil
}

// FindTokenByPrefix returns the token, its bot and the stored hash.
func (s *Store) FindTokenByPrefix(ctx context.Context, prefix string) (*domain.Token, *domain.Bot, []byte, error) {
	var token tokenRow
	err := s.db.WithContext(ctx).Where("prefix = ?", prefix).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, domain.ErrTokenNotFound
	}
	if err != nil {
		return nil, nil, nil, err
	}
	bot, err := s.GetBot(ctx, token.BotID)
	if err != nil {
		return nil, nil, nil, err
	}
	t := toDomainToken(token)
	return &t, bot, token.TokenHash, nil
}

func (s *Store) TouchToken(ctx context.Context, id string) error {
	now := time.Now()
	return s.db.WithContext(ctx).Table(tableTokens).Where("id = ?", id).
		Update("last_used_at", now).Error
}
