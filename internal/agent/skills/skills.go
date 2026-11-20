// Package skills adapts platform-published SKILL.md versions to the Eino
// skill middleware Backend interface. A backend instance only sees the
// bindings of one agent version; skill text is untrusted prompt content and
// never widens tool authorization.
package skills

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"gopkg.in/yaml.v3"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
)

// Store is the subset of data.MCPStore used here (kept as an interface for
// deterministic tests).
type Store interface {
	GetSkill(ctx context.Context, name string, version int64) (*domain.Skill, error)
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
		row, err := b.store.GetSkill(ctx, binding.Name, binding.Version)
		if err != nil || !row.Published {
			// Skip bindings whose skill was unpublished; the middleware only
			// offers what still exists.
			continue
		}
		fm, _ := parseBoundSkill(row.Content, row.Name)
		result = append(result, fm)
	}
	return result, nil
}

func (b *Backend) Get(ctx context.Context, name string) (skill.Skill, error) {
	for _, binding := range b.bindings {
		if binding.Name != name {
			continue
		}
		row, err := b.store.GetSkill(ctx, binding.Name, binding.Version)
		if err != nil {
			return skill.Skill{}, err
		}
		if !row.Published {
			return skill.Skill{}, errors.New("skill is not published")
		}
		fm, content := parseBoundSkill(row.Content, row.Name)
		return skill.Skill{FrontMatter: fm, Content: content}, nil
	}
	return skill.Skill{}, errors.New("skill not bound to this agent")
}

// ParseFrontMatter extracts the YAML front matter of a SKILL.md document and
// returns the parsed front matter plus the remaining body. Parsing with
// yaml.v3 (the same way Eino's skill filesystem backend does) handles quoted
// values, colons inside descriptions and multiline strings; a hand-rolled
// "key: value" split does not.
func ParseFrontMatter(content string) (skill.FrontMatter, string) {
	var fm skill.FrontMatter
	rest := strings.TrimSpace(content)
	if !strings.HasPrefix(rest, "---") {
		return fm, rest
	}
	after := rest[3:]
	if after != "" && !strings.HasPrefix(after, "\n") && !strings.HasPrefix(after, "\r\n") {
		// A body beginning with "---" (e.g. a horizontal rule) is not front
		// matter; leave it untouched.
		return fm, rest
	}
	end := strings.Index(after, "\n---")
	if end < 0 {
		return fm, rest
	}
	header := strings.TrimSpace(after[:end])
	body := strings.TrimSpace(after[end+4:])
	if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
		// Keep the body usable; callers fall back to the skill name when the
		// front matter is malformed.
		return skill.FrontMatter{}, body
	}
	return sanitizeFrontMatter(fm), body
}

// parseBoundSkill parses a stored SKILL.md and applies the platform policy:
// only inline context mode is supported, because fork modes would need
// sub-agent permission inheritance which is deliberately out of scope.
func parseBoundSkill(content, fallbackName string) (skill.FrontMatter, string) {
	fm, body := ParseFrontMatter(content)
	if fm.Name == "" {
		fm.Name = fallbackName
	}
	return sanitizeFrontMatter(fm), body
}

func sanitizeFrontMatter(fm skill.FrontMatter) skill.FrontMatter {
	fm.Context = ""
	fm.Agent = ""
	fm.Model = ""
	return fm
}
