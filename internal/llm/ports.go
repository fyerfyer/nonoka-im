package llm

import (
	"context"
	"errors"
	"sync"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/audit"
)

var (
	ErrInvalidInput = errors.New("invalid model input")
	ErrForbidden    = errors.New("platform admin required")
	ErrNotFound     = errors.New("model not found")
)

// Store is the persistence surface owned by this package. The platform data
// adapter implements it.
type Store interface {
	ListModels(ctx context.Context, includeDisabled bool) ([]domain.Model, error)
	GetModel(ctx context.Context, id uint64) (*domain.Model, error)
	GetModelByKey(ctx context.Context, modelKey string) (*domain.Model, error)
	UpsertModels(ctx context.Context, models []domain.Model) error
	UpdateModelRouting(ctx context.Context, id uint64, modelKey, provider, gatewayModelKey string) error
	MergeModel(ctx context.Context, fromID, toID uint64) error
	UpdateModelCuration(ctx context.Context, id uint64, curation domain.ModelCuration) (*domain.Model, error)

	GetAgentGatewayKey(ctx context.Context, agentID int64) (*domain.AgentGatewayKey, error)
	UpsertAgentGatewayKey(ctx context.Context, key domain.AgentGatewayKey) error
	SetAgentGatewayKeyStatus(ctx context.Context, agentID int64, status, lastErr string) error
	SetAgentGatewayKeySynced(ctx context.Context, agentID int64, specHash string) error
	DeleteAgentGatewayKey(ctx context.Context, agentID int64) error

	GetPlatformGatewayKey(ctx context.Context, kind string) (*domain.PlatformGatewayKey, error)
	UpsertPlatformGatewayKey(ctx context.Context, key domain.PlatformGatewayKey) error

	EnqueueGatewayKeyCleanup(ctx context.Context, cleanup domain.GatewayKeyCleanup) error
	DueGatewayKeyCleanups(ctx context.Context, limit int) ([]domain.GatewayKeyCleanup, error)
	CompleteGatewayKeyCleanup(ctx context.Context, virtualKeyID string) error
	FailGatewayKeyCleanup(ctx context.Context, virtualKeyID string, attempts int, next time.Time, lastErr string) error
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

// Gateway is the Bifrost control-plane client surface. It is nil when no
// Bifrost instance is configured; runtime resolution then fails closed.
type Gateway interface {
	CreateVirtualKey(ctx context.Context, spec VirtualKeySpec) (*VirtualKey, error)
	UpdateVirtualKey(ctx context.Context, id string, spec VirtualKeySpec, active bool) error
	DeleteVirtualKey(ctx context.Context, id string) error
	EnsureCustomer(ctx context.Context, spec CustomerSpec) (string, error)
	ListGatewayModels(ctx context.Context, virtualKey string) ([]GatewayModel, error)
}

// Config is the platform gateway policy. Zero values disable the corresponding
// safety limit.
type Config struct {
	BaseURL             string
	AgentMonthlyBudget  float64
	GlobalMonthlyBudget float64
	AgentRequestPerMin  int
	AgentTokensPerMin   int
	PlaygroundKeyName   string
	CustomerName        string
}

type Service struct {
	Store   Store
	Sealer  Sealer
	Authz   Authorizer
	Gateway Gateway
	Config  Config
	Audit   *audit.Service

	customerMu    sync.Mutex
	customerID    string
	customerReady bool
}

func NewService(store Store, sealer Sealer, authz Authorizer, gateway Gateway, cfg Config) *Service {
	if cfg.PlaygroundKeyName == "" {
		cfg.PlaygroundKeyName = "nonoka-playground"
	}
	if cfg.CustomerName == "" {
		cfg.CustomerName = "nonoka"
	}
	cfg.BaseURL = trimBaseURL(cfg.BaseURL)
	return &Service{Store: store, Sealer: sealer, Authz: authz, Gateway: gateway, Config: cfg}
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

// EnsurePlatformAdmins grants the OpenFGA platform-admin relation to the
// configured operator IDs. It is idempotent and used at startup.
func (s *Service) EnsurePlatformAdmins(ctx context.Context, ids []int64) error {
	if s.Authz == nil {
		return nil
	}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if err := s.Authz.SetPlatformAdmin(ctx, id, true); err != nil {
			return err
		}
	}
	return nil
}
