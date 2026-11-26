package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"strings"
	"time"

	"nonoka-im/internal/bot/domain"

	"github.com/go-kratos/kratos/v2/log"
)

type Principal struct {
	Bot   *domain.Bot
	Token *domain.Token
}

type AuthService struct {
	Store Store
	Log   *log.Helper
}

func NewAuthService(store Store, logger log.Logger) *AuthService {
	return &AuthService{Store: store, Log: log.NewHelper(logger)}
}

// Authenticate validates a bot bearer token.
func (s *AuthService) Authenticate(ctx context.Context, raw string) (*Principal, error) {
	prefix, secret, ok := splitToken(raw)
	if !ok {
		return nil, domain.ErrTokenNotFound
	}
	token, bot, storedHash, err := s.Store.FindTokenByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(sum[:], storedHash) != 1 {
		return nil, domain.ErrTokenNotFound
	}
	if !token.Active(time.Now()) {
		return nil, domain.ErrTokenNotFound
	}
	if !bot.Active() {
		return nil, domain.ErrDisabled
	}
	go func(id string) {
		touchCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Store.TouchToken(touchCtx, id)
	}(token.ID)
	return &Principal{Bot: bot, Token: token}, nil
}

func splitToken(raw string) (prefix, secret string, ok bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "nb_") {
		return "", "", false
	}
	rest := strings.TrimPrefix(raw, "nb_")
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 || len(parts[0]) < 6 || len(parts[1]) < 20 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
