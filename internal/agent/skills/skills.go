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

	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/data"
)

// Store is the subset of data.MCPStore used here (kept as an interface for
// deterministic tests).
type Store interface {
	GetSkill(ctx context.Context, name string, version int64) (*data.Skill, error)
}

// Backend lists and reads only the skills bound to one agent version.
type Backend struct {
	store    Store
	bindings []runtime.SkillBinding
}

func NewBackend(store Store, bindings []runtime.SkillBinding) *Backend {
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
		fm, _ := ParseFrontMatter(row.Content)
		if fm.Name == "" {
			fm.Name = row.Name
		}
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
		fm, content := ParseFrontMatter(row.Content)
		if fm.Name == "" {
			fm.Name = row.Name
		}
		// Only inline context mode is supported: fork modes would need
		// sub-agent permission inheritance which is deliberately out of scope.
		fm.Context = ""
		fm.Agent = ""
		fm.Model = ""
		return skill.Skill{FrontMatter: fm, Content: content}, nil
	}
	return skill.Skill{}, errors.New("skill not bound to this agent")
}

// ParseFrontMatter extracts the YAML front matter of a SKILL.md document and
// returns the parsed front matter plus the remaining body.
func ParseFrontMatter(content string) (skill.FrontMatter, string) {
	var fm skill.FrontMatter
	rest := strings.TrimSpace(content)
	if !strings.HasPrefix(rest, "---") {
		return fm, rest
	}
	end := strings.Index(rest[3:], "\n---")
	if end < 0 {
		return fm, rest
	}
	header := rest[3 : 3+end]
	body := strings.TrimSpace(rest[3+end+4:])
	// Minimal yaml subset: parse simple "key: value" lines so we do not need
	// to export yaml parsing behavior into the runtime contract.
	for _, line := range strings.Split(header, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "name":
			fm.Name = value
		case "description":
			fm.Description = value
		}
	}
	return fm, body
}
