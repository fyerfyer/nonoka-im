package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nonoka-im/internal/bot/domain"
)

func (s *Service) ListDeliveries(ctx context.Context, userID, botID int64, status string, limit, offset int) ([]domain.Delivery, int64, error) {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, 0, mapNotFound(err)
	}
	if err := s.requireOwnerOrAdmin(ctx, userID, bot); err != nil {
		return nil, 0, err
	}
	switch status {
	case "", domain.DeliveryPending, domain.DeliveryDelivering, domain.DeliverySucceeded, domain.DeliveryFailed:
	default:
		return nil, 0, fmt.Errorf("%w: invalid delivery status", ErrInvalidInput)
	}
	return s.Store.ListDeliveries(ctx, botID, status, limit, offset)
}

func (s *Service) ReplayDelivery(ctx context.Context, userID, botID, deliveryID int64) error {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return mapNotFound(err)
	}
	if err := s.requireOwner(ctx, userID, bot); err != nil {
		return err
	}
	if err := s.Store.ReplayDelivery(ctx, botID, deliveryID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	s.record(ctx, userID, "bot.delivery.replay", "bot", fmt.Sprintf("%d", botID), nil, map[string]any{"delivery_id": deliveryID})
	return nil
}

func (s *Service) Usage(ctx context.Context, userID, botID int64, days int) (map[string]int64, error) {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	if err := s.requireOwnerOrAdmin(ctx, userID, bot); err != nil {
		return nil, err
	}
	if days <= 0 || days > 90 {
		days = 7
	}
	return s.Store.DeliveryStats(ctx, botID, time.Now().AddDate(0, 0, -days))
}
