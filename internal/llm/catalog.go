package llm

import (
	"context"
	"fmt"

	"nonoka-im/internal/agent/domain"
)

func (s *Service) Catalog(ctx context.Context, userID int64) ([]CatalogService, bool, error) {
	admin := s.Authz != nil && s.Authz.CanManagePlatform(ctx, userID) == nil
	services, err := s.Store.ListModelServices(ctx, admin)
	if err != nil {
		return nil, admin, err
	}
	allModels, err := s.Store.ListAllModels(ctx, admin)
	if err != nil {
		return nil, admin, err
	}
	byService := make(map[uint64][]domain.Model)
	for _, m := range allModels {
		byService[m.ServiceID] = append(byService[m.ServiceID], m)
	}
	out := make([]CatalogService, 0, len(services))
	for i := range services {
		service := services[i]
		if !admin {
			service.BaseURL = ""
			service.GatewayKeyID = ""
			service.APIKeyEnc = ""
		}
		out = append(out, CatalogService{Service: service, Models: byService[service.ID]})
	}
	return out, admin, nil
}

func (s *Service) ListModels(ctx context.Context, userID int64, serviceID uint64) ([]domain.Model, bool, error) {
	admin := s.Authz != nil && s.Authz.CanManagePlatform(ctx, userID) == nil
	service, err := s.Store.GetModelService(ctx, serviceID)
	if err != nil {
		return nil, admin, err
	}
	if !admin && !service.Enabled {
		return nil, admin, ErrNotFound
	}
	models, err := s.Store.ListModels(ctx, serviceID, admin)
	return models, admin, err
}

func (s *Service) AvailableProviderModels(ctx context.Context, userID int64) ([]ProviderModel, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	if s.Gateway == nil {
		return nil, fmt.Errorf("%w: Bifrost is not configured", ErrInvalidInput)
	}
	return s.Gateway.ListProviderModels(ctx)
}
