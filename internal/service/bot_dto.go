package service

import (
	"fmt"
	"time"

	botapp "nonoka-im/internal/bot/app"
	"nonoka-im/internal/bot/domain"
	"nonoka-im/internal/httpapi"
)

type botDTO struct {
	ID                 string   `json:"id"`
	BotUserID          string   `json:"botUserId"`
	OwnerUserID        string   `json:"ownerUserId"`
	Username           string   `json:"username"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Status             string   `json:"status"`
	EventSubscriptions []string `json:"eventSubscriptions"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
}

func botDTOFrom(bot *domain.Bot) botDTO {
	if bot == nil {
		return botDTO{}
	}
	return botDTO{
		ID: fmt.Sprintf("%d", bot.ID), BotUserID: fmt.Sprintf("%d", bot.BotUserID),
		OwnerUserID: fmt.Sprintf("%d", bot.OwnerUserID), Username: bot.Username,
		Name: bot.Name, Description: bot.Description, Status: bot.Status,
		EventSubscriptions: bot.EventSubscriptions,
		CreatedAt:          bot.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          bot.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func botDTOs(bots []domain.Bot) []botDTO {
	out := make([]botDTO, 0, len(bots))
	for i := range bots {
		out = append(out, botDTOFrom(&bots[i]))
	}
	return out
}

type tokenDTO struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  string   `json:"expiresAt,omitempty"`
	RevokedAt  string   `json:"revokedAt,omitempty"`
	LastUsedAt string   `json:"lastUsedAt,omitempty"`
	CreatedAt  string   `json:"createdAt"`
}

func tokenDTOFrom(token *domain.Token) tokenDTO {
	if token == nil {
		return tokenDTO{}
	}
	out := tokenDTO{
		ID: token.ID, Name: token.Name, Prefix: token.Prefix, Scopes: token.Scopes,
		CreatedAt: token.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if token.ExpiresAt != nil {
		out.ExpiresAt = token.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if token.RevokedAt != nil {
		out.RevokedAt = token.RevokedAt.UTC().Format(time.RFC3339Nano)
	}
	if token.LastUsedAt != nil {
		out.LastUsedAt = token.LastUsedAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

type webhookDTO struct {
	ID                  string   `json:"id"`
	URL                 string   `json:"url"`
	SecretPrefix        string   `json:"secretPrefix"`
	Events              []string `json:"events"`
	Enabled             bool     `json:"enabled"`
	VerifiedAt          string   `json:"verifiedAt,omitempty"`
	LastSuccessAt       string   `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures int      `json:"consecutiveFailures"`
	DisabledAt          string   `json:"disabledAt,omitempty"`
	CreatedAt           string   `json:"createdAt"`
}

func webhookDTOFrom(webhook *domain.Webhook) webhookDTO {
	if webhook == nil {
		return webhookDTO{}
	}
	out := webhookDTO{
		ID: fmt.Sprintf("%d", webhook.ID), URL: webhook.URL, SecretPrefix: webhook.SecretPrefix,
		Events: webhook.Events, Enabled: webhook.Enabled,
		ConsecutiveFailures: webhook.ConsecutiveFailures,
		CreatedAt:           webhook.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if webhook.VerifiedAt != nil {
		out.VerifiedAt = webhook.VerifiedAt.UTC().Format(time.RFC3339Nano)
	}
	if webhook.LastSuccessAt != nil {
		out.LastSuccessAt = webhook.LastSuccessAt.UTC().Format(time.RFC3339Nano)
	}
	if webhook.DisabledAt != nil {
		out.DisabledAt = webhook.DisabledAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

type deliveryDTO struct {
	ID              string `json:"id"`
	EventID         string `json:"eventId"`
	EventType       string `json:"eventType"`
	Topic           string `json:"topic"`
	MsgID           string `json:"msgId"`
	Status          string `json:"status"`
	Attempts        int    `json:"attempts"`
	NextAttemptAt   string `json:"nextAttemptAt"`
	LastStatusCode  int    `json:"lastStatusCode,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	ResponseSnippet string `json:"responseSnippet,omitempty"`
	CreatedAt       string `json:"createdAt"`
	DeliveredAt     string `json:"deliveredAt,omitempty"`
}

func deliveryDTOFrom(delivery *domain.Delivery) deliveryDTO {
	if delivery == nil {
		return deliveryDTO{}
	}
	out := deliveryDTO{
		ID: fmt.Sprintf("%d", delivery.ID), EventID: delivery.EventID,
		EventType: delivery.EventType, Topic: delivery.Topic, MsgID: fmt.Sprintf("%d", delivery.MsgID),
		Status: delivery.Status, Attempts: delivery.Attempts,
		NextAttemptAt:  delivery.NextAttemptAt.UTC().Format(time.RFC3339Nano),
		LastStatusCode: delivery.LastStatusCode, LastError: delivery.LastError,
		ResponseSnippet: delivery.ResponseSnippet,
		CreatedAt:       delivery.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if delivery.DeliveredAt != nil {
		out.DeliveredAt = delivery.DeliveredAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

type messageDTO struct {
	MessageID string `json:"messageId"`
	Topic     string `json:"topic"`
	TopicSeq  string `json:"topicSeq"`
	SenderID  string `json:"senderId"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

func messageDTOFrom(msg botapp.Message) messageDTO {
	return messageDTO{
		MessageID: fmt.Sprintf("%d", msg.ID), Topic: msg.Topic, TopicSeq: fmt.Sprintf("%d", msg.TopicSeq),
		SenderID: fmt.Sprintf("%d", msg.SenderID), Type: messageTypeName(msg.Type),
		Content: string(msg.Content), CreatedAt: time.Unix(msg.Timestamp, 0).UTC().Format(time.RFC3339Nano),
	}
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

type listResponse struct {
	Items      any    `json:"items"`
	Total      int64  `json:"total"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

func nextCursor(offset, count int, total int64) string {
	if int64(offset+count) >= total {
		return ""
	}
	return httpapi.EncodeCursor(offset + count)
}
