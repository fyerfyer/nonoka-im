package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/tools/catalog"
	"nonoka-im/internal/audit"

	"github.com/go-kratos/kratos/v2/log"
)

// AgentService owns agent lifecycle and sharing usecases.
type AgentService struct {
	Store    Store
	Platform PlatformStore
	Models   ModelCatalog
	Keys     GatewayKeys
	Authz    Authorizer
	Groups   GroupDirectory
	Users    UserDirectory
	Audit    *audit.Service
	Log      *log.Helper
}

func NewAgentService(store Store, platform PlatformStore, models ModelCatalog, keys GatewayKeys, az Authorizer, groups GroupDirectory, users UserDirectory, auditor *audit.Service, logger log.Logger) *AgentService {
	return &AgentService{Store: store, Platform: platform, Models: models, Keys: keys, Authz: az, Groups: groups, Users: users, Audit: auditor, Log: log.NewHelper(logger)}
}

func (s *AgentService) List(ctx context.Context, userID int64, limit, offset int) ([]domain.Agent, int64, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	// Prefer one ListObjects call over scanning the whole table and running an
	// OpenFGA check per row. This also yields the correct visible total.
	if lister, ok := s.Authz.(interface {
		ListVisibleAgents(context.Context, int64) ([]int64, error)
	}); ok {
		ids, err := lister.ListVisibleAgents(ctx, userID)
		if err == nil {
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			total := int64(len(ids))
			if offset > len(ids) {
				offset = len(ids)
			}
			end := min(offset+limit, len(ids))
			rows, err := s.Store.ListAgentsByIDs(ctx, ids[offset:end])
			if err != nil {
				return nil, 0, offset, err
			}
			return rows, total, end, nil
		}
		// Fall through to the compatibility path for authorizers that do not
		// support ListObjects or when OpenFGA rejects the query.
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

func (s *AgentService) Detail(ctx context.Context, userID, agentID int64) (*domain.Agent, *domain.AgentVersion, bool, bool, error) {
	agent, err := s.Store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, nil, false, false, ErrNotFound
	}
	if err := s.Authz.CanViewAgent(ctx, userID, agentID); err != nil {
		return nil, nil, false, false, ErrForbidden
	}
	manager := s.Authz.CanManageAgent(ctx, userID, agentID) == nil
	owner := s.Authz.CanOwnAgent(ctx, userID, agentID) == nil
	var published *domain.AgentVersion
	if manager && agent.CurrentVersion > 0 {
		published, err = s.Store.GetVersion(ctx, agentID, agent.CurrentVersion)
		if err != nil {
			return nil, nil, manager, owner, err
		}
	}
	return agent, published, manager, owner, nil
}

func (s *AgentService) Create(ctx context.Context, userID int64, input Input) (*domain.Agent, error) {
	if err := s.validateInput(ctx, &input); err != nil {
		return nil, err
	}
	mcpJSON, skillsJSON, err := s.validateExtensions(ctx, userID, input)
	if err != nil {
		return nil, err
	}
	toolsJSON, _ := json.Marshal(input.Tools)
	presetsJSON, _ := json.Marshal(input.CapabilityPresets)
	policiesJSON, _ := json.Marshal(input.ToolPolicies)
	spec := domain.AgentSpec{
		Name: input.Name, Description: input.Description, Instruction: input.Instruction,
		ModelID: input.ModelID, Tools: string(toolsJSON),
		CapabilityPresets: string(presetsJSON), ToolPolicies: string(policiesJSON),
		MaxTokens: input.MaxTokens, MCPBindings: mcpJSON, Skills: skillsJSON,
	}
	agent, err := s.Store.CreateAgent(ctx, userID, spec, func(ctx context.Context, id int64) error {
		return s.Authz.SetAgentOwner(ctx, id, userID)
	})
	if err != nil {
		return nil, err
	}
	if err := s.record(ctx, userID, "agent.create", "agent", strconv.FormatInt(agent.ID, 10), nil, agentAuditFields(*agent)); err != nil {
		return nil, err
	}
	return agent, nil
}

func (s *AgentService) UpdateDraft(ctx context.Context, userID, agentID int64, input Input) (*domain.Agent, error) {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return nil, err
	}
	if err := s.validateInput(ctx, &input); err != nil {
		return nil, err
	}
	agent, err := s.Store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, ErrNotFound
	}
	mcpJSON, skillsJSON, err := s.validateExtensions(ctx, agent.OwnerUserID, input)
	if err != nil {
		return nil, err
	}
	toolsJSON, _ := json.Marshal(input.Tools)
	presetsJSON, _ := json.Marshal(input.CapabilityPresets)
	policiesJSON, _ := json.Marshal(input.ToolPolicies)
	spec := domain.AgentSpec{
		Name: input.Name, Description: input.Description, Instruction: input.Instruction,
		ModelID: input.ModelID, Tools: string(toolsJSON),
		CapabilityPresets: string(presetsJSON), ToolPolicies: string(policiesJSON),
		MaxTokens: input.MaxTokens, MCPBindings: mcpJSON, Skills: skillsJSON,
	}
	updated, err := s.Store.UpdateDraft(ctx, agentID, input.Revision, spec, false)
	if err != nil {
		return nil, err
	}
	if err := s.record(ctx, userID, "agent.update_draft", "agent", strconv.FormatInt(agentID, 10), agentAuditFields(*agent), agentAuditFields(*updated)); err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *AgentService) Publish(ctx context.Context, userID, agentID, revision int64, note string) (*domain.AgentVersion, error) {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return nil, err
	}
	note = strings.TrimSpace(note)
	if len(note) > 1000 {
		return nil, fmt.Errorf("%w: publish note is too long", ErrInvalidInput)
	}
	agent, err := s.Store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, ErrNotFound
	}
	if s.Models == nil {
		return nil, fmt.Errorf("%w: model catalog unavailable", ErrInvalidInput)
	}
	model, err := s.Models.Lookup(ctx, agent.DraftModelID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if !model.Enabled {
		return nil, fmt.Errorf("%w: selected model is disabled", ErrInvalidInput)
	}
	if !model.Curated {
		return nil, fmt.Errorf("%w: selected model is not curated", ErrInvalidInput)
	}
	if model.MaxOutputTokens > 0 && agent.DraftMaxTokens > model.MaxOutputTokens {
		return nil, fmt.Errorf("%w: max_tokens exceeds the current model limit %d", ErrInvalidInput, model.MaxOutputTokens)
	}
	version, err := s.Store.Publish(ctx, agentID, revision, note, userID)
	if err != nil {
		return nil, err
	}
	shares, shareErr := s.Authz.ListAgentShares(ctx, agentID)
	if shareErr != nil {
		if rerr := s.Store.RollbackPublish(ctx, agentID, version.Version); rerr != nil {
			return nil, fmt.Errorf("load agent shares: %w (rollback failed: %v)", shareErr, rerr)
		}
		return nil, fmt.Errorf("load agent shares: %w", shareErr)
	}
	if vErr := s.validateFrozenExtensions(ctx, version, len(shares) > 0); vErr != nil {
		if rerr := s.Store.RollbackPublish(ctx, agentID, version.Version); rerr != nil {
			return nil, fmt.Errorf("%w (rollback failed: %v)", vErr, rerr)
		}
		return nil, vErr
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
	if err := s.ensureGatewayKey(ctx, agentID); err != nil {
		if rerr := s.Store.RollbackPublish(ctx, agentID, version.Version); rerr != nil {
			return nil, fmt.Errorf("ensure agent gateway key: %w (rollback failed: %v)", err, rerr)
		}
		return nil, fmt.Errorf("ensure agent gateway key: %w", err)
	}
	if err := s.record(ctx, userID, "gateway_key.ensure", "agent", strconv.FormatInt(agentID, 10), nil, map[string]any{"status": "active"}); err != nil {
		return nil, err
	}
	if err := s.record(ctx, userID, "agent.publish", "agent", strconv.FormatInt(agentID, 10), agentAuditFields(*agent), versionAuditFields(*version)); err != nil {
		return nil, err
	}
	return version, nil
}

// ensureGatewayKey keeps the per-agent Bifrost key allowlist in sync with every
// published version. Keeping older version models avoids breaking in-flight
// runs when an agent changes models.
func (s *AgentService) ensureGatewayKey(ctx context.Context, agentID int64) error {
	if s.Keys == nil || s.Models == nil {
		return nil
	}
	versions, err := s.Store.ListVersions(ctx, agentID)
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	models := make([]domain.Model, 0, len(versions))
	for i := range versions {
		id := versions[i].ModelID
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		model, err := s.Models.Lookup(ctx, id)
		if err != nil {
			continue
		}
		models = append(models, *model)
	}
	if len(models) == 0 {
		return fmt.Errorf("agent %d has no published model", agentID)
	}
	return s.Keys.EnsureAgentKey(ctx, agentID, models)
}

func (s *AgentService) Versions(ctx context.Context, userID, agentID int64) ([]domain.AgentVersion, error) {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return nil, err
	}
	return s.Store.ListVersions(ctx, agentID)
}

