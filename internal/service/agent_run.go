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
		run, err := s.runs.Get(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, runView(run))
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
