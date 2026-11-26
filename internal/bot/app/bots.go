package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nonoka-im/internal/bot/domain"
)

type CreateInput struct {
	Name               string
	Description        string
	Username           string
	EventSubscriptions []string
}

func (s *Service) Create(ctx context.Context, ownerID int64, input CreateInput) (*domain.Bot, error) {
	if ownerID <= 0 {
		return nil, ErrForbidden
	}
	name, err := validateName(input.Name)
	if err != nil {
		return nil, err
	}
	if len(input.Description) > maxBotDescription {
		return nil, fmt.Errorf("%w: description is too long", ErrInvalidInput)
	}
	events, err := domain.NormalizeSubscriptions(input.EventSubscriptions)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid event subscription", ErrInvalidInput)
	}
	username, generated, err := normalizeUsername(input.Username)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		bot, err := s.Store.CreateBot(ctx, ownerID, username, name, strings.TrimSpace(input.Description), events)
		if err == nil {
			s.record(ctx, ownerID, "bot.create", "bot", fmt.Sprintf("%d", bot.ID), nil, map[string]any{
				"name": bot.Name, "username": bot.Username, "status": bot.Status,
			})
			return bot, nil
		}
		if !errors.Is(err, domain.ErrConflict) {
			return nil, err
		}
		if !generated {
			return nil, ErrConflict
		}
		username, err = randomUsername()
		if err != nil {
			return nil, err
		}
	}
	return nil, ErrConflict
}

func (s *Service) List(ctx context.Context, userID int64, all bool, limit, offset int) ([]domain.Bot, int64, error) {
	if all {
		if err := s.requireAdmin(ctx, userID); err != nil {
			return nil, 0, err
		}
		return s.Store.ListAllBots(ctx, limit, offset)
	}
	return s.Store.ListBots(ctx, userID, limit, offset)
}

func (s *Service) Detail(ctx context.Context, userID, botID int64) (*domain.Bot, error) {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	if err := s.requireOwnerOrAdmin(ctx, userID, bot); err != nil {
		return nil, err
	}
	return bot, nil
}

type UpdateInput struct {
	Name               string
	Description        string
	Status             string
	EventSubscriptions []string
}

func (s *Service) Update(ctx context.Context, userID, botID int64, input UpdateInput) (*domain.Bot, error) {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, mapNotFound(err)
	}
	if err := s.requireOwner(ctx, userID, bot); err != nil {
		return nil, err
	}
	name, err := validateName(input.Name)
	if err != nil {
		return nil, err
	}
	if len(input.Description) > maxBotDescription {
		return nil, fmt.Errorf("%w: description is too long", ErrInvalidInput)
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = bot.Status
	}
	if status != domain.StatusActive && status != domain.StatusDisabled {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}
	events, err := domain.NormalizeSubscriptions(input.EventSubscriptions)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid event subscription", ErrInvalidInput)
	}
	if err := s.Store.UpdateBot(ctx, botID, name, strings.TrimSpace(input.Description), status, events); err != nil {
		return nil, err
	}
	updated, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return nil, err
	}
	s.record(ctx, userID, "bot.update", "bot", fmt.Sprintf("%d", botID), map[string]any{
		"name": bot.Name, "status": bot.Status, "events": bot.EventSubscriptions,
	}, map[string]any{
		"name": updated.Name, "status": updated.Status, "events": updated.EventSubscriptions,
	})
	return updated, nil
}

func (s *Service) Disable(ctx context.Context, userID, botID int64) error {
	bot, err := s.Store.GetBot(ctx, botID)
	if err != nil {
		return mapNotFound(err)
	}
	if err := s.requireOwner(ctx, userID, bot); err != nil {
		return err
	}
	if err := s.Store.DisableBot(ctx, botID); err != nil {
		return mapNotFound(err)
	}
	s.record(ctx, userID, "bot.delete", "bot", fmt.Sprintf("%d", botID), map[string]any{"status": bot.Status}, map[string]any{"status": domain.StatusDisabled})
	return nil
}

func (s *Service) Directory(ctx context.Context, userID int64, limit, offset int) ([]domain.Bot, error) {
	if userID <= 0 {
		return nil, ErrForbidden
	}
	// The directory is intentionally small and public to authenticated users.
	bots, _, err := s.Store.ListAllBots(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Bot, 0, len(bots))
	for _, bot := range bots {
		if bot.Status == domain.StatusActive {
			out = append(out, bot)
		}
	}
	return out, nil
}
