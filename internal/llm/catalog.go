package llm

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/audit"
)

// Catalog returns the local curated model catalog. Non-admin callers only see
// enabled models.
func (s *Service) Catalog(ctx context.Context, userID int64) ([]domain.Model, bool, error) {
	admin := s.Authz != nil && s.Authz.CanManagePlatform(ctx, userID) == nil
	models, err := s.Store.ListModels(ctx, admin)
	if err != nil {
		return nil, admin, err
	}
	if !admin {
		filtered := models[:0]
		for _, m := range models {
			if m.Enabled && m.Curated {
				filtered = append(filtered, m)
			}
		}
		models = filtered
	}
	return models, admin, nil
}

// Lookup validates one model reference for agent draft/publish.
func (s *Service) Lookup(ctx context.Context, modelID int64) (*domain.Model, error) {
	if modelID <= 0 {
		return nil, fmt.Errorf("%w: model is required", ErrInvalidInput)
	}
	model, err := s.Store.GetModel(ctx, uint64(modelID))
	if err != nil {
		return nil, err
	}
	return model, nil
}

// UpdateCuration edits the platform-owned part of a model catalog entry.
func (s *Service) UpdateCuration(ctx context.Context, userID int64, id uint64, curation domain.ModelCuration) (*domain.Model, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return nil, err
	}
	before, _ := s.Store.GetModel(ctx, id)
	model, err := s.Store.UpdateModelCuration(ctx, id, curation)
	if err != nil {
		return nil, err
	}
	if s.Audit != nil {
		if err := s.Audit.Record(ctx, audit.Event{
			ActorUserID: userID, Action: "model.update", ResourceType: "model", ResourceID: fmt.Sprintf("%d", id),
			Before: modelAuditFields(before), After: modelAuditFields(model),
		}); err != nil {
			return nil, err
		}
	}
	return model, nil
}

func (s *Service) SyncCatalog(ctx context.Context, userID int64) (int, error) {
	if err := s.requireAdmin(ctx, userID); err != nil {
		return 0, err
	}
	n, err := s.syncCatalog(ctx)
	if err == nil && s.Audit != nil {
		if aerr := s.Audit.Record(ctx, audit.Event{
			ActorUserID: userID, Action: "model.sync", ResourceType: "model", ResourceID: "catalog",
			After: map[string]any{"synced": n},
		}); aerr != nil {
			return n, aerr
		}
	}
	return n, err
}

func modelAuditFields(m *domain.Model) map[string]any {
	if m == nil {
		return nil
	}
	return map[string]any{
		"modelKey": m.ModelKey, "provider": m.Provider, "displayName": m.DisplayName,
		"enabled": m.Enabled, "curated": m.Curated, "recommended": m.Recommended,
		"supportsVision": m.SupportsVision, "supportsTools": m.SupportsTools,
	}
}

func (s *Service) syncCatalog(ctx context.Context) (int, error) {
	if s.Gateway == nil {
		return 0, fmt.Errorf("%w: Bifrost is not configured", ErrInvalidInput)
	}
	key, err := s.playgroundKeyValue(ctx)
	if err != nil {
		return 0, err
	}
	remote, err := s.Gateway.ListGatewayModels(ctx, key)
	if err != nil {
		return 0, err
	}
	models := make([]domain.Model, 0, len(remote))
	for i := range remote {
		m := remote[i]
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		provider := strings.TrimSpace(m.OwnedBy)
		gatewayKey := id
		if provider == "" && strings.Contains(id, "/") {
			provider = strings.SplitN(id, "/", 2)[0]
		}
		if strings.Contains(id, "/") {
			gatewayKey = strings.SplitN(id, "/", 2)[1]
		}
		display := strings.TrimSpace(m.NormalizedName)
		if display == "" {
			display = id
		}
		supportsVision := slices.Contains(m.Architecture.InputModalities, "image")
		supportsTools := true
		if len(m.SupportedParameters) > 0 {
			supportsTools = slices.Contains(m.SupportedParameters, "tools") || slices.Contains(m.SupportedParameters, "tool_choice")
		}
		models = append(models, domain.Model{
			ModelKey: id, Provider: provider, GatewayModelKey: gatewayKey,
			DisplayName: display, ContextWindow: m.ContextLength,
			MaxOutputTokens:            defaultMaxOutput(m.MaxOutputTokens),
			SupportsTools:              supportsTools,
			SupportsVision:             supportsVision,
			InputPricePerMTok:          PricePerMTok(m.Pricing.Prompt),
			OutputPricePerMTok:         PricePerMTok(m.Pricing.Completion),
			InputCacheReadPricePerMTok: PricePerMTok(m.Pricing.CacheRead),
			Enabled:                    false, Curated: false, LastSyncedAt: ptrTime(time.Now()),
		})
	}
	if err := s.Store.UpsertModels(ctx, models); err != nil {
		return 0, err
	}
	if err := s.reconcileLegacyModels(ctx, models); err != nil {
		return len(models), fmt.Errorf("repair legacy model catalog: %w", err)
	}
	return len(models), nil
}

// reconcileLegacyModels repairs pre-migration rows that were stored without a
// provider prefix. When a synced Bifrost model matches a legacy key, the legacy
// row is repointed to the full gateway key and the duplicate sync row is
// removed only if no agent references it.
func (s *Service) reconcileLegacyModels(ctx context.Context, remote []domain.Model) error {
	local, err := s.Store.ListModels(ctx, true)
	if err != nil {
		return err
	}
	var firstErr error
	for i := range local {
		row := local[i]
		if row.Provider != "" && row.GatewayModelKey != "" && strings.Contains(row.ModelKey, "/") {
			continue
		}
		var match *domain.Model
		for j := range remote {
			r := &remote[j]
			if r.ModelKey == row.ModelKey || r.GatewayModelKey == row.ModelKey || strings.HasSuffix(r.ModelKey, "/"+row.ModelKey) {
				match = r
				break
			}
		}
		if match == nil {
			continue
		}
		if match.ModelKey != row.ModelKey {
			if existing, gerr := s.Store.GetModelByKey(ctx, match.ModelKey); gerr == nil && existing != nil && existing.ID != row.ID {
				// Merge the legacy row into the synced one so agent references
				// follow the full gateway model key.
				if err := s.Store.MergeModel(ctx, row.ID, existing.ID); err != nil && firstErr == nil {
					firstErr = err
				}
				continue
			}
		}
		if err := s.Store.UpdateModelRouting(ctx, row.ID, match.ModelKey, match.Provider, match.GatewayModelKey); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func defaultMaxOutput(v int) int {
	if v <= 0 {
		return 4096
	}
	return v
}

func ptrTime(t time.Time) *time.Time { return &t }
