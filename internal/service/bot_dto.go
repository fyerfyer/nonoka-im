package service

import (
	"fmt"
	"time"

	botapp "nonoka-im/internal/bot/app"
	"nonoka-im/internal/bot/domain"
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
