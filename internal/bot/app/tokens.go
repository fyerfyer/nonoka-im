package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"nonoka-im/internal/bot/domain"
)

func (s *Service) CreateToken(ctx context.Context, userID, botID int64, name string, expiresIn time.Duration) (*domain.Token, string, error) {
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
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	if len(name) > maxTokenNameBytes {
		return nil, "", fmt.Errorf("%w: token name is too long", ErrInvalidInput)
	}
	tokens, err := s.Store.ListTokens(ctx, botID)
	if err != nil {
		return nil, "", err
	}
	now := time.Now()
	active := 0
	for _, token := range tokens {
		if token.Active(now) {
			active++
		}
	}
	if active >= s.maxTokens() {
		return nil, "", fmt.Errorf("%w: active token limit reached", ErrConflict)
	}
	id, err := randomHex(12)
	if err != nil {
		return nil, "", err
	}
	prefix, err := randomHex(6)
	if err != nil {
		return nil, "", err
	}
	secretBytes := make([]byte, tokenSecretBytes)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	plain := "nb_" + prefix + "_" + secret
	sum := sha256.Sum256([]byte(secret))
	token := domain.Token{
		ID: id, BotID: botID, Name: name, Prefix: prefix,
		Scopes: []string{"bot:send", "bot:read"}, CreatedBy: userID,
	}
	if expiresIn > 0 {
		expires := now.Add(expiresIn)
		token.ExpiresAt = &expires
	}
	if err := s.Store.CreateToken(ctx, token, sum[:]); err != nil {
		return nil, "", err
	}
	token.CreatedAt = now
	s.record(ctx, userID, "bot.token.create", "bot", fmt.Sprintf("%d", botID), nil, map[string]any{
		"token_id": id, "name": name, "prefix": prefix,
	})
	return &token, plain, nil
}

func (s *Service) ListTokens(ctx context.Context, userID, botID int64) ([]domain.Token, error) {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	if err := s.requireOwnerOrAdmin(ctx, userID, bot); err != nil {
		return nil, err
	}
	return s.Store.ListTokens(ctx, botID)
}

func (s *Service) RevokeToken(ctx context.Context, userID, botID int64, tokenID string) error {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return mapNotFound(err)
	}
	if err := s.requireOwner(ctx, userID, bot); err != nil {
		return err
	}
	if err := s.Store.RevokeToken(ctx, botID, tokenID); err != nil {
		if errors.Is(err, domain.ErrTokenNotFound) {
			return ErrNotFound
		}
		return err
	}
	s.record(ctx, userID, "bot.token.revoke", "bot", fmt.Sprintf("%d", botID), nil, map[string]any{"token_id": tokenID})
	return nil
}
