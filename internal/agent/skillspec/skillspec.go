// Package skillspec parses and validates SKILL.md documents without depending
// on the Eino runtime or any transport. It is shared by the management API and
// the runtime skill backend.
package skillspec

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// FrontMatter is the supported SKILL.md header. Unknown fields are ignored;
// unsupported execution modes are rejected by Validate.
type FrontMatter struct {
	Name         string            `yaml:"name" json:"name"`
	Description  string            `yaml:"description" json:"description"`
	Version      string            `yaml:"version,omitempty" json:"version,omitempty"`
	License      string            `yaml:"license,omitempty" json:"license,omitempty"`
	AllowedTools []string          `yaml:"allowed-tools,omitempty" json:"allowedTools,omitempty"`
	Metadata     map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	// Deprecated/unsupported fields are parsed only so validation can reject
	// them explicitly instead of silently accepting a fork-capable document.
	Context string `yaml:"context,omitempty" json:"-"`
	Agent   string `yaml:"agent,omitempty" json:"-"`
	Model   string `yaml:"model,omitempty" json:"-"`
}

// Parse extracts front matter and the body. It returns an error only when the
// document declares a header that cannot be parsed; callers that need strict
// validation should call Validate.
func Parse(content string) (FrontMatter, string, error) {
	var fm FrontMatter
	rest := strings.TrimSpace(content)
	if !strings.HasPrefix(rest, "---") {
		return fm, rest, nil
	}
	after := rest[3:]
	if after != "" && !strings.HasPrefix(after, "\n") && !strings.HasPrefix(after, "\r\n") {
		return fm, rest, nil
	}
	end := strings.Index(after, "\n---")
	if end < 0 {
		return fm, rest, fmt.Errorf("unterminated front matter")
	}
	header := strings.TrimSpace(after[:end])
	body := strings.TrimSpace(after[end+4:])
	if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
		return fm, body, fmt.Errorf("invalid front matter: %w", err)
	}
	return fm, body, nil
}

// Validate enforces the platform's skill contract and returns the parsed
// header, body and SHA-256 checksum of the original content.
func Validate(content, slug string) (FrontMatter, string, string, error) {
	if strings.TrimSpace(content) == "" {
		return FrontMatter{}, "", "", fmt.Errorf("skill content is empty")
	}
	if len(content) > 200_000 {
		return FrontMatter{}, "", "", fmt.Errorf("skill content exceeds 200000 bytes")
	}
	fm, body, err := Parse(content)
	if err != nil {
		return FrontMatter{}, "", "", err
	}
	if fm.Name == "" || fm.Description == "" {
		return FrontMatter{}, "", "", fmt.Errorf("skill front matter requires name and description")
	}
	if slug != "" && fm.Name != slug {
		return FrontMatter{}, "", "", fmt.Errorf("skill name %q does not match slug %q", fm.Name, slug)
	}
	if len(fm.Name) > 100 || len(fm.Description) > 2000 {
		return FrontMatter{}, "", "", fmt.Errorf("skill name or description is too long")
	}
	if body == "" {
		return FrontMatter{}, "", "", fmt.Errorf("skill body is empty")
	}
	if fm.Context != "" || fm.Agent != "" || fm.Model != "" {
		return FrontMatter{}, "", "", fmt.Errorf("skill execution mode (context/agent/model) is not supported")
	}
	if len(fm.AllowedTools) > 64 {
		return FrontMatter{}, "", "", fmt.Errorf("too many allowed-tools entries")
	}
	sum := sha256.Sum256([]byte(content))
	return fm, body, hex.EncodeToString(sum[:]), nil
}

// Checksum returns the SHA-256 of raw content.
func Checksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
