package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/agent/tools/catalog"
	"nonoka-im/internal/authz"

	"github.com/go-kratos/kratos/v2/log"
)

// AgentService owns agent lifecycle and sharing usecases.
type AgentService struct {
	Store    Store
	Platform PlatformStore
	Authz    Authorizer
	Groups   GroupDirectory
	Users    UserDirectory
	Sealer   APIKeySealer
	Log      *log.Helper
}

func NewAgentService(store Store, platform PlatformStore, az Authorizer, groups GroupDirectory, users UserDirectory, sealer APIKeySealer, logger log.Logger) *AgentService {
	return &AgentService{Store: store, Platform: platform, Authz: az, Groups: groups, Users: users, Sealer: sealer, Log: log.NewHelper(logger)}
}

func (s *AgentService) List(ctx context.Context, userID int64, limit, offset int) ([]domain.Agent, int64, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	visible := make([]domain.Agent, 0, limit)
	scanned := offset
	for len(visible) < limit {
		rows, total, err := s.Store.ListAgents(ctx, limit-len(visible), scanned)
		if err != nil {
			return nil, 0, scanned, err
		}
		if len(rows) == 0 {
			scanned = int(total)
			break
		}
		for i := range rows {
			scanned++
			if s.Authz.CanViewAgent(ctx, userID, rows[i].ID) == nil {
				visible = append(visible, rows[i])
				if len(visible) == limit {
					break
				}
			}
		}
		if int64(scanned) >= total {
			break
		}
	}
	_, total, err := s.Store.ListAgents(ctx, 1, 0)
	if err != nil {
		return nil, 0, scanned, err
	}
	return visible, total, scanned, nil
}

func (s *AgentService) Detail(ctx context.Context, userID, agentID int64) (*domain.Agent, *domain.AgentVersion, bool, error) {
	agent, err := s.Store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, nil, false, ErrNotFound
	}
	if err := s.Authz.CanViewAgent(ctx, userID, agentID); err != nil {
		return nil, nil, false, ErrForbidden
	}
	manager := s.Authz.CanManageAgent(ctx, userID, agentID) == nil
	var published *domain.AgentVersion
	if manager && agent.CurrentVersion > 0 {
		published, err = s.Store.GetVersion(ctx, agentID, agent.CurrentVersion)
		if err != nil {
			return nil, nil, manager, err
		}
	}
	return agent, published, manager, nil
}

func (s *AgentService) Create(ctx context.Context, userID int64, input Input) (*domain.Agent, error) {
	if err := validateInput(&input); err != nil {
		return nil, err
	}
	mcpJSON, skillsJSON, err := s.validateExtensions(ctx, input)
	if err != nil {
		return nil, err
	}
	sealed, _, err := s.seal(input.APIKey)
	if err != nil {
		return nil, err
	}
	toolsJSON, _ := json.Marshal(input.Tools)
	spec := domain.AgentSpec{
		Name: input.Name, Description: input.Description, Instruction: input.Instruction,
		Model: input.Model, Tools: string(toolsJSON), BaseURL: input.BaseURL, APIKeyEnc: sealed,
		MaxTokens: input.MaxTokens, MCPBindings: mcpJSON, Skills: skillsJSON,
	}
	return s.Store.CreateAgent(ctx, userID, spec, func(ctx context.Context, id int64) error {
		return s.Authz.SetAgentOwner(ctx, id, userID)
	})
}

func (s *AgentService) UpdateDraft(ctx context.Context, userID, agentID int64, input Input) (*domain.Agent, error) {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return nil, err
	}
	if err := validateInput(&input); err != nil {
		return nil, err
	}
	mcpJSON, skillsJSON, err := s.validateExtensions(ctx, input)
	if err != nil {
		return nil, err
	}
	sealed, updateKey, err := s.seal(input.APIKey)
	if err != nil {
		return nil, err
	}
	toolsJSON, _ := json.Marshal(input.Tools)
	spec := domain.AgentSpec{
		Name: input.Name, Description: input.Description, Instruction: input.Instruction,
		Model: input.Model, Tools: string(toolsJSON), BaseURL: input.BaseURL, APIKeyEnc: sealed,
		MaxTokens: input.MaxTokens, MCPBindings: mcpJSON, Skills: skillsJSON,
	}
	return s.Store.UpdateDraft(ctx, agentID, input.Revision, spec, updateKey)
}

