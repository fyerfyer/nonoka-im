package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/skillspec"
)

var skillSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// SkillInput is the transport-neutral skill publication request.
type SkillInput struct {
	Slug        string
	DisplayName string
	Description string
	Visibility  string
	Content     string
}

// SkillService owns versioned skill upload, validation and review.
type SkillService struct {
	Store PlatformStore
	Authz Authorizer
}

func NewSkillService(store PlatformStore, az Authorizer) *SkillService {
	return &SkillService{Store: store, Authz: az}
}

func (s *SkillService) isAdmin(ctx context.Context, userID int64) bool {
	return s.Authz != nil && s.Authz.CanManagePlatform(ctx, userID) == nil
}

func (s *SkillService) List(ctx context.Context, userID int64) ([]domain.Skill, error) {
	if userID <= 0 {
		return nil, ErrForbidden
	}
	return s.Store.ListSkillsForUser(ctx, userID, s.isAdmin(ctx, userID))
}

func (s *SkillService) Get(ctx context.Context, userID int64, id uint64) (*domain.Skill, []domain.SkillVersion, error) {
	skill, err := s.Store.GetSkillByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if !s.canView(ctx, userID, skill) {
		return nil, nil, ErrForbidden
	}
	versions, err := s.Store.ListSkillVersions(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return skill, versions, nil
}

func (s *SkillService) Create(ctx context.Context, userID int64, input SkillInput) (*domain.Skill, *domain.SkillVersion, error) {
	if userID <= 0 {
		return nil, nil, ErrForbidden
	}
	if err := normalizeSkillInput(&input); err != nil {
		return nil, nil, err
	}
	fm, _, checksum, err := skillspec.Validate(input.Content, input.Slug)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	review := domain.ReviewApproved
	if input.Visibility != domain.VisibilityPrivate {
		review = domain.ReviewPending
	}
	skill, err := s.Store.CreateSkill(ctx, userID, input.Slug, input.DisplayName, input.Description, input.Visibility, review)
	if err != nil {
		return nil, nil, err
	}
	version, err := s.Store.CreateSkillVersion(ctx, domain.SkillVersion{
		SkillID: skill.ID, Version: 1, Content: input.Content,
		FrontMatter: marshalFrontMatter(fm), Checksum: checksum,
		Status: domain.SkillVersionPublished, ReviewStatus: review, CreatedBy: userID,
	})
	if err != nil {
		_ = s.Store.DeleteSkill(ctx, skill.ID)
		return nil, nil, err
	}
	skill, err = s.Store.GetSkillByID(ctx, skill.ID)
	return skill, version, err
}

func (s *SkillService) AddVersion(ctx context.Context, userID int64, skillID uint64, content string) (*domain.SkillVersion, error) {
	skill, err := s.Store.GetSkillByID(ctx, skillID)
	if err != nil {
		return nil, err
	}
	if !s.canManage(ctx, userID, skill) {
		return nil, ErrForbidden
	}
	fm, _, checksum, err := skillspec.Validate(content, skill.Slug)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	review := domain.ReviewApproved
	if skill.Visibility != domain.VisibilityPrivate {
		review = domain.ReviewPending
	}
	return s.Store.CreateSkillVersion(ctx, domain.SkillVersion{
		SkillID: skill.ID, Version: skill.LatestVersion + 1, Content: content,
		FrontMatter: marshalFrontMatter(fm), Checksum: checksum,
		Status: domain.SkillVersionPublished, ReviewStatus: review, CreatedBy: userID,
	})
}

func (s *SkillService) UpdateMeta(ctx context.Context, userID int64, id uint64, displayName, description, visibility string) (*domain.Skill, error) {
	skill, err := s.Store.GetSkillByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.canManage(ctx, userID, skill) {
		return nil, ErrForbidden
	}
	visibility = strings.TrimSpace(visibility)
	if visibility == "" {
		visibility = skill.Visibility
	}
	if visibility != domain.VisibilityPrivate && visibility != domain.VisibilityShared && visibility != domain.VisibilityPublic {
		return nil, fmt.Errorf("%w: invalid visibility", ErrInvalidInput)
	}
	review := skill.ReviewStatus
	if visibility == domain.VisibilityPrivate {
		review = domain.ReviewApproved
	} else if visibility != skill.Visibility {
		review = domain.ReviewPending
	}
	if err := s.Store.UpdateSkillMeta(ctx, id, strings.TrimSpace(displayName), strings.TrimSpace(description), visibility, review); err != nil {
		return nil, err
	}
	return s.Store.GetSkillByID(ctx, id)
}

func (s *SkillService) ReviewSkill(ctx context.Context, userID int64, id uint64, approved bool) error {
	if !s.isAdmin(ctx, userID) {
		return ErrAdminRequired
	}
	status := domain.ReviewRejected
	if approved {
		status = domain.ReviewApproved
	}
	return s.Store.SetSkillReview(ctx, id, status)
}

func (s *SkillService) ReviewVersion(ctx context.Context, userID int64, skillID uint64, version int64, approved bool, reason string) error {
	if !s.isAdmin(ctx, userID) {
		return ErrAdminRequired
	}
	status := domain.ReviewRejected
	if approved {
		status = domain.ReviewApproved
	}
	return s.Store.SetSkillVersionReview(ctx, skillID, version, status, reason, userID)
}

func (s *SkillService) Delete(ctx context.Context, userID int64, id uint64) error {
	skill, err := s.Store.GetSkillByID(ctx, id)
	if err != nil {
		return err
	}
	if !s.canManage(ctx, userID, skill) {
		return ErrForbidden
	}
	return s.Store.DeleteSkill(ctx, id)
}

// CanManage reports whether the caller may edit/review this skill (owner or
// platform admin). Transport uses it to decide whether to include full content.
func (s *SkillService) CanManage(ctx context.Context, userID int64, skill *domain.Skill) bool {
	return s.canManage(ctx, userID, skill)
}

func (s *SkillService) canView(ctx context.Context, userID int64, skill *domain.Skill) bool {
	if s.isAdmin(ctx, userID) || skill.OwnerUserID == userID {
		return true
	}
	return skill.Visibility != domain.VisibilityPrivate && skill.ReviewStatus == domain.ReviewApproved
}

func (s *SkillService) canManage(ctx context.Context, userID int64, skill *domain.Skill) bool {
	return s.isAdmin(ctx, userID) || skill.OwnerUserID == userID
}

func normalizeSkillInput(input *SkillInput) error {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Description = strings.TrimSpace(input.Description)
	input.Visibility = strings.TrimSpace(input.Visibility)
	if input.Slug == "" || !skillSlugPattern.MatchString(input.Slug) {
		return fmt.Errorf("%w: slug must match %s", ErrInvalidInput, skillSlugPattern.String())
	}
	if input.DisplayName == "" {
		input.DisplayName = input.Slug
	}
	if len(input.DisplayName) > 200 || len(input.Description) > 4000 {
		return fmt.Errorf("%w: display name or description is too long", ErrInvalidInput)
	}
	if input.Visibility == "" {
		input.Visibility = domain.VisibilityPrivate
	}
	if input.Visibility != domain.VisibilityPrivate && input.Visibility != domain.VisibilityShared && input.Visibility != domain.VisibilityPublic {
		return fmt.Errorf("%w: invalid visibility", ErrInvalidInput)
	}
	return nil
}

func marshalFrontMatter(fm skillspec.FrontMatter) string {
	raw, _ := json.Marshal(fm)
	return string(raw)
}
