package sdk

import (
	"context"
	"fmt"
	"net/http"
)

type SkillInput struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	Content     string `json:"content"`
}

type SkillsResource struct {
	http *apiTransport
}

func (r *SkillsResource) List(ctx context.Context) ([]Skill, error) {
	var reply struct {
		Skills []Skill `json:"skills"`
	}
	if err := r.http.doJSON(ctx, http.MethodGet, "/v1/skills", nil, nil, &reply, true); err != nil {
		return nil, err
	}
	return reply.Skills, nil
}

func (r *SkillsResource) Get(ctx context.Context, id uint64) (*Skill, error) {
	var out Skill
	if err := r.http.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/skills/%d", id), nil, nil, &out, true); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *SkillsResource) Create(ctx context.Context, input SkillInput) (*Skill, error) {
	var out Skill
	if err := r.http.doJSON(ctx, http.MethodPost, "/v1/skills", nil, input, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *SkillsResource) AddVersion(ctx context.Context, id uint64, content string) (*SkillVersion, error) {
	var out SkillVersion
	body := map[string]any{"content": content}
	if err := r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/skills/%d/versions", id), nil, body, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *SkillsResource) Update(ctx context.Context, id uint64, displayName, description, visibility string) (*Skill, error) {
	var out Skill
	body := map[string]any{"displayName": displayName, "description": description, "visibility": visibility}
	if err := r.http.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/v1/skills/%d", id), nil, body, &out, false); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *SkillsResource) Delete(ctx context.Context, id uint64) error {
	return r.http.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/v1/skills/%d", id), nil, nil, nil, false)
}

func (r *SkillsResource) Review(ctx context.Context, id uint64, approved bool, reason string) error {
	body := map[string]any{"approved": approved, "reason": reason}
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/skills/%d/review", id), nil, body, nil, false)
}

func (r *SkillsResource) ReviewVersion(ctx context.Context, id uint64, version int64, approved bool, reason string) error {
	body := map[string]any{"approved": approved, "reason": reason}
	return r.http.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/skills/%d/versions/%d/review", id, version), nil, body, nil, false)
}