func (s *AgentService) Publish(ctx context.Context, userID, agentID, revision int64) (*domain.AgentVersion, error) {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return nil, err
	}
	version, err := s.Store.Publish(ctx, agentID, revision)
	if err != nil {
		return nil, err
	}
	var toolNames []string
	if err := json.Unmarshal([]byte(version.Tools), &toolNames); err != nil {
		return nil, err
	}
	var bindErr error
	var bindTool string
	for _, tool := range toolNames {
		if err := s.Authz.SetAgentToolBinding(ctx, agentID, tool, true); err != nil {
			bindTool, bindErr = tool, err
			break
		}
	}
	var bindings []domain.MCPBinding
	if version.MCPBindings != "" {
		if err := json.Unmarshal([]byte(version.MCPBindings), &bindings); err != nil {
			return nil, err
		}
	}
	for _, binding := range bindings {
		if err := s.Authz.SetAgentToolBinding(ctx, agentID, binding.Exposed, true); err != nil {
			if bindErr == nil {
				bindTool, bindErr = binding.Exposed, err
			}
			break
		}
	}
	if bindErr != nil {
		if rerr := s.Store.RollbackPublish(ctx, agentID, version.Version); rerr != nil {
			return nil, fmt.Errorf("bind tool %q: %w (rollback failed: %v)", bindTool, bindErr, rerr)
		}
		return nil, fmt.Errorf("bind tool %q: %w", bindTool, bindErr)
	}
	return version, nil
}

func (s *AgentService) SetStatus(ctx context.Context, userID, agentID int64, status string) error {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return err
	}
	if status != "active" && status != "disabled" {
		return fmt.Errorf("%w: invalid agent status", ErrInvalidInput)
	}
	return s.Store.SetAgentStatus(ctx, agentID, status)
}

func (s *AgentService) Share(ctx context.Context, userID, agentID int64, kind, target string, enabled bool) error {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return err
	}
	if kind != "user" && kind != "group" {
		return fmt.Errorf("%w: invalid share kind", ErrInvalidInput)
	}
	if kind == "user" {
		uid, err := strconv.ParseInt(target, 10, 64)
		if err != nil || uid <= 0 {
			return fmt.Errorf("%w: invalid target user", ErrInvalidInput)
		}
		ok, err := s.Users.UserExists(ctx, uid)
		if err != nil || !ok {
			return fmt.Errorf("%w: target user does not exist", ErrInvalidInput)
		}
	}
	agent, err := s.Store.GetAgent(ctx, agentID)
	if err != nil {
		return ErrNotFound
	}
	if kind == "group" {
		if s.Authz.CanManageGroup(ctx, userID, target) != nil {
			return ErrForbidden
		}
		if enabled {
			if err := s.Groups.AddMember(ctx, target, agent.BotUserID); err != nil {
				return err
			}
			if err := s.Authz.SetGroupMember(ctx, target, agent.BotUserID, true); err != nil {
				return err
			}
		}
	}
	if err := s.Authz.SetAgentShare(ctx, agentID, kind, target, enabled); err != nil {
		return err
	}
	if kind == "group" && !enabled {
		return s.Authz.SetGroupMember(ctx, target, agent.BotUserID, false)
	}
	return nil
}

func (s *AgentService) Shares(ctx context.Context, userID, agentID int64) ([]authz.AgentShare, error) {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return nil, err
	}
	return s.Authz.ListAgentShares(ctx, agentID)
}

func (s *AgentService) Delete(ctx context.Context, userID, agentID int64) error {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return err
	}
	toolNames, err := s.Store.DeleteAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if err := s.Authz.DeleteAgentTuples(ctx, agentID, toolNames); err != nil {
		s.Log.Warnf("delete agent FGA tuples: agent_id=%d err=%v", agentID, err)
	}
	return nil
}

func (s *AgentService) VerifyBotSend(ctx context.Context, agentID, botID int64, topic string) error {
	agent, err := s.Store.GetAgent(ctx, agentID)
	if err != nil || agent.BotUserID != botID || agent.CurrentVersion <= 0 || agent.Status != "active" {
		return authz.ErrDenied
	}
	if err := s.Authz.CanAccessTopic(ctx, botID, topic); err != nil {
		return authz.ErrDenied
	}
	return nil
}

func (s *AgentService) requireManager(ctx context.Context, userID, agentID int64) error {
	if s.Authz.CanManageAgent(ctx, userID, agentID) != nil {
		return ErrForbidden
	}
	return nil
}

func (s *AgentService) seal(apiKey string) (string, bool, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", false, nil
	}
	if s.Sealer == nil {
		return "", false, fmt.Errorf("model credential sealing unavailable")
	}
	return s.Sealer.Seal(apiKey)
}