func (s *AgentService) Version(ctx context.Context, userID, agentID, version int64) (*domain.AgentVersion, error) {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return nil, err
	}
	v, err := s.Store.GetVersion(ctx, agentID, version)
	if err != nil {
		return nil, ErrNotFound
	}
	return v, nil
}

func (s *AgentService) Restore(ctx context.Context, userID, agentID, version, revision int64) (*domain.Agent, error) {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return nil, err
	}
	if version <= 0 {
		return nil, fmt.Errorf("%w: invalid version", ErrInvalidInput)
	}
	agent, err := s.Store.RestoreDraft(ctx, agentID, version, revision)
	if err != nil {
		return nil, err
	}
	if err := s.record(ctx, userID, "agent.restore", "agent", strconv.FormatInt(agentID, 10), nil, agentAuditFields(*agent)); err != nil {
		return nil, err
	}
	return agent, nil
}

func (s *AgentService) SetStatus(ctx context.Context, userID, agentID int64, status string) error {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return err
	}
	if status != "active" && status != "disabled" {
		return fmt.Errorf("%w: invalid agent status", ErrInvalidInput)
	}
	before, _ := s.Store.GetAgent(ctx, agentID)
	if err := s.Store.SetAgentStatus(ctx, agentID, status); err != nil {
		return err
	}
	after, _ := s.Store.GetAgent(ctx, agentID)
	return s.record(ctx, userID, "agent.status", "agent", strconv.FormatInt(agentID, 10), auditAgentPtr(before), auditAgentPtr(after))
}

