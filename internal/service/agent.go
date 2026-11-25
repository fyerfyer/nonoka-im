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
		uid := extractUserIDFromContext(ctx)
		visible := make([]AgentView, 0, len(rows))
		for i := range rows {
			owner := rows[i].OwnerUserID == uid
			visible = append(visible, agentView(&rows[i], owner, owner))
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
		row, published, manager, owner, err := s.agents.Detail(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		view := agentView(row, manager, owner)
		if published != nil {
			p := versionView(published)
			view.Published = &p
			view.DraftChanged = row.DraftRevision > published.SourceRevision
		} else {
			view.DraftChanged = true
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
		writeAgentJSON(w, http.StatusCreated, agentView(row, true, true))
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
		writeAgentJSON(w, http.StatusOK, agentView(row, true, true))
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
		version, err := s.agents.Publish(ctx, extractUserIDFromContext(ctx), id, input.Revision, input.Note)
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
		if err := s.agents.Share(ctx, extractUserIDFromContext(ctx), id, input.Kind, input.Target, input.Role, input.Enabled); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"kind": input.Kind, "target": input.Target, "role": input.Role, "enabled": input.Enabled})
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

func (s *AgentHTTP) Versions(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		rows, err := s.agents.Versions(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		out := make([]AgentVersionSummaryView, 0, len(rows))
		for i := range rows {
			out = append(out, versionSummaryView(&rows[i]))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"versions": out})
		return nil
	})
}

func (s *AgentHTTP) Version(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		version, err := parsePositiveInt64(c.Vars().Get("version"))
		if err != nil {
			return invalidAgentInput("invalid version")
		}
		row, err := s.agents.Version(ctx, extractUserIDFromContext(ctx), id, version)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, versionView(row))
		return nil
	})
}

func (s *AgentHTTP) Restore(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		row, err := s.agents.Restore(ctx, extractUserIDFromContext(ctx), id, input.Version, input.Revision)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, agentView(row, true, true))
		return nil
	})
}

func (s *AgentHTTP) Test(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		agent, cfg, err := s.agents.TestSnapshot(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		run, err := s.runs.StartTest(ctx, extractUserIDFromContext(ctx), agent, cfg, input.Message)
		if err != nil {
			return err
		}
		if s.quota != nil {
			_ = s.quota.RecordTestRun(ctx, extractUserIDFromContext(ctx), agent.ID)
		}
		writeAgentJSON(w, http.StatusAccepted, runView(run))
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
