package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/agent/domain"
)

const playgroundKeyKind = "playground"

// agentKeyReconcileInterval bounds how long a spec-hash match is trusted. A
// periodic update recreates keys deleted out of band at the gateway.
const agentKeyReconcileInterval = time.Hour

// Bootstrap ensures the gateway customer and playground key exist, syncs the
// catalog, and enables the preferred model for first-run usability.
func (s *Service) Bootstrap(ctx context.Context, preferred string) (*domain.Model, error) {
	if s.Gateway == nil {
		return nil, fmt.Errorf("%w: Bifrost is not configured", ErrInvalidInput)
	}
	if _, err := s.EnsurePlaygroundKey(ctx); err != nil {
		return nil, err
	}
	if _, err := s.syncCatalog(ctx); err != nil {
		return nil, err
	}
	models, err := s.Store.ListModels(ctx, true)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("%w: Bifrost returned no models", ErrInvalidInput)
	}
	picked := pickModel(models, preferred)
	if picked == nil {
		return nil, fmt.Errorf("%w: preferred model %q is unavailable", ErrInvalidInput, preferred)
	}
	// Prefer an operator-curated model. Only auto-enable the bootstrap
	// preference when no usable model exists, so restarts do not override a
	// deliberate disable.
	var usable *domain.Model
	for i := range models {
		if models[i].Enabled && models[i].Curated {
			usable = &models[i]
			break
		}
	}
	if usable != nil {
		if !picked.Enabled || !picked.Curated {
			return usable, nil
		}
		return picked, nil
	}
	enabled := true
	if updated, uerr := s.Store.UpdateModelCuration(ctx, picked.ID, domain.ModelCuration{
		Enabled: &enabled, Recommended: &enabled,
	}); uerr == nil {
		picked = updated
	}
	return picked, nil
}

