package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"nonoka-im/internal/agent/domain"
)

// MCPInput is the transport-neutral MCP server registration request.
type MCPInput struct {
	Name    string
	URL     string
	Tools   []string
	Risk    string
	Enabled bool
}

// MCPService owns the platform MCP registry.
type MCPService struct{ Platform PlatformStore }

func NewMCPService(platform PlatformStore) *MCPService { return &MCPService{Platform: platform} }

func (s *MCPService) Register(ctx context.Context, userID int64, input MCPInput) (*domain.MCPServer, error) {
	if !IsAdmin(userID) {
		return nil, ErrAdminRequired
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 || strings.ContainsAny(input.Name, " \t\n/") {
		return nil, fmt.Errorf("%w: invalid server name", ErrInvalidInput)
	}
	u, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: url must be an http(s) URL", ErrInvalidInput)
	}
	if len(input.Tools) == 0 || len(input.Tools) > 100 {
		return nil, fmt.Errorf("%w: tools must list 1-100 allowed tool names", ErrInvalidInput)
	}
	seen := map[string]bool{}
	for _, name := range input.Tools {
		if name == "" || strings.ContainsAny(name, " \t\n") || seen[name] {
			return nil, fmt.Errorf("%w: invalid or duplicate tool name %q", ErrInvalidInput, name)
		}
		seen[name] = true
	}
	if input.Risk == "" {
		input.Risk = "read_only"
	}
	if input.Risk != "read_only" && input.Risk != "write" {
		return nil, fmt.Errorf("%w: risk must be read_only or write", ErrInvalidInput)
	}
	toolsJSON, _ := json.Marshal(input.Tools)
	return s.Platform.RegisterServer(ctx, input.Name, "http", u.String(), string(toolsJSON), input.Risk)
}

func (s *MCPService) List(ctx context.Context, userID int64) ([]domain.MCPServer, error) {
	if !IsAdmin(userID) {
		return nil, ErrAdminRequired
	}
	return s.Platform.ListServers(ctx)
}

func (s *MCPService) SetStatus(ctx context.Context, userID int64, id uint64, enabled bool) error {
	if !IsAdmin(userID) {
		return ErrAdminRequired
	}
	return s.Platform.SetServerEnabled(ctx, id, enabled)
}
