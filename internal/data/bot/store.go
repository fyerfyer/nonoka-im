package botdata

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

const (
	tableBots       = "bots"
	tableTokens     = "bot_tokens"
	tableWebhooks   = "bot_webhooks"
	tableDeliveries = "bot_deliveries"
	tableUsers      = "users"
)

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func duplicateError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}