// EnsurePlaygroundKey creates or reuses the shared draft/test sandbox key.
func (s *Service) EnsurePlaygroundKey(ctx context.Context) (*domain.PlatformGatewayKey, error) {
	if s.Gateway == nil {
		return nil, fmt.Errorf("%w: Bifrost is not configured", ErrInvalidInput)
	}
	existing, err := s.Store.GetPlatformGatewayKey(ctx, playgroundKeyKind)
	if err != nil && !errors.Is(err, domain.ErrGatewayKeyNotFound) {
		return nil, err
	}
	customerID, err := s.ensureGlobalCustomer(ctx)
	if err != nil {
		return nil, err
	}
	spec := VirtualKeySpec{
		Name: s.Config.PlaygroundKeyName, Description: "Nonoka playground sandbox",
		AllowAll: true, CustomerID: customerID,
		Metadata:  map[string]string{"kind": "playground"},
		RateLimit: s.rateLimitSpec(),
	}
	if spec.RateLimit == nil {
		spec.ClearRateLimit = true
	}
	if existing != nil && existing.VirtualKeyID != "" {
		if uerr := s.Gateway.UpdateVirtualKey(ctx, existing.VirtualKeyID, spec, true); uerr == nil {
			existing.Status = domain.GatewayKeyActive
			existing.LastError = ""
			_ = s.Store.UpsertPlatformGatewayKey(ctx, *existing)
			return existing, nil
		} else {
			var ge gatewayError
			if !errors.As(uerr, &ge) || ge.Status != 404 {
				return nil, uerr
			}
		}
	}
	created, err := s.Gateway.CreateVirtualKey(ctx, spec)
	if err != nil {
		return nil, err
	}
	sealed, err := s.seal(created.Value)
	if err != nil {
		_ = s.Gateway.DeleteVirtualKey(ctx, created.ID)
		return nil, err
	}
	key := &domain.PlatformGatewayKey{
		Kind: playgroundKeyKind, VirtualKeyID: created.ID, ValueEnc: sealed,
		Status: domain.GatewayKeyActive, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := s.Store.UpsertPlatformGatewayKey(ctx, *key); err != nil {
		_ = s.Gateway.DeleteVirtualKey(ctx, created.ID)
		return nil, err
	}
	return key, nil
}

// EnsureAgentKey creates or updates the per-agent virtual key and its model
// allowlist. models should be the union of models referenced by the agent's
// published versions.
func (s *Service) EnsureAgentKey(ctx context.Context, agentID int64, models []domain.Model) error {
	if agentID <= 0 {
		return fmt.Errorf("%w: agent is required", ErrInvalidInput)
	}
	if s.Gateway == nil {
		return fmt.Errorf("%w: Bifrost is not configured", ErrInvalidInput)
	}
	spec, err := s.agentKeySpec(ctx, agentID, models)
	if err != nil {
		return err
	}
	specHash, err := spec.Hash()
	if err != nil {
		return err
	}
	existing, err := s.Store.GetAgentGatewayKey(ctx, agentID)
	if err == nil && existing.VirtualKeyID != "" {
		if existing.SpecHash == specHash && existing.Status == domain.GatewayKeyActive &&
			time.Since(existing.UpdatedAt) < agentKeyReconcileInterval {
			return nil
		}
		if uerr := s.Gateway.UpdateVirtualKey(ctx, existing.VirtualKeyID, spec, true); uerr != nil {
			var ge gatewayError
			if errors.As(uerr, &ge) && ge.Status == http.StatusNotFound {
				// The gateway key was deleted out of band; replace it below.
				_ = s.Store.DeleteAgentGatewayKey(ctx, agentID)
			} else {
				_ = s.Store.SetAgentGatewayKeyStatus(ctx, agentID, domain.GatewayKeyDegraded, uerr.Error())
				return uerr
			}
		} else {
			return s.Store.SetAgentGatewayKeySynced(ctx, agentID, specHash)
		}
	}
	if err != nil && !errors.Is(err, domain.ErrGatewayKeyNotFound) {
		return err
	}
	created, err := s.Gateway.CreateVirtualKey(ctx, spec)
	if err != nil {
		return err
	}
	sealed, err := s.seal(created.Value)
	if err != nil {
		_ = s.Gateway.DeleteVirtualKey(ctx, created.ID)
		return err
	}
	row := domain.AgentGatewayKey{
		AgentID: agentID, VirtualKeyID: created.ID, ValueEnc: sealed,
		Status: domain.GatewayKeyActive, SpecHash: specHash, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := s.Store.UpsertAgentGatewayKey(ctx, row); err != nil {
		_ = s.Gateway.DeleteVirtualKey(ctx, created.ID)
		return err
	}
	return nil
}

// RevokeAgentKey deactivates and deletes the per-agent virtual key. Failures
// are queued so agent deletion is never blocked by a gateway outage.
func (s *Service) RevokeAgentKey(ctx context.Context, agentID int64) error {
	key, err := s.Store.GetAgentGatewayKey(ctx, agentID)
	if errors.Is(err, domain.ErrGatewayKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var revokeErr error
	if s.Gateway != nil {
		_ = s.Gateway.UpdateVirtualKey(ctx, key.VirtualKeyID, VirtualKeySpec{Name: "revoked", Description: "revoked", DenyAll: true}, false)
		revokeErr = s.Gateway.DeleteVirtualKey(ctx, key.VirtualKeyID)
	} else {
		revokeErr = fmt.Errorf("Bifrost is not configured")
	}
	if revokeErr != nil {
		_ = s.Store.SetAgentGatewayKeyStatus(ctx, agentID, domain.GatewayKeyRevokedPending, revokeErr.Error())
		_ = s.Store.EnqueueGatewayKeyCleanup(ctx, domain.GatewayKeyCleanup{
			VirtualKeyID: key.VirtualKeyID, AgentID: agentID, Reason: "agent_deleted",
			NextAttemptAt: time.Now(), LastError: revokeErr.Error(), CreatedAt: time.Now(),
		})
		if s.Gateway != nil {
			_ = s.Store.DeleteAgentGatewayKey(ctx, agentID)
		}
	} else {
		_ = s.Store.DeleteAgentGatewayKey(ctx, agentID)
	}
	return revokeErr
}

// CleanupKeys retries failed virtual-key revocations.
func (s *Service) CleanupKeys(ctx context.Context) (int, error) {
	if s.Gateway == nil {
		return 0, nil
	}
	rows, err := s.Store.DueGatewayKeyCleanups(ctx, 20)
	if err != nil {
		return 0, err
	}
	done := 0
	for i := range rows {
		row := rows[i]
		if derr := s.Gateway.DeleteVirtualKey(ctx, row.VirtualKeyID); derr != nil {
			var ge gatewayError
			if errors.As(derr, &ge) && ge.Status == 404 {
				_ = s.Store.CompleteGatewayKeyCleanup(ctx, row.VirtualKeyID)
				done++
				continue
			}
			attempts := row.Attempts + 1
			delay := time.Minute * time.Duration(1<<min(attempts, 6))
			_ = s.Store.FailGatewayKeyCleanup(ctx, row.VirtualKeyID, attempts, time.Now().Add(delay), derr.Error())
			continue
		}
		_ = s.Store.CompleteGatewayKeyCleanup(ctx, row.VirtualKeyID)
		done++
	}
	return done, nil
}

// AgentKeyStatus returns the non-secret key status for the management API.
func (s *Service) AgentKeyStatus(ctx context.Context, agentID int64) (string, error) {
	key, err := s.Store.GetAgentGatewayKey(ctx, agentID)
	if errors.Is(err, domain.ErrGatewayKeyNotFound) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	return key.Status, nil
}

func (s *Service) agentKeySpec(ctx context.Context, agentID int64, models []domain.Model) (VirtualKeySpec, error) {
	providers := map[string][]string{}
	seen := map[string]bool{}
	for i := range models {
		m := models[i]
		if m.Provider == "" || m.GatewayModelKey == "" {
			continue
		}
		k := m.Provider + "\x00" + m.GatewayModelKey
		if seen[k] {
			continue
		}
		seen[k] = true
		providers[m.Provider] = append(providers[m.Provider], m.GatewayModelKey)
	}
	if len(providers) == 0 {
		return VirtualKeySpec{}, fmt.Errorf("%w: agent has no runnable model", ErrInvalidInput)
	}
	for provider := range providers {
		sort.Strings(providers[provider])
	}
	customerID, err := s.ensureGlobalCustomer(ctx)
	if err != nil {
		return VirtualKeySpec{}, err
	}
	spec := VirtualKeySpec{
		Name:           fmt.Sprintf("nonoka-agent-%d", agentID),
		Description:    "Nonoka agent " + strconv.FormatInt(agentID, 10),
		ProviderModels: providers,
		CustomerID:     customerID,
		Metadata:       map[string]string{"agent_id": strconv.FormatInt(agentID, 10), "kind": "agent"},
		RateLimit:      s.rateLimitSpec(),
	}
	if s.Config.AgentMonthlyBudget > 0 {
		spec.Budgets = []BudgetSpec{{MaxLimit: s.Config.AgentMonthlyBudget, ResetDuration: "1M"}}
	} else {
		spec.ClearBudgets = true
	}
	if spec.RateLimit == nil {
		spec.ClearRateLimit = true
	}
	return spec, nil
}

func (s *Service) ensureGlobalCustomer(ctx context.Context) (string, error) {
	if s.Gateway == nil {
		return "", nil
	}
	// The customer budget is deployment configuration, so resolve it once per
	// process instead of issuing a GET+PUT for every agent key backfill.
	s.customerMu.Lock()
	defer s.customerMu.Unlock()
	if s.customerReady {
		return s.customerID, nil
	}
	spec := CustomerSpec{Name: s.Config.CustomerName, Metadata: map[string]string{"kind": "deployment"}}
	if s.Config.GlobalMonthlyBudget > 0 {
		spec.Budgets = []BudgetSpec{{MaxLimit: s.Config.GlobalMonthlyBudget, ResetDuration: "1M"}}
	}
	// Always reconcile the customer, even when the budget is disabled, so a
	// removed BIFROST_GLOBAL_MONTHLY_BUDGET clears an existing hard budget.
	id, err := s.Gateway.EnsureCustomer(ctx, spec)
	if err != nil {
		return "", err
	}
	s.customerID, s.customerReady = id, true
	if s.Config.GlobalMonthlyBudget <= 0 {
		// Keys are not attached to a budgeted customer when the global budget
		// is disabled.
		s.customerID = ""
	}
	return s.customerID, nil
}

func (s *Service) rateLimitSpec() *RateLimitSpec {
	if s.Config.AgentRequestPerMin <= 0 && s.Config.AgentTokensPerMin <= 0 {
		return nil
	}
	return &RateLimitSpec{
		TokenMaxLimit: s.Config.AgentTokensPerMin, TokenResetDuration: "1m",
		RequestMaxLimit: s.Config.AgentRequestPerMin, RequestResetDuration: "1m",
	}
}

func (s *Service) playgroundKeyValue(ctx context.Context) (string, error) {
	key, err := s.EnsurePlaygroundKey(ctx)
	if err != nil {
		return "", err
	}
	return s.open(key.ValueEnc)
}

func (s *Service) agentKeyValue(ctx context.Context, agentID int64) (string, error) {
	key, err := s.Store.GetAgentGatewayKey(ctx, agentID)
	if err != nil {
		return "", err
	}
	if key.Status != "" && key.Status != domain.GatewayKeyActive && key.Status != domain.GatewayKeyDegraded {
		return "", fmt.Errorf("%w: agent gateway key is %s", ErrInvalidInput, key.Status)
	}
	return s.open(key.ValueEnc)
}

func (s *Service) seal(plain string) (string, error) {
	if s.Sealer == nil {
		return "", fmt.Errorf("%w: credential sealer unavailable", ErrInvalidInput)
	}
	return s.Sealer.Seal(plain)
}

func (s *Service) open(sealed string) (string, error) {
	if strings.TrimSpace(sealed) == "" {
		return "", fmt.Errorf("%w: gateway key value is empty", ErrInvalidInput)
	}
	if s.Sealer == nil {
		return "", fmt.Errorf("%w: credential sealer unavailable", ErrInvalidInput)
	}
	return s.Sealer.Open(sealed)
}

func pickModel(models []domain.Model, preferred string) *domain.Model {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		for i := range models {
			m := &models[i]
			if m.ModelKey == preferred || m.GatewayModelKey == preferred || strings.HasSuffix(m.ModelKey, "/"+preferred) {
				return m
			}
		}
	}
	for i := range models {
		if models[i].Enabled {
			return &models[i]
		}
	}
	if len(models) > 0 {
		return &models[0]
	}
	return nil
}