func (s *AgentService) Share(ctx context.Context, userID, agentID int64, kind, target, role string, enabled bool) error {
	if err := s.requireOwner(ctx, userID, agentID); err != nil {
		return err
	}
	if kind != "user" && kind != "group" {
		return fmt.Errorf("%w: invalid share kind", ErrInvalidInput)
	}
	if role == "" {
		role = "viewer"
	}
	if role != "viewer" && role != "editor" {
		return fmt.Errorf("%w: share role must be viewer or editor", ErrInvalidInput)
	}
	if enabled {
		agent, err := s.Store.GetAgent(ctx, agentID)
		if err != nil {
			return ErrNotFound
		}
		if agent.CurrentVersion > 0 {
			version, err := s.Store.GetVersion(ctx, agentID, agent.CurrentVersion)
			if err != nil {
				return ErrNotFound
			}
			if err := s.validateFrozenExtensions(ctx, version, true); err != nil {
				return fmt.Errorf("%w: %v", ErrInvalidInput, err)
			}
		}
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
		} else {
			if err := s.Groups.RemoveMember(ctx, target, agent.BotUserID); err != nil {
				return err
			}
		}
	}
	if err := s.Authz.SetAgentShare(ctx, agentID, kind, target, role, enabled); err != nil {
		return err
	}
	if kind == "group" && !enabled {
		if err := s.Authz.SetGroupMember(ctx, target, agent.BotUserID, false); err != nil {
			return err
		}
	}
	return s.record(ctx, userID, "agent.share", "agent", strconv.FormatInt(agentID, 10), nil, map[string]any{
		"kind": kind, "target": target, "role": role, "enabled": enabled,
	})
}

func (s *AgentService) Shares(ctx context.Context, userID, agentID int64) ([]domain.AgentShare, error) {
	if err := s.requireManager(ctx, userID, agentID); err != nil {
		return nil, err
	}
	return s.Authz.ListAgentShares(ctx, agentID)
}

