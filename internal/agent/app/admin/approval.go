package admin

import (
	"context"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/audit"
)

// ApprovalService owns the approval inbox and one-shot decisions.
type ApprovalService struct {
	Store Store
	Authz Authorizer
	Audit *audit.Service
}

func NewApprovalService(store Store, az Authorizer, auditor *audit.Service) *ApprovalService {
	return &ApprovalService{Store: store, Authz: az, Audit: auditor}
}

func (s *ApprovalService) List(ctx context.Context, userID int64, status string, limit, offset int) ([]domain.AgentApproval, int64, error) {
	switch status {
	case "", domain.ApprovalPending, domain.ApprovalApproved, domain.ApprovalRejected, domain.ApprovalExpired, domain.ApprovalConsumed:
	default:
		return nil, 0, ErrInvalidInput
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.Store.ListApprovals(ctx, userID, status, limit, offset)
}

func (s *ApprovalService) Get(ctx context.Context, userID int64, approvalID uint64) (*domain.AgentApproval, error) {
	approval, err := s.Store.GetApproval(ctx, approvalID)
	if err != nil {
		return nil, ErrNotFound
	}
	run, err := s.Store.GetRun(ctx, approval.RunID)
	if err != nil {
		return nil, ErrNotFound
	}
	if userID != approval.ApproverID && s.Authz.CanManageAgent(ctx, userID, run.AgentID) != nil {
		return nil, ErrForbidden
	}
	return approval, nil
}

func (s *ApprovalService) Decide(ctx context.Context, userID int64, approvalID uint64, action string) error {
	var decision string
	switch action {
	case "approve":
		decision = domain.ApprovalApproved
	case "reject":
		decision = domain.ApprovalRejected
	default:
		return ErrInvalidInput
	}
	approval, err := s.Store.GetApproval(ctx, approvalID)
	if err != nil {
		return ErrNotFound
	}
	if approval.Status != domain.ApprovalPending {
		return domain.ErrApprovalGone
	}
	run, err := s.Store.GetRun(ctx, approval.RunID)
	if err != nil {
		return ErrNotFound
	}
	if run.Status != domain.RunWaitingApproval {
		return domain.ErrRunState
	}
	if userID != approval.ApproverID || s.Authz.CanUseAgent(ctx, userID, run.AgentID) != nil {
		return ErrForbidden
	}
	if err := s.Store.DecideAndRequeue(ctx, approvalID, decision, userID); err != nil {
		return err
	}
	return recordAudit(ctx, s.Audit, userID, "approval."+action, "approval", uintKey(approvalID), approvalAuditFields(*approval), map[string]any{
		"status": decision, "decidedBy": userID,
	})
}
