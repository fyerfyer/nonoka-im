package admin

import (
	"context"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/realtime"
)

// RunService owns run history, live event replay and operator actions.
type RunService struct {
	Store      Store
	Authz      Authorizer
	EventStore realtime.RunEventStore
	Publisher  realtime.RunEventSink
}

func NewRunService(store Store, az Authorizer, events realtime.RunEventStore, publisher realtime.RunEventSink) *RunService {
	return &RunService{Store: store, Authz: az, EventStore: events, Publisher: publisher}
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

func (s *RunService) Get(ctx context.Context, userID int64, runID uint64) (*domain.AgentRun, []domain.AgentRunStep, []domain.ToolInvocation, []domain.AgentRunCitation, error) {
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return nil, nil, nil, nil, ErrNotFound
	}
	if userID != run.ActorUserID && s.Authz.CanManageAgent(ctx, userID, run.AgentID) != nil {
		return nil, nil, nil, nil, ErrForbidden
	}
	steps, err := s.Store.ListRunSteps(ctx, runID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	tools, err := s.Store.ListToolInvocations(ctx, runID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	citations, err := s.Store.ListRunCitations(ctx, runID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return run, steps, tools, citations, nil
}

// GetByReply resolves the agent answer message to its run and sources.
func (s *RunService) GetByReply(ctx context.Context, userID, msgID int64) (*domain.AgentRun, []domain.AgentRunCitation, error) {
	run, err := s.Store.GetRunByReplyMsgID(ctx, msgID)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	if userID != run.ActorUserID && s.Authz.CanManageAgent(ctx, userID, run.AgentID) != nil {
		return nil, nil, ErrForbidden
	}
	citations, err := s.Store.ListRunCitations(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}
	return run, citations, nil
}

// Events returns the bounded replay buffer for a run. Completed runs still
// rely on the persisted final IM message; the buffer is a reconnect aid.
func (s *RunService) Events(ctx context.Context, userID int64, runID, afterSeq uint64) ([]realtime.RunEvent, error) {
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return nil, ErrNotFound
	}
	if userID != run.ActorUserID && s.Authz.CanManageAgent(ctx, userID, run.AgentID) != nil {
		return nil, ErrForbidden
	}
	if s.EventStore == nil {
		return []realtime.RunEvent{}, nil
	}
	return s.EventStore.Since(ctx, runID, afterSeq)
}

func (s *RunService) Act(ctx context.Context, userID int64, runID uint64, action string, acknowledgeSideEffect bool) error {
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return ErrNotFound
	}
	if userID != run.ActorUserID && s.Authz.CanManageAgent(ctx, userID, run.AgentID) != nil {
		return ErrForbidden
	}
	if err := s.Store.SetRunAction(ctx, runID, action, acknowledgeSideEffect); err != nil {
		return err
	}
	if action == "cancel" && s.Publisher != nil {
		_ = s.Publisher.Publish(context.WithoutCancel(ctx), realtime.RunEvent{
			RunID: run.ID, Type: realtime.RunCancelled, Topic: run.Topic, AgentID: run.AgentID,
			ActorUserID: run.ActorUserID, Status: domain.RunCancelled, ErrorCode: "cancelled",
			TimestampMs: time.Now().UnixMilli(),
		}, []int64{run.ActorUserID})
	}
	return nil
}
