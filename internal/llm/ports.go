package llm

import (
	"context"
	"errors"

	"nonoka-im/internal/agent/domain"
)

var (
	ErrInvalidInput = errors.New("invalid model service input")
	ErrForbidden    = errors.New("platform admin required")
	ErrNotFound     = errors.New("model service not found")
)

// Store is the persistence surface owned by this package. The platform data
// adapter implements it.
type Store interface {
	ListModelServices(ctx context.Context, includeDisabled bool) ([]domain.ModelService, error)
	GetModelService(ctx context.Context, id uint64) (*domain.ModelService, error)
	GetModelServiceByName(ctx context.Context, name string) (*domain.ModelService, error)
	CreateModelService(ctx context.Context, spec domain.ModelServiceSpec, apiKeyEnc string) (*domain.ModelService, error)
	UpdateModelService(ctx context.Context, id uint64, spec domain.ModelServiceSpec, apiKeyEnc string, updateKey bool) (*domain.ModelService, error)
	DeleteModelService(ctx context.Context, id uint64) error
	ListModels(ctx context.Context, serviceID uint64, includeDisabled bool) ([]domain.Model, error)
	ListAllModels(ctx context.Context, includeDisabled bool) ([]domain.Model, error)
	GetModel(ctx context.Context, id uint64) (*domain.Model, error)
	CreateModel(ctx context.Context, spec domain.ModelSpec) (*domain.Model, error)
	UpdateModel(ctx context.Context, id uint64, spec domain.ModelSpec) (*domain.Model, error)
	UpsertModels(ctx context.Context, serviceID uint64, models []domain.Model) error
	DeleteModel(ctx context.Context, id uint64) error
	ModelServiceReferencedByAgent(ctx context.Context, id uint64) (bool, error)
}

// Sealer encrypts gateway credentials at rest.
type Sealer interface {
	Seal(plain string) (string, error)
	Open(sealed string) (string, error)
}

// Authorizer is the narrow platform-admin check.
type Authorizer interface {
	CanManagePlatform(ctx context.Context, userID int64) error
	SetPlatformAdmin(ctx context.Context, userID int64, enabled bool) error
}

// Gateway is the control-plane client surface. It is nil when no Bifrost
// instance is configured; openai_compatible services still work.
type Gateway interface {
	CreateVirtualKey(ctx context.Context, spec VirtualKeySpec) (*VirtualKey, error)
	UpdateVirtualKey(ctx context.Context, id string, spec VirtualKeySpec, active bool) error
	DeleteVirtualKey(ctx context.Context, id string) error
	ListProviderModels(ctx context.Context) ([]ProviderModel, error)
	ListGatewayModels(ctx context.Context, virtualKey string) ([]GatewayModel, error)
}

// CatalogService groups a service with its selectable models for the API.
type CatalogService struct {
	Service domain.ModelService
	Models  []domain.Model
}

type Service struct {
	Store      Store
	Sealer     Sealer
	Authz      Authorizer
	Gateway    Gateway
	GatewayURL string
}

func NewService(store Store, sealer Sealer, authz Authorizer, gateway Gateway, gatewayURL string) *Service {
	return &Service{Store: store, Sealer: sealer, Authz: authz, Gateway: gateway, GatewayURL: gatewayURL}
}

func (s *Service) requireAdmin(ctx context.Context, userID int64) error {
	if s.Authz == nil {
		return ErrForbidden
	}
	if err := s.Authz.CanManagePlatform(ctx, userID); err != nil {
		return ErrForbidden
	}
	return nil
}

// Catalog lists services and models. Non-admin callers only see enabled
// entries and never receive the gateway URL; the transport additionally omits
// the sealed key.
