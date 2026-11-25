package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"nonoka-im/internal/httpapi"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func (s *AgentHTTP) Runs(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		limit := httpapi.Limit(r, 50, 200)
		offset, err := httpapi.Offset(r.URL.Query().Get("cursor"))
		if err != nil {
			return invalidAgentInput("invalid cursor")
		}
		runs, total, err := s.runs.List(ctx, extractUserIDFromContext(ctx), id, limit, offset)
		if err != nil {
			return err
		}
		hasMore := int64(offset+len(runs)) < total
		writeAgentJSON(w, http.StatusOK, httpapi.NewPage(runViews(runs), offset, limit, hasMore))
		return nil
	})
}

func (s *AgentHTTP) RunDetail(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid run id")
		}
		run, steps, tools, citations, err := s.runs.Get(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		view := runView(run)
		view.Steps = runStepViews(steps)
		view.ToolInvocations = toolInvocationViews(tools)
		view.Citations = citationViews(citations)
		writeAgentJSON(w, http.StatusOK, view)
		return nil
	})
}

// RunByReply resolves an agent answer message to its run and citations for the
// chat source chips. It returns 404 when the message has no agent run.
func (s *AgentHTTP) RunByReply(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		msgID, err := strconv.ParseInt(c.Vars().Get("msgId"), 10, 64)
		if err != nil || msgID <= 0 {
			return invalidAgentInput("invalid message id")
		}
		run, citations, err := s.runs.GetByReply(ctx, extractUserIDFromContext(ctx), msgID)
		if err != nil {
			return err
		}
		view := runView(run)
		view.Citations = citationViews(citations)
		writeAgentJSON(w, http.StatusOK, view)
		return nil
	})
}

func (s *AgentHTTP) RunEvents(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid run id")
		}
		afterSeq, _ := strconv.ParseUint(r.URL.Query().Get("afterSeq"), 10, 64)
		rows, err := s.runs.Events(ctx, extractUserIDFromContext(ctx), id, afterSeq)
		if err != nil {
			return err
		}
		next := afterSeq
		for _, row := range rows {
			if row.Seq > next {
				next = row.Seq
			}
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"events": rows, "nextSeq": next})
		return nil
	})
}

func (s *AgentHTTP) RunAction(c khttp.Context, action string) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid run id")
		}
		acknowledge := strings.EqualFold(r.URL.Query().Get("acknowledgeSideEffect"), "true")
		if err := s.runs.Act(ctx, extractUserIDFromContext(ctx), id, action, acknowledge); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"action": action})
		return nil
	})
}
