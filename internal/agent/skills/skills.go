// Package skills adapts platform-published SKILL.md versions to the Eino
// skill middleware Backend interface. A backend instance only sees the
// bindings of one agent version; skill text is untrusted prompt content and
// never widens tool authorization.
package skills

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/adk/middlewares/skill"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
	"nonoka-im/internal/agent/skillspec"
)

// Store is the subset of the platform store used here.
type Store interface {
	GetSkillVersion(ctx context.Context, skillID uint64, version int64) (*domain.SkillVersion, error)
}

// Backend lists and reads only the skills bound to one agent version.
type Backend struct {
	store    Store
	bindings []port.SkillBinding
}

func NewBackend(store Store, bindings []port.SkillBinding) *Backend {
	return &Backend{store: store, bindings: bindings}
}

func (b *Backend) List(ctx context.Context) ([]skill.FrontMatter, error) {
	result := make([]skill.FrontMatter, 0, len(b.bindings))
	for _, binding := range b.bindings {
		row, err := b.load(ctx, binding)
		if err != nil {
			return nil, err
		}
		fm, _, err := skillspec.Parse(row.Content)
		if err != nil {
			return nil, fmt.Errorf("skill %d/%d: %w", binding.SkillID, binding.Version, err)
		}
		result = append(result, toEinoFrontMatter(fm))
	}
	return result, nil
}

func (b *Backend) Get(ctx context.Context, name string) (skill.Skill, error) {
	for _, binding := range b.bindings {
		row, err := b.load(ctx, binding)
		if err != nil {
			return skill.Skill{}, err
		}
		fm, body, err := skillspec.Parse(row.Content)
		if err != nil {
			return skill.Skill{}, err
		}
		if fm.Name != name {
			continue
		}
		return skill.Skill{FrontMatter: toEinoFrontMatter(fm), Content: body}, nil
	}
	return skill.Skill{}, errors.New("skill not bound to this agent")
}

func (b *Backend) load(ctx context.Context, binding port.SkillBinding) (*domain.SkillVersion, error) {
	if binding.SkillID == 0 || binding.Version <= 0 {
		return nil, fmt.Errorf("invalid skill binding")
	}
	row, err := b.store.GetSkillVersion(ctx, binding.SkillID, binding.Version)
	if err != nil {
		return nil, err
	}
	if binding.Checksum != "" && row.Checksum != binding.Checksum {
		return nil, fmt.Errorf("skill %d version %d checksum changed; republish the agent", binding.SkillID, binding.Version)
	}
	if row.Status != domain.SkillVersionPublished && row.Status != domain.SkillVersionDeprecated {
		return nil, fmt.Errorf("skill %d version %d is not published", binding.SkillID, binding.Version)
	}
	return row, nil
}

func toEinoFrontMatter(fm skillspec.FrontMatter) skill.FrontMatter {
	// Execution modes are validated out at upload time; keep the runtime
	// defense-in-depth sanitization as well.
	return skill.FrontMatter{Name: fm.Name, Description: fm.Description}
}

// ParseFrontMatter is kept for compatibility with existing callers/tests.
func ParseFrontMatter(content string) (skill.FrontMatter, string) {
	fm, body, _ := skillspec.Parse(content)
	return toEinoFrontMatter(fm), body
}
