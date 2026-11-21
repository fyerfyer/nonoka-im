package admin

import (
	"context"

	"nonoka-im/internal/agent/domain"
)

// RunService owns run history and operator actions.
type RunService struct {
	Store Store
	Authz Authorizer
}

func NewRunService(store Store, az Authorizer) *RunService {
	return &RunService{Store: store, Authz: az}
}

func (s *RunService) List(ctx context.Context, userID, agentID int64, limit, offset int) ([]domain.AgentRun, int64, error) {
	if s.Authz.CanManageAgent(ctx, userID, agentID) != nil {
		return nil, 0, ErrForbidden
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.Store.ListRunsPage(ctx, agentID, limit, offset)
}

func (s *RunService) Get(ctx context.Context, userID int64, runID uint64) (*domain.AgentRun, error) {
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return nil, ErrNotFound
	}
	if userID != run.ActorUserID && s.Authz.CanManageAgent(ctx, userID, run.AgentID) != nil {
		return nil, ErrForbidden
	}
	return run, nil
}

func (s *RunService) Act(ctx context.Context, userID int64, runID uint64, action string, acknowledgeSideEffect bool) error {
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return ErrNotFound
	}
	if userID != run.ActorUserID && s.Authz.CanManageAgent(ctx, userID, run.AgentID) != nil {
		return ErrForbidden
	}
	return s.Store.SetRunAction(ctx, runID, action, acknowledgeSideEffect)
}