func (s *AgentService) Delete(ctx context.Context, userID, agentID int64) error {
	if err := s.requireOwner(ctx, userID, agentID); err != nil {
		return err
	}
	before, _ := s.Store.GetAgent(ctx, agentID)
	toolNames, err := s.Store.DeleteAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if err := s.Authz.DeleteAgentTuples(ctx, agentID, toolNames); err != nil {
		s.Log.Warnf("delete agent FGA tuples: agent_id=%d err=%v", agentID, err)
	}
	revokeStatus := "not_managed"
	if s.Keys != nil {
		if err := s.Keys.RevokeAgentKey(ctx, agentID); err != nil {
			revokeStatus = "revoked_pending"
			s.Log.Warnf("revoke agent gateway key: agent_id=%d err=%v (queued for cleanup)", agentID, err)
		} else {
			revokeStatus = "revoked"
		}
	}
	if err := s.record(ctx, userID, "gateway_key.revoke", "agent", strconv.FormatInt(agentID, 10), nil, map[string]any{"status": revokeStatus}); err != nil {
		return err
	}
	if err := s.record(ctx, userID, "agent.delete", "agent", strconv.FormatInt(agentID, 10), auditAgentPtr(before), nil); err != nil {
		return err
	}
	return nil
}

func auditAgentPtr(a *domain.Agent) map[string]any {
	if a == nil {
		return nil
	}
	return agentAuditFields(*a)
}

func (s *AgentService) VerifyBotSend(ctx context.Context, agentID, botID int64, topic string) error {
	agent, err := s.Store.GetAgent(ctx, agentID)
	if err != nil || agent.BotUserID != botID || agent.CurrentVersion <= 0 || agent.Status != "active" {
		return ErrForbidden
	}
	if err := s.Authz.CanAccessTopic(ctx, botID, topic); err != nil {
		return ErrForbidden
	}
	return nil
}

func (s *AgentService) requireManager(ctx context.Context, userID, agentID int64) error {
	if s.Authz.CanManageAgent(ctx, userID, agentID) != nil {
		return ErrForbidden
	}
	return nil
}

func (s *AgentService) requireOwner(ctx context.Context, userID, agentID int64) error {
	if s.Authz.CanOwnAgent(ctx, userID, agentID) != nil {
		return ErrForbidden
	}
	return nil
}

