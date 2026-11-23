package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nonoka-im/internal/agent/domain"
)

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
			return fmt.Errorf("grant platform admin %d: %w", id, err)
		}
	}
	return nil
}

// DefaultSpec describes the optional demo/bootstrap service created from
// environment settings when no service exists.
type DefaultSpec struct {
	Name      string
	Display   string
	AllowAll  bool
	Preferred string // preferred model key; empty picks the first gateway model
}

// EnsureDefaultService creates the bootstrap Bifrost service once and returns
// its service/model IDs. Runtime never falls back to environment model
// settings; this function only seeds the database.
func (s *Service) EnsureDefaultService(ctx context.Context, spec DefaultSpec) (int64, int64, string, error) {
	if spec.Name == "" {
		spec.Name = "bifrost-default"
	}
	service, err := s.Store.GetModelServiceByName(ctx, spec.Name)
	if err == nil {
		models, lerr := s.Store.ListModels(ctx, service.ID, false)
		if lerr != nil {
			return 0, 0, "", lerr
		}
		if len(models) == 0 {
			if serr := s.sync(ctx, service.ID); serr == nil {
				models, lerr = s.Store.ListModels(ctx, service.ID, false)
				if lerr != nil {
					return 0, 0, "", lerr
				}
			}
		}
		if len(models) == 0 {
			return 0, 0, "", fmt.Errorf("default model service %q has no enabled models", spec.Name)
		}
		picked := pickModel(models, spec.Preferred)
		return int64(service.ID), int64(picked.ID), picked.ModelKey, nil
	}
	if !errors.Is(err, domain.ErrModelServiceNotFound) {
		return 0, 0, "", err
	}
	if s.Gateway == nil || s.GatewayURL == "" {
		return 0, 0, "", fmt.Errorf("Bifrost is not configured")
	}
	enabled := true
	created, err := s.createBifrost(ctx, 0, domain.ModelServiceSpec{
		Name: spec.Name, DisplayName: spec.Display, GatewayType: "bifrost",
		AllowAll: spec.AllowAll, Enabled: &enabled,
	})
	if err != nil {
		return 0, 0, "", err
	}
	models, err := s.Store.ListModels(ctx, created.ID, false)
	if err != nil {
		return 0, 0, "", err
	}
	if len(models) == 0 {
		return 0, 0, "", fmt.Errorf("default model service %q has no enabled models", spec.Name)
	}
	picked := pickModel(models, spec.Preferred)
	return int64(created.ID), int64(picked.ID), picked.ModelKey, nil
}

func pickModel(models []domain.Model, preferred string) domain.Model {
	if preferred != "" {
		for _, m := range models {
			if m.ModelKey == preferred || strings.HasSuffix(m.ModelKey, "/"+preferred) {
				return m
			}
		}
	}
	return models[0]
}
