package service

import (
	"context"
	"net/http"
	"strconv"

	"nonoka-im/internal/httpapi"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// ListApprovals returns the caller's approvals (default: all statuses).
func (s *AgentHTTP) ListApprovals(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		status := r.URL.Query().Get("status")
		limit := httpapi.Limit(r, 50, 200)
		offset, err := httpapi.Offset(r.URL.Query().Get("cursor"))
		if err != nil {
			return invalidAgentInput("invalid cursor")
		}
		rows, total, err := s.approvals.List(ctx, extractUserIDFromContext(ctx), status, limit, offset)
		if err != nil {
			return err
		}
		hasMore := int64(offset+len(rows)) < total
		writeAgentJSON(w, http.StatusOK, httpapi.NewPage(approvalViews(rows), offset, limit, hasMore))
		return nil
	})
}

func (s *AgentHTTP) ApprovalDetail(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid approval id")
		}
		approval, err := s.approvals.Get(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, approvalView(approval))
		return nil
	})
}

func (s *AgentHTTP) ApprovalAction(c khttp.Context, action string) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid approval id")
		}
		if err := s.approvals.Decide(ctx, extractUserIDFromContext(ctx), id, action); err != nil {
			return err
		}
		status := "approved"
		if action == "reject" {
			status = "rejected"
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": status})
		return nil
	})
}
