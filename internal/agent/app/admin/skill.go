package admin

import (
	"context"
	"fmt"
	"strings"

	"nonoka-im/internal/agent/domain"
)

// SkillInput is the transport-neutral skill publication request.
type SkillInput struct {
	Name    string
	Version int64
	Content string
}

// SkillService owns the skill registry.
type SkillService struct{ Platform PlatformStore }

func NewSkillService(platform PlatformStore) *SkillService { return &SkillService{Platform: platform} }

func (s *SkillService) Publish(ctx context.Context, userID int64, input SkillInput) (*domain.Skill, error) {
	if !IsAdmin(userID) {
		return nil, ErrAdminRequired
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 || strings.ContainsAny(input.Name, " \t\n/") {
		return nil, fmt.Errorf("%w: invalid skill name", ErrInvalidInput)
	}
	if input.Version <= 0 {
		return nil, fmt.Errorf("%w: invalid skill version", ErrInvalidInput)
	}
	if input.Content == "" || len(input.Content) > 200000 || !strings.Contains(input.Content, "name:") {
		return nil, fmt.Errorf("%w: skill content must be a SKILL.md document", ErrInvalidInput)
	}
	return s.Platform.PublishSkill(ctx, input.Name, input.Version, input.Content)
}

func (s *SkillService) List(ctx context.Context, userID int64) ([]domain.Skill, error) {
	if userID <= 0 {
		return nil, ErrForbidden
	}
	return s.Platform.ListSkills(ctx, true)
}
