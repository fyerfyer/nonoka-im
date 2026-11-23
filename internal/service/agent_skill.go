package service

import (
	"context"
	"net/http"
	"time"

	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/domain"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type SkillView struct {
	ID            uint64             `json:"id"`
	OwnerUserID   int64              `json:"ownerUserId"`
	Slug          string             `json:"slug"`
	DisplayName   string             `json:"displayName"`
	Description   string             `json:"description"`
	Visibility    string             `json:"visibility"`
	ReviewStatus  string             `json:"reviewStatus"`
	LatestVersion int64              `json:"latestVersion"`
	Versions      []SkillVersionView `json:"versions,omitempty"`
	CreatedAt     time.Time          `json:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt"`
}

type SkillVersionView struct {
	ID           uint64     `json:"id"`
	SkillID      uint64     `json:"skillId"`
	Version      int64      `json:"version"`
	Checksum     string     `json:"checksum"`
	Status       string     `json:"status"`
	ReviewStatus string     `json:"reviewStatus"`
	ReviewReason string     `json:"reviewReason,omitempty"`
	CreatedBy    int64      `json:"createdBy"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	Content      string     `json:"content,omitempty"`
	ReviewAt     *time.Time `json:"reviewAt,omitempty"`
}

type skillRequest struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	Content     string `json:"content"`
}

type skillVersionRequest struct {
	Content string `json:"content"`
}

type skillReviewRequest struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

type skillMetaRequest struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
}

func (s *AgentHTTP) ListSkills(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		rows, err := s.skills.List(ctx, extractUserIDFromContext(ctx))
		if err != nil {
			return err
		}
		out := make([]SkillView, 0, len(rows))
		for i := range rows {
			out = append(out, skillView(&rows[i], nil))
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"skills": out})
		return nil
	})
}

func (s *AgentHTTP) GetSkill(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid skill id")
		if err != nil {
			return err
		}
		skill, versions, err := s.skills.Get(ctx, extractUserIDFromContext(ctx), id)
		if err != nil {
			return err
		}
		views := make([]SkillVersionView, 0, len(versions))
		for i := range versions {
			views = append(views, skillVersionView(&versions[i], s.skills.CanManage(ctx, extractUserIDFromContext(ctx), skill)))
		}
		view := skillView(skill, views)
		writeAgentJSON(w, http.StatusOK, view)
		return nil
	})
}

func (s *AgentHTTP) CreateSkill(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		var input skillRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		skill, version, err := s.skills.Create(ctx, extractUserIDFromContext(ctx), admin.SkillInput{
			Slug: input.Slug, DisplayName: input.DisplayName, Description: input.Description,
			Visibility: input.Visibility, Content: input.Content,
		})
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, skillView(skill, []SkillVersionView{skillVersionView(version, true)}))
		return nil
	})
}

func (s *AgentHTTP) AddSkillVersion(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid skill id")
		if err != nil {
			return err
		}
		var input skillVersionRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		version, err := s.skills.AddVersion(ctx, extractUserIDFromContext(ctx), id, input.Content)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, skillVersionView(version, true))
		return nil
	})
}

func (s *AgentHTTP) UpdateSkill(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid skill id")
		if err != nil {
			return err
		}
		var input skillMetaRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		skill, err := s.skills.UpdateMeta(ctx, extractUserIDFromContext(ctx), id, input.DisplayName, input.Description, input.Visibility)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, skillView(skill, nil))
		return nil
	})
}

func (s *AgentHTTP) DeleteSkill(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid skill id")
		if err != nil {
			return err
		}
		if err := s.skills.Delete(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return nil
	})
}

func (s *AgentHTTP) ReviewSkill(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid skill id")
		if err != nil {
			return err
		}
		var input skillReviewRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		if err := s.skills.ReviewSkill(ctx, extractUserIDFromContext(ctx), id, input.Approved); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"id": id, "approved": input.Approved})
		return nil
	})
}

func (s *AgentHTTP) ReviewSkillVersion(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseUintVar(c, "id", "invalid skill id")
		if err != nil {
			return err
		}
		version, err := parseUintVar(c, "version", "invalid skill version")
		if err != nil {
			return err
		}
		var input skillReviewRequest
		if err := decodeJSONBody(r, &input); err != nil {
			return err
		}
		if err := s.skills.ReviewVersion(ctx, extractUserIDFromContext(ctx), id, int64(version), input.Approved, input.Reason); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"id": id, "version": version, "approved": input.Approved})
		return nil
	})
}

func skillView(skill *domain.Skill, versions []SkillVersionView) SkillView {
	return SkillView{
		ID: skill.ID, OwnerUserID: skill.OwnerUserID, Slug: skill.Slug, DisplayName: skill.DisplayName,
		Description: skill.Description, Visibility: skill.Visibility, ReviewStatus: skill.ReviewStatus,
		LatestVersion: skill.LatestVersion, Versions: versions, CreatedAt: skill.CreatedAt, UpdatedAt: skill.UpdatedAt,
	}
}

func skillVersionView(version *domain.SkillVersion, includeContent bool) SkillVersionView {
	view := SkillVersionView{
		ID: version.ID, SkillID: version.SkillID, Version: version.Version, Checksum: version.Checksum,
		Status: version.Status, ReviewStatus: version.ReviewStatus, ReviewReason: version.ReviewReason,
		CreatedBy: version.CreatedBy, CreatedAt: version.CreatedAt, UpdatedAt: version.UpdatedAt,
		ReviewAt: version.ReviewAt,
	}
	if includeContent {
		view.Content = version.Content
	}
	return view
}
