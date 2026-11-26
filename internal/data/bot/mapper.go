package botdata

import (
	"encoding/json"

	"nonoka-im/internal/bot/domain"
)

func toDomainBot(row botRow) domain.Bot {
	var events []string
	_ = json.Unmarshal([]byte(row.EventSubscriptions), &events)
	return domain.Bot{
		ID: row.ID, BotUserID: row.BotUserID, OwnerUserID: row.OwnerUserID,
		Username: row.Username, Name: row.Name, Description: row.Description,
		Status: row.Status, EventSubscriptions: events,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func toDomainToken(row tokenRow) domain.Token {
	var scopes []string
	_ = json.Unmarshal([]byte(row.Scopes), &scopes)
	return domain.Token{
		ID: row.ID, BotID: row.BotID, Name: row.Name, Prefix: row.Prefix,
		Scopes: scopes, ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt,
		LastUsedAt: row.LastUsedAt, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
	}
}

func toDomainWebhook(row webhookRow) domain.Webhook {
	var events []string
	_ = json.Unmarshal([]byte(row.Events), &events)
	return domain.Webhook{
		ID: row.ID, BotID: row.BotID, URL: row.URL, SecretPrefix: row.SecretPrefix,
		Events: events, Enabled: row.Enabled, VerifiedAt: row.VerifiedAt,
		LastSuccessAt: row.LastSuccessAt, ConsecutiveFailures: row.ConsecutiveFailures,
		DisabledAt: row.DisabledAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func toDomainDelivery(row deliveryRow) domain.Delivery {
	out := domain.Delivery{
		ID: row.ID, BotID: row.BotID, EventID: row.EventID, EventType: row.EventType,
		Topic: row.Topic, MsgID: row.MsgID, Status: row.Status, Attempts: row.Attempts,
		NextAttemptAt: row.NextAttemptAt, LockedUntil: row.LockedUntil,
		LastError: row.LastError, ResponseSnippet: row.ResponseSnippet,
		CreatedAt: row.CreatedAt, DeliveredAt: row.DeliveredAt,
	}
	if row.LastStatusCode != nil {
		out.LastStatusCode = *row.LastStatusCode
	}
	return out
}
