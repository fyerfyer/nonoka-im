package service

import (
	"context"
	"net/http"

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
