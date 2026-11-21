package service

import (
	"context"
	"net/http"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/httpapi"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func (s *AgentHTTP) List(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		limit := httpapi.Limit(r, 50, 200)
		offset, err := httpapi.Offset(r.URL.Query().Get("cursor"))
		if err != nil {
			return invalidAgentInput("invalid cursor")
		}
		rows, total, scanned, err := s.agents.List(ctx, extractUserIDFromContext(ctx), limit, offset)
		if err != nil {
			return err
		}
		visible := make([]AgentView, 0, len(rows))
		for i := range rows {
			visible = append(visible, agentView(&rows[i], rows[i].OwnerUserID == extractUserIDFromContext(ctx)))
		}
		page := httpapi.Page[AgentView]{Items: visible, HasMore: int64(scanned) < total}
		if page.HasMore {
			page.NextCursor = httpapi.EncodeCursor(scanned)
		}
		writeAgentJSON(w, http.StatusOK, page)
		return nil
	})
}

func (s *AgentHTTP) Detail(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		row, published, manager, err := s.agents.Detail(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		view := agentView(row, manager)
		if published != nil {
			p := versionView(published)
			view.Published = &p
		}
		writeAgentJSON(w, http.StatusOK, view)
		return nil
	})
}

func (s *AgentHTTP) Create(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		row, err := s.agents.Create(ctx, extractUserIDFromContext(ctx), input.input())
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, agentView(row, true))
		return nil
	})
}

func (s *AgentHTTP) Draft(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		row, err := s.agents.UpdateDraft(ctx, extractUserIDFromContext(ctx), id, input.input())
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, agentView(row, true))
		return nil
	})
}

func (s *AgentHTTP) Publish(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		version, err := s.agents.Publish(ctx, extractUserIDFromContext(ctx), id, input.Revision)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, versionView(version))
		return nil
	})
}

func (s *AgentHTTP) Status(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		if err := s.agents.SetStatus(ctx, extractUserIDFromContext(ctx), id, input.Status); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": input.Status})
		return nil
	})
}

func (s *AgentHTTP) Share(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		if err := s.agents.Share(ctx, extractUserIDFromContext(ctx), id, input.Kind, input.Target, input.Enabled); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"kind": input.Kind, "target": input.Target, "enabled": input.Enabled})
		return nil
	})
}

func (s *AgentHTTP) Shares(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		shares, err := s.agents.Shares(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"shares": shares})
		return nil
	})
}

func (s *AgentHTTP) Delete(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		if err := s.agents.Delete(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return nil
	})
}

// VerifyBotSend is called by the HTTP auth middleware for bot credentials.
func (s *AgentHTTP) VerifyBotSend(ctx context.Context, claims map[string]any) error {
	agentID, ok1 := claims["agent_id"].(float64)
	botID, ok2 := claims["user_id"].(float64)
	topic, ok3 := claims["topic"].(string)
	if !ok1 || !ok2 || !ok3 || claims["scope"] != "agent:send" {
		return domain.ErrAgentNotFound
	}
	return s.agents.VerifyBotSend(ctx, int64(agentID), int64(botID), topic)
}
