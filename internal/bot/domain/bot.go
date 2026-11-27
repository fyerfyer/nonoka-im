// Package domain holds external-bot entities and pure rules. It must not
// import gorm, mongo, HTTP or transport packages.
package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("bot not found")
	ErrTokenNotFound = errors.New("bot token not found")
	ErrWebhookAbsent = errors.New("bot webhook not configured")
	ErrForbidden     = errors.New("bot access denied")
	ErrInvalid       = errors.New("invalid bot input")
	ErrConflict      = errors.New("bot already exists")
	ErrDeliveryLease = errors.New("bot delivery lease lost")
	ErrDisabled      = errors.New("bot is disabled")
	ErrStreamExpired = errors.New("bot stream expired")
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

const (
	EventMessageMention = "message.mention"
	EventMessageDirect  = "message.direct"
	EventWebhookTest    = "webhook.test"
)

const (
	DeliveryPending    = "pending"
	DeliveryDelivering = "delivering"
	DeliverySucceeded  = "succeeded"
	DeliveryFailed     = "failed"
)

// Bot is an external integration identity. It owns one bot user and is managed
// by a normal Nonoka user.
type Bot struct {
	ID                 int64
	BotUserID          int64
	OwnerUserID        int64
	Username           string
	Name               string
	Description        string
	Status             string
	EventSubscriptions []string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (b Bot) Active() bool { return b.Status == StatusActive }

func (b Bot) Subscribes(eventType string) bool {
	for _, event := range b.EventSubscriptions {
		if event == eventType || event == "*" {
			return true
		}
	}
	return false
}

// Token is the metadata of one bot credential. The plaintext token is never
// persisted.
type Token struct {
	ID         string
	BotID      int64
	Name       string
	Prefix     string
	Scopes     []string
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
	CreatedBy  int64
	CreatedAt  time.Time
}

func (t Token) Active(now time.Time) bool {
	if t.RevokedAt != nil {
		return false
	}
	if t.ExpiresAt != nil && !now.Before(*t.ExpiresAt) {
		return false
	}
	return true
}

// Webhook is the single delivery endpoint configured for a bot.
type Webhook struct {
	ID                  int64
	BotID               int64
	URL                 string
	SecretPrefix        string
	Events              []string
	Enabled             bool
	VerifiedAt          *time.Time
	LastSuccessAt       *time.Time
	ConsecutiveFailures int
	DisabledAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (w Webhook) Accepts(eventType string) bool {
	if !w.Enabled || w.DisabledAt != nil {
		return false
	}
	for _, event := range w.Events {
		if event == eventType || event == "*" {
			return true
		}
	}
	return false
}

// Delivery is one durable webhook delivery attempt series for one bot/event.
type Delivery struct {
	ID              int64
	BotID           int64
	EventID         string
	EventType       string
	Topic           string
	MsgID           int64
	Status          string
	Attempts        int
	NextAttemptAt   time.Time
	LockedUntil     *time.Time
	LastStatusCode  int
	LastError       string
	ResponseSnippet string
	CreatedAt       time.Time
	DeliveredAt     *time.Time
}

// NormalizeSubscriptions validates and deduplicates bot event subscriptions.
func NormalizeSubscriptions(events []string) ([]string, error) {
	if len(events) == 0 {
		return []string{EventMessageMention, EventMessageDirect}, nil
	}
	seen := make(map[string]struct{}, len(events))
	out := make([]string, 0, len(events))
	for _, raw := range events {
		event := strings.TrimSpace(raw)
		switch event {
		case EventMessageMention, EventMessageDirect:
		default:
			return nil, ErrInvalid
		}
		if _, ok := seen[event]; ok {
			continue
		}
		seen[event] = struct{}{}
		out = append(out, event)
	}
	if len(out) == 0 {
		return nil, ErrInvalid
	}
	return out, nil
}
