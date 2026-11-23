package llm

import (
	"context"
	"fmt"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
)

func (s *Service) Lookup(ctx context.Context, serviceID, modelID int64) (domain.ModelService, domain.Model, error) {
	if serviceID <= 0 || modelID <= 0 {
		return domain.ModelService{}, domain.Model{}, fmt.Errorf("%w: model service/model is required", ErrInvalidInput)
	}
	service, err := s.Store.GetModelService(ctx, uint64(serviceID))
	if err != nil {
		return domain.ModelService{}, domain.Model{}, err
	}
	m, err := s.Store.GetModel(ctx, uint64(modelID))
	if err != nil {
		return domain.ModelService{}, domain.Model{}, err
	}
	if m.ServiceID != service.ID {
		return domain.ModelService{}, domain.Model{}, fmt.Errorf("%w: model does not belong to the service", ErrInvalidInput)
	}
	return *service, *m, nil
}

// Resolve returns the runtime endpoint for one published AgentVersion. It
// fails closed when the service or model is disabled.
func (s *Service) Resolve(ctx context.Context, serviceID, modelID int64) (port.ModelEndpoint, error) {
	if serviceID <= 0 || modelID <= 0 {
		return port.ModelEndpoint{}, fmt.Errorf("%w: model service/model is not configured", ErrInvalidInput)
	}
	service, err := s.Store.GetModelService(ctx, uint64(serviceID))
	if err != nil {
		return port.ModelEndpoint{}, err
	}
	if !service.Enabled {
		return port.ModelEndpoint{}, fmt.Errorf("%w: model service %q is disabled", ErrInvalidInput, service.Name)
	}
	model, err := s.Store.GetModel(ctx, uint64(modelID))
	if err != nil {
		return port.ModelEndpoint{}, err
	}
	if model.ServiceID != service.ID {
		return port.ModelEndpoint{}, fmt.Errorf("%w: model does not belong to the service", ErrInvalidInput)
	}
	if !model.Enabled {
		return port.ModelEndpoint{}, fmt.Errorf("%w: model %q is disabled", ErrInvalidInput, model.ModelKey)
	}
	key, err := s.openKey(service)
	if err != nil {
		return port.ModelEndpoint{}, err
	}
	return port.ModelEndpoint{
		BaseURL: service.BaseURL, APIKey: key, Model: model.ModelKey,
		MaxTokens: model.MaxOutputTokens, ContextWindow: model.ContextWindow,
	}, nil
}

func (s *Service) openKey(service *domain.ModelService) (string, error) {
	if service.APIKeyEnc == "" {
		return "", nil
	}
	if s.Sealer == nil {
		return "", fmt.Errorf("%w: credential sealer unavailable", ErrInvalidInput)
	}
	key, err := s.Sealer.Open(service.APIKeyEnc)
	if err != nil {
		return "", fmt.Errorf("unseal model service key: %w", err)
	}
	return key, nil
}