func (s *AgentService) validateExtensions(ctx context.Context, ownerID int64, input Input) (string, string, error) {
	mcpBindings := make([]domain.MCPBinding, 0, len(input.MCPBindings))
	seen := map[string]bool{}
	for _, binding := range input.MCPBindings {
		if binding.ServerID == 0 || binding.Tool == "" || strings.ContainsAny(binding.Tool, " \t\n/") {
			return "", "", fmt.Errorf("%w: invalid mcp binding", ErrInvalidInput)
		}
		key := fmt.Sprintf("%d/%s", binding.ServerID, binding.Tool)
		if seen[key] {
			return "", "", fmt.Errorf("%w: duplicate mcp binding %q", ErrInvalidInput, key)
		}
		seen[key] = true
		server, err := s.Platform.GetServerByID(ctx, binding.ServerID)
		if err != nil || !server.Enabled {
			return "", "", fmt.Errorf("%w: mcp server %d is not available", ErrInvalidInput, binding.ServerID)
		}
		if server.OwnerUserID != ownerID && server.Visibility == domain.VisibilityPrivate {
			return "", "", fmt.Errorf("%w: mcp server %d is private", ErrInvalidInput, binding.ServerID)
		}
		if server.Visibility != domain.VisibilityPrivate && server.ReviewStatus != domain.ReviewApproved {
			return "", "", fmt.Errorf("%w: mcp server %d is not approved", ErrInvalidInput, binding.ServerID)
		}
		tool, err := s.Platform.GetTool(ctx, binding.ServerID, binding.Tool)
		if err != nil {
			return "", "", fmt.Errorf("%w: tool %q is not available on mcp server %d", ErrInvalidInput, binding.Tool, binding.ServerID)
		}
		policy := binding.Policy
		if policy == "" {
			policy = catalog.PolicyAuto
			if tool.Risk == domain.MCPRiskWrite {
				policy = catalog.PolicyAsk
			}
		}
		if policy != catalog.PolicyAuto && policy != catalog.PolicyAsk && policy != catalog.PolicyDeny {
			return "", "", fmt.Errorf("%w: invalid mcp tool policy", ErrInvalidInput)
		}
		mcpBindings = append(mcpBindings, domain.MCPBinding{
			ServerID: binding.ServerID, Server: server.Name, Tool: binding.Tool,
			Exposed:    domain.MCPExposedName(binding.ServerID, binding.Tool),
			SchemaHash: tool.SchemaHash, Policy: policy,
		})
	}
	skills := make([]domain.SkillBinding, 0, len(input.Skills))
	seenSkill := map[uint64]bool{}
	for _, binding := range input.Skills {
		if binding.SkillID == 0 || binding.Version <= 0 {
			return "", "", fmt.Errorf("%w: invalid skill binding", ErrInvalidInput)
		}
		if seenSkill[binding.SkillID] {
			return "", "", fmt.Errorf("%w: duplicate skill binding %d", ErrInvalidInput, binding.SkillID)
		}
		seenSkill[binding.SkillID] = true
		skill, err := s.Platform.GetSkillByID(ctx, binding.SkillID)
		if err != nil {
			return "", "", fmt.Errorf("%w: skill %d not found", ErrInvalidInput, binding.SkillID)
		}
		if skill.OwnerUserID != ownerID && skill.Visibility == domain.VisibilityPrivate {
			return "", "", fmt.Errorf("%w: skill %d is private", ErrInvalidInput, binding.SkillID)
		}
		sv, err := s.Platform.GetSkillVersion(ctx, binding.SkillID, binding.Version)
		if err != nil {
			return "", "", fmt.Errorf("%w: skill %d version %d not found", ErrInvalidInput, binding.SkillID, binding.Version)
		}
		if skill.Visibility != domain.VisibilityPrivate &&
			(skill.ReviewStatus != domain.ReviewApproved || sv.ReviewStatus != domain.ReviewApproved) {
			return "", "", fmt.Errorf("%w: skill %d version %d is not approved", ErrInvalidInput, binding.SkillID, binding.Version)
		}
		skills = append(skills, domain.SkillBinding{
			SkillID: binding.SkillID, Name: skill.Slug, Version: binding.Version, Checksum: sv.Checksum,
		})
	}
	mcpJSON, _ := json.Marshal(mcpBindings)
	skillsJSON, _ := json.Marshal(skills)
	return string(mcpJSON), string(skillsJSON), nil
}

// validateFrozenExtensions re-checks the published snapshot against the live
// registry. It prevents publishing stale MCP schemas and keeps private
// resources out of agents that have active shares.
func (s *AgentService) validateFrozenExtensions(ctx context.Context, version *domain.AgentVersion, sharedAgent bool) error {
	var bindings []domain.MCPBinding
	if version.MCPBindings != "" {
		if err := json.Unmarshal([]byte(version.MCPBindings), &bindings); err != nil {
			return fmt.Errorf("%w: invalid mcp binding snapshot", ErrInvalidInput)
		}
	}
	for _, binding := range bindings {
		server, err := s.Platform.GetServerByID(ctx, binding.ServerID)
		if err != nil || !server.Enabled {
			return fmt.Errorf("%w: mcp server %d is not available", ErrInvalidInput, binding.ServerID)
		}
		if sharedAgent && server.Visibility == domain.VisibilityPrivate {
			return fmt.Errorf("%w: private mcp server %d cannot be used by a shared agent", ErrInvalidInput, binding.ServerID)
		}
		if server.Visibility != domain.VisibilityPrivate && server.ReviewStatus != domain.ReviewApproved {
			return fmt.Errorf("%w: mcp server %d is not approved", ErrInvalidInput, binding.ServerID)
		}
		tool, err := s.Platform.GetTool(ctx, binding.ServerID, binding.Tool)
		if err != nil {
			return fmt.Errorf("%w: mcp tool %q is no longer available", ErrInvalidInput, binding.Tool)
		}
		if binding.SchemaHash != "" && tool.SchemaHash != binding.SchemaHash {
			return fmt.Errorf("%w: mcp tool %q schema changed; re-save the draft and republish", ErrInvalidInput, binding.Tool)
		}
	}
	var skills []domain.SkillBinding
	if version.Skills != "" {
		if err := json.Unmarshal([]byte(version.Skills), &skills); err != nil {
			return fmt.Errorf("%w: invalid skill binding snapshot", ErrInvalidInput)
		}
	}
	for _, binding := range skills {
		skill, err := s.Platform.GetSkillByID(ctx, binding.SkillID)
		if err != nil {
			return fmt.Errorf("%w: skill %d is not available", ErrInvalidInput, binding.SkillID)
		}
		if sharedAgent && skill.Visibility == domain.VisibilityPrivate {
			return fmt.Errorf("%w: private skill %d cannot be bound to a shared agent", ErrInvalidInput, binding.SkillID)
		}
		sv, err := s.Platform.GetSkillVersion(ctx, binding.SkillID, binding.Version)
		if err != nil {
			return fmt.Errorf("%w: skill %d version %d is not available", ErrInvalidInput, binding.SkillID, binding.Version)
		}
		if skill.Visibility != domain.VisibilityPrivate &&
			(skill.ReviewStatus != domain.ReviewApproved || sv.ReviewStatus != domain.ReviewApproved) {
			return fmt.Errorf("%w: skill %d version %d is not approved", ErrInvalidInput, binding.SkillID, binding.Version)
		}
		if binding.Checksum != "" && sv.Checksum != binding.Checksum {
			return fmt.Errorf("%w: skill %d version %d content changed", ErrInvalidInput, binding.SkillID, binding.Version)
		}
	}
	return nil
}

