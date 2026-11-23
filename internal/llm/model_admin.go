package llm

import (
	"context"

	"nonoka-im/internal/agent/domain"
)

func (s *Service) CreateModel(ctx context.Context, userID int64, spec domain.ModelSpec) (*domain.Model, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	if err := normalizeModelSpec(&spec); err != nil {
		return nil, err
	}
	if _, err := s.Store.GetModelService(ctx, spec.ServiceID); err != nil {
		return nil, err
	}
	return s.Store.CreateModel(ctx, spec)
}

func (s *Service) UpdateModel(ctx context.Context, userID int64, id uint64, spec domain.ModelSpec) (*domain.Model, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	if err := normalizeModelSpec(&spec); err != nil {
		return nil, err
	}
	existing, err := s.Store.GetModel(ctx, id)
	if err != nil {
		return nil, err
	}
	if spec.ServiceID == 0 {
		spec.ServiceID = existing.ServiceID
	}
	if spec.ModelKey == "" {
		spec.ModelKey = existing.ModelKey
	}
	return s.Store.UpdateModel(ctx, id, spec)
}

func (s *Service) DeleteModel(ctx context.Context, userID int64, id uint64) error {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return err
	}
	return s.Store.DeleteModel(ctx, id)
}

// Lookup returns the service/model metadata without decrypting credentials.
// Management usecases use it to validate references and enforce output limits.
