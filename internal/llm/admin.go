package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nonoka-im/internal/agent/domain"
)

func (s *Service) Create(ctx context.Context, userID int64, spec domain.ModelServiceSpec) (*domain.ModelService, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	if err := normalizeSpec(&spec); err != nil {
		return nil, err
	}
	switch spec.GatewayType {
	case "bifrost":
		return s.createBifrost(ctx, userID, spec)
	case "openai_compatible":
		return s.createManual(ctx, userID, spec)
	default:
		return nil, fmt.Errorf("%w: gateway_type must be bifrost or openai_compatible", ErrInvalidInput)
	}
}

func (s *Service) createBifrost(ctx context.Context, userID int64, spec domain.ModelServiceSpec) (*domain.ModelService, error) {
	if s.Gateway == nil || s.GatewayURL == "" {
		return nil, fmt.Errorf("%w: Bifrost is not configured", ErrInvalidInput)
	}
	if spec.APIKey != "" {
		return nil, fmt.Errorf("%w: Bifrost model service credentials are issued by the gateway", ErrInvalidInput)
	}
	vk, err := s.Gateway.CreateVirtualKey(ctx, VirtualKeySpec{
		Name: spec.Name, Description: spec.DisplayName,
		ProviderModels: spec.ProviderModels, AllowAll: spec.AllowAll,
	})
	if err != nil {
		return nil, err
	}
	sealed, err := s.Sealer.Seal(vk.Value)
	if err != nil {
		_ = s.Gateway.DeleteVirtualKey(ctx, vk.ID)
		return nil, fmt.Errorf("seal gateway virtual key: %w", err)
	}
	spec.BaseURL = strings.TrimRight(s.GatewayURL, "/") + "/v1"
	spec.GatewayKeyID = vk.ID
	service, err := s.Store.CreateModelService(ctx, spec, sealed)
	if err != nil {
		_ = s.Gateway.DeleteVirtualKey(ctx, vk.ID)
		return nil, err
	}
	if err := s.sync(ctx, service.ID); err != nil {
		// Keep the service and surface the sync error: the virtual key and
		// local row are valid, and the admin can retry sync without creating a
		// second key.
		return service, fmt.Errorf("model service created but model sync failed: %w", err)
	}
	return s.Store.GetModelService(ctx, service.ID)
}

func (s *Service) createManual(ctx context.Context, userID int64, spec domain.ModelServiceSpec) (*domain.ModelService, error) {
	if spec.BaseURL == "" {
		return nil, fmt.Errorf("%w: base_url is required", ErrInvalidInput)
	}
	sealed := ""
	if strings.TrimSpace(spec.APIKey) != "" {
		var err error
		sealed, err = s.Sealer.Seal(spec.APIKey)
		if err != nil {
			return nil, err
		}
	}
	service, err := s.Store.CreateModelService(ctx, spec, sealed)
	if err != nil {
		return nil, err
	}
	// A manual service starts empty; the admin adds models explicitly.
	return service, nil
}

func (s *Service) Update(ctx context.Context, userID int64, id uint64, spec domain.ModelServiceSpec) (*domain.ModelService, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	existing, err := s.Store.GetModelService(ctx, id)
	if err != nil {
		return nil, err
	}
	if spec.Name == "" {
		spec.Name = existing.Name
	}
	if spec.DisplayName == "" {
		spec.DisplayName = existing.DisplayName
	}
	if spec.GatewayType == "" {
		spec.GatewayType = existing.GatewayType
	}
	if spec.BaseURL == "" {
		spec.BaseURL = existing.BaseURL
	}
	if err := normalizeSpec(&spec); err != nil {
		return nil, err
	}
	if spec.GatewayKeyID == "" {
		spec.GatewayKeyID = existing.GatewayKeyID
	}
	if spec.GatewayType != existing.GatewayType {
		return nil, fmt.Errorf("%w: gateway_type cannot change", ErrInvalidInput)
	}
	if spec.GatewayType == "bifrost" {
		if s.Gateway == nil {
			return nil, fmt.Errorf("%w: Bifrost is not configured", ErrInvalidInput)
		}
		if spec.GatewayKeyID == "" {
			spec.GatewayKeyID = existing.GatewayKeyID
		}
		if spec.BaseURL == "" {
			spec.BaseURL = strings.TrimRight(s.GatewayURL, "/") + "/v1"
		}
		if spec.ProviderModels != nil || spec.AllowAll {
			active := true
			if spec.Enabled != nil {
				active = *spec.Enabled
			}
			if err := s.Gateway.UpdateVirtualKey(ctx, existing.GatewayKeyID, VirtualKeySpec{
				Name: spec.Name, Description: spec.DisplayName,
				ProviderModels: spec.ProviderModels, AllowAll: spec.AllowAll,
			}, active); err != nil {
				return nil, err
			}
		}
	}
	sealed := ""
	updateKey := false
	if spec.UpdateAPIKey {
		if strings.TrimSpace(spec.APIKey) == "" {
			return nil, fmt.Errorf("%w: api_key is empty", ErrInvalidInput)
		}
		sealed, err = s.Sealer.Seal(spec.APIKey)
		if err != nil {
			return nil, err
		}
		updateKey = true
	}
	return s.Store.UpdateModelService(ctx, id, spec, sealed, updateKey)
}

func (s *Service) Delete(ctx context.Context, userID int64, id uint64) error {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return err
	}
	service, err := s.Store.GetModelService(ctx, id)
	if err != nil {
		return err
	}
	inUse, err := s.Store.ModelServiceReferencedByAgent(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return domain.ErrModelServiceInUse
	}
	if service.GatewayType == "bifrost" && service.GatewayKeyID != "" && s.Gateway != nil {
		if err := s.Gateway.DeleteVirtualKey(ctx, service.GatewayKeyID); err != nil {
			return err
		}
	}
	return s.Store.DeleteModelService(ctx, id)
}

// SyncModels refreshes the local catalog from the gateway. The local enabled
// flag is preserved by the store upsert.
func (s *Service) SyncModels(ctx context.Context, userID int64, id uint64) error {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return err
	}
	return s.sync(ctx, id)
}

func (s *Service) sync(ctx context.Context, id uint64) error {
	service, err := s.Store.GetModelService(ctx, id)
	if err != nil {
		return err
	}
	if service.GatewayType != "bifrost" || s.Gateway == nil {
		return nil
	}
	key, err := s.openKey(service)
	if err != nil {
		return err
	}
	remote, err := s.Gateway.ListGatewayModels(ctx, key)
	if err != nil {
		return err
	}
	models := make([]domain.Model, 0, len(remote))
	for _, m := range remote {
		if strings.TrimSpace(m.ID) == "" {
			continue
		}
		display := m.NormalizedName
		if display == "" {
			display = m.ID
		}
		models = append(models, domain.Model{
			ServiceID:          id,
			ModelKey:           m.ID,
			DisplayName:        display,
			ContextWindow:      m.ContextLength,
			MaxOutputTokens:    defaultMaxOutput(m.MaxOutputTokens),
			SupportsTools:      true,
			InputPricePerMTok:  PricePerMTok(m.Pricing.Prompt),
			OutputPricePerMTok: PricePerMTok(m.Pricing.Completion),
			Enabled:            true,
			LastSyncedAt:       ptrTime(time.Now()),
		})
	}
	return s.Store.UpsertModels(ctx, id, models)
}

func defaultMaxOutput(v int) int {
	if v <= 0 {
		return 4096
	}
	return v
}

func ptrTime(t time.Time) *time.Time { return &t }