func (s *AgentService) validateInput(ctx context.Context, input *Input) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Instruction = strings.TrimSpace(input.Instruction)
	if len(input.Name) == 0 || len(input.Name) > 100 || input.Instruction == "" || len(input.Instruction) > 20000 || len(input.Description) > 4000 {
		return fmt.Errorf("%w: invalid agent name, description or instruction", ErrInvalidInput)
	}
	if input.ModelID <= 0 {
		return fmt.Errorf("%w: a model is required", ErrInvalidInput)
	}
	if s.Models == nil {
		return fmt.Errorf("%w: model catalog unavailable", ErrInvalidInput)
	}
	model, err := s.Models.Lookup(ctx, input.ModelID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if !model.Enabled {
		return fmt.Errorf("%w: model %q is disabled", ErrInvalidInput, model.ModelKey)
	}
	if !model.Curated {
		return fmt.Errorf("%w: model %q is not curated", ErrInvalidInput, model.ModelKey)
	}
	if input.MaxTokens < 0 || input.MaxTokens > 128000 {
		return fmt.Errorf("%w: invalid max_tokens", ErrInvalidInput)
	}
	if input.MaxTokens == 0 {
		input.MaxTokens = 4096
	}
	if model.MaxOutputTokens > 0 && input.MaxTokens > model.MaxOutputTokens {
		return fmt.Errorf("%w: max_tokens exceeds model limit %d", ErrInvalidInput, model.MaxOutputTokens)
	}
	allowed := make(map[string]bool)
	system := make(map[string]bool)
	for _, def := range catalog.BuiltinDefinitions() {
		allowed[def.Name] = true
		if def.System {
			system[def.Name] = true
		}
	}
	for name, policy := range input.ToolPolicies {
		if !allowed[name] {
			return fmt.Errorf("%w: policy refers to unknown builtin tool %q", ErrInvalidInput, name)
		}
		if policy != catalog.PolicyAuto && policy != catalog.PolicyAsk && policy != catalog.PolicyDeny {
			return fmt.Errorf("%w: tool policy must be auto, ask or deny", ErrInvalidInput)
		}
		if system[name] && policy == catalog.PolicyDeny {
			return fmt.Errorf("%w: system tools cannot be disabled", ErrInvalidInput)
		}
	}
	if len(input.CapabilityPresets) == 0 && len(input.Tools) == 0 {
		input.CapabilityPresets = catalog.DefaultPresetKeys()
	}
	resolved, err := catalog.ResolveTools(input.CapabilityPresets, input.Tools)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	filtered := resolved[:0]
	for _, name := range resolved {
		if input.ToolPolicies[name] == catalog.PolicyDeny && !system[name] {
			continue
		}
		filtered = append(filtered, name)
	}
	input.Tools = filtered
	return nil
}
