package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type RunsResource struct {
	http   *apiTransport
	client *Client
}

func (r *RunsResource) Get(ctx context.Context, runID uint64) (*AgentRun, error) {
	var out AgentRun
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/agent-runs/%d", runID), nil, nil, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

// ByReply resolves the agent answer message to its run and citations.
func (r *RunsResource) ByReply(ctx context.Context, msgID int64) (*AgentRun, error) {
	var out AgentRun
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/agent-runs/by-reply/%d", msgID), nil, nil, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

// ByClient resolves the stable reply client_msg_id to its run and citations.
func (r *RunsResource) ByClient(ctx context.Context, clientMsgID string) (*AgentRun, error) {
	var out AgentRun
	path := "/v1/agent-runs/by-client/" + url.PathEscape(clientMsgID)
	if err := r.http.doJSON(ctx, http.MethodGet, path, nil, nil, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

// Events returns replayable run events after a sequence number. The final
// answer still comes from the IM message referenced by completed.replyMsgId.
func (r *RunsResource) Events(ctx context.Context, runID, afterSeq uint64) ([]RunEvent, uint64, error) {
	query := url.Values{}
	query.Set("afterSeq", strconv.FormatUint(afterSeq, 10))
	var reply struct {
		Events  []RunEvent `json:"events"`
		NextSeq uint64     `json:"nextSeq"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/agent-runs/%d/events", runID), query, nil, &reply, true); err != nil {
		return nil, 0, err
	}
	return reply.Events, reply.NextSeq, nil
}

func (r *RunsResource) Cancel(ctx context.Context, runID uint64) error {
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/agent-runs/%d/cancel", runID), nil, nil, nil, false)
}

func (r *RunsResource) Retry(ctx context.Context, runID uint64) error {
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/agent-runs/%d/retry", runID), nil, nil, nil, false)
}

func (r *RunsResource) Skip(ctx context.Context, runID uint64) error {
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/agent-runs/%d/skip", runID), nil, nil, nil, false)
}

func (r *RunsResource) Approve(ctx context.Context, approvalID uint64) error {
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/agent-approvals/%d/approve", approvalID), nil, nil, nil, false)
}

func (r *RunsResource) Reject(ctx context.Context, approvalID uint64) error {
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/agent-approvals/%d/reject", approvalID), nil, nil, nil, false)
}

func (r *RunsResource) Approvals(ctx context.Context, status string, opts ListOptions) (*Page[AgentApproval], error) {
	query := url.Values{}
	if status != "" {
		query.Set("status", status)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	query.Set("limit", strconv.Itoa(limit))
	if opts.Cursor != "" {
		query.Set("cursor", opts.Cursor)
	}
	var page Page[AgentApproval]
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/agent-approvals", query, nil, &page, true); err != nil {
		return nil, err
	}
	return &page, nil
}

func (r *RunsResource) Approval(ctx context.Context, approvalID uint64) (*AgentApproval, error) {
	var out AgentApproval
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/agent-approvals/%d", approvalID), nil, nil, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}