func (s *AgentService) validateExtensions(ctx context.Context, input Input) (string, string, error) {
	mcpBindings := make([]domain.MCPBinding, 0, len(input.MCPBindings))
	seen := map[string]bool{}
	for _, binding := range input.MCPBindings {
		if binding.Server == "" || binding.Tool == "" || strings.ContainsAny(binding.Server+binding.Tool, " \t\n") {
			return "", "", fmt.Errorf("%w: invalid mcp binding", ErrInvalidInput)
		}
		key := binding.Server + "/" + binding.Tool
		if seen[key] {
			return "", "", fmt.Errorf("%w: duplicate mcp binding %q", ErrInvalidInput, key)
		}
		seen[key] = true
		server, err := s.Platform.GetServer(ctx, binding.Server)
		if err != nil || !server.Enabled {
			return "", "", fmt.Errorf("%w: mcp server %q is not available", ErrInvalidInput, binding.Server)
		}
		var allowed []string
		if err := json.Unmarshal([]byte(server.AllowedTools), &allowed); err != nil {
			return "", "", fmt.Errorf("%w: mcp server %q has an invalid tool list", ErrInvalidInput, binding.Server)
		}
		found := false
		for _, name := range allowed {
			if name == binding.Tool {
				found = true
				break
			}
		}
		if !found {
			return "", "", fmt.Errorf("%w: tool %q is not allowed on mcp server %q", ErrInvalidInput, binding.Tool, binding.Server)
		}
		mcpBindings = append(mcpBindings, domain.MCPBinding{Server: binding.Server, Tool: binding.Tool, Exposed: tools.ExposedMCPName(binding.Server, binding.Tool)})
	}
	skills := make([]domain.SkillBinding, 0, len(input.Skills))
	seenSkill := map[string]bool{}
	for _, binding := range input.Skills {
		if binding.Name == "" || binding.Version <= 0 {
			return "", "", fmt.Errorf("%w: invalid skill binding", ErrInvalidInput)
		}
		if seenSkill[binding.Name] {
			return "", "", fmt.Errorf("%w: duplicate skill binding %q", ErrInvalidInput, binding.Name)
		}
		seenSkill[binding.Name] = true
		row, err := s.Platform.GetSkill(ctx, binding.Name, binding.Version)
		if err != nil || !row.Published {
			return "", "", fmt.Errorf("%w: skill %q version %d is not published", ErrInvalidInput, binding.Name, binding.Version)
		}
		skills = append(skills, domain.SkillBinding{Name: binding.Name, Version: binding.Version})
	}
	mcpJSON, _ := json.Marshal(mcpBindings)
	skillsJSON, _ := json.Marshal(skills)
	return string(mcpJSON), string(skillsJSON), nil
}

func validateInput(input *Input) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Instruction = strings.TrimSpace(input.Instruction)
	input.Model = strings.TrimSpace(input.Model)
	if input.Model == "" {
		input.Model = os.Getenv("OPENAI_MODEL")
	}
	if len(input.Name) == 0 || len(input.Name) > 100 || input.Instruction == "" || len(input.Instruction) > 20000 || len(input.Description) > 4000 {
		return fmt.Errorf("%w: invalid agent name, description or instruction", ErrInvalidInput)
	}
	if len(input.Model) > 100 {
		return fmt.Errorf("%w: invalid model", ErrInvalidInput)
	}
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	if input.BaseURL != "" {
		if err := validateModelBaseURL(input.BaseURL); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}
	if len(input.APIKey) > 512 {
		return fmt.Errorf("%w: invalid api_key", ErrInvalidInput)
	}
	if input.MaxTokens < 0 || input.MaxTokens > 128000 {
		return fmt.Errorf("%w: invalid max_tokens", ErrInvalidInput)
	}
	allowed := make(map[string]bool)
	for _, def := range catalog.BuiltinDefinitions() {
		allowed[def.Name] = true
	}
	seen := map[string]bool{}
	for _, name := range input.Tools {
		if !allowed[name] || seen[name] {
			return fmt.Errorf("%w: unsupported or duplicate tool %q", ErrInvalidInput, name)
		}
		seen[name] = true
	}
	if input.MaxTokens == 0 {
		input.MaxTokens = 4096
	}
	return nil
}

// validateModelBaseURL is the transitional SSRF guard for agent-owned model
// endpoints. Shared agents must eventually use the platform Model Service.
func validateModelBaseURL(raw string) error {
	if len(raw) > 200 {
		return fmt.Errorf("invalid base_url")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("base_url must be an http(s) URL")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("base_url must include a host")
	}
	if u.Scheme == "http" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("base_url must use https")
	}
	if allow := strings.TrimSpace(os.Getenv("AGENT_MODEL_ALLOWED_HOSTS")); allow != "" {
		allowed := false
		for _, entry := range strings.Split(allow, ",") {
			entry = strings.ToLower(strings.TrimSpace(entry))
			if entry != "" && (entry == host || strings.HasSuffix(host, "."+entry)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("base_url host is not in AGENT_MODEL_ALLOWED_HOSTS")
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("base_url must not target a private or loopback address")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("base_url host %q could not be resolved", host)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("base_url must not target a private or loopback address")
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast()
}

// IsAdmin reports whether the user is a configured platform admin.
func IsAdmin(userID int64) bool {
	for _, part := range strings.Split(os.Getenv("AGENT_ADMIN_USER_IDS"), ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id == userID {
			return true
		}
	}
	return false
}
