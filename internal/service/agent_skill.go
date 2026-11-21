package service

import (
	"context"
	"net/http"

	"nonoka-im/internal/agent/app/admin"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type skillRequest struct {
	Name    string `json:"name"`
	Version int64  `json:"version"`
	Content string `json:"content"`
}

func (s *AgentHTTP) PublishSkill(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		var input skillRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		row, err := s.skills.Publish(ctx, extractUserIDFromContext(ctx), admin.SkillInput{
			Name: input.Name, Version: input.Version, Content: input.Content,
		})
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, map[string]any{"id": row.ID, "name": row.Name, "version": row.Version})
		return nil
	})
}

func (s *AgentHTTP) ListSkills(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		rows, err := s.skills.List(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		skills := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			skills = append(skills, map[string]any{
				"name": row.Name, "version": row.Version, "published": row.Published, "createdAt": row.CreatedAt,
			})
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"skills": skills})
		return nil
	})
}
