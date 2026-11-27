package service

import (
	"context"
	"encoding/json"

	agentv1 "nonoka-im/api/agent/v1"
	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/httpapi"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type draftMCPBinding struct {
	ServerID   uint64 `json:"serverId"`
	Server     string `json:"server"`
	Tool       string `json:"tool"`
	Exposed    string `json:"exposed"`
	SchemaHash string `json:"schemaHash"`
	Policy     string `json:"policy"`
}

type draftSkillBinding struct {
	SkillID  uint64 `json:"skillId"`
	Name     string `json:"name"`
	Version  int64  `json:"version"`
	Checksum string `json:"checksum"`
}

func parseStringJSON(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func parsePolicyJSON(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	out := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func parseMCPBindings(raw string) []*agentv1.MCPBinding {
	if raw == "" {
		return nil
	}
	var rows []draftMCPBinding
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]*agentv1.MCPBinding, 0, len(rows))
	for i := range rows {
		out = append(out, &agentv1.MCPBinding{
			ServerId: rows[i].ServerID, Server: rows[i].Server, Tool: rows[i].Tool,
			Exposed: rows[i].Exposed, SchemaHash: rows[i].SchemaHash, Policy: rows[i].Policy,
		})
	}
	return out
}

func parseSkillBindings(raw string) []*agentv1.SkillBinding {
	if raw == "" {
		return nil
	}
	var rows []draftSkillBinding
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]*agentv1.SkillBinding, 0, len(rows))
	for i := range rows {
		out = append(out, &agentv1.SkillBinding{
			SkillId: rows[i].SkillID, Name: rows[i].Name,
			Version: rows[i].Version, Checksum: rows[i].Checksum,
		})
	}
	return out
}

func agentDraftProto(row *domain.Agent) *agentv1.AgentDraft {
	if row == nil {
		return nil
	}
	return &agentv1.AgentDraft{
		Instruction: row.DraftInstruction, ModelId: row.DraftModelID,
		Tools:             parseStringJSON(row.DraftTools),
		CapabilityPresets: parseStringJSON(row.DraftCapabilityPresets),
		ToolPolicies:      parsePolicyJSON(row.DraftToolPolicies),
		MaxTokens:         int32(row.DraftMaxTokens),
		McpBindings:       parseMCPBindings(row.DraftMCPBindings),
		Skills:            parseSkillBindings(row.DraftSkills),
		Revision:          row.DraftRevision,
	}
}

func agentVersionProto(v *domain.AgentVersion) *agentv1.AgentVersion {
	if v == nil {
		return nil
	}
	out := &agentv1.AgentVersion{
		Id: v.ID, AgentId: v.AgentID, Version: v.Version, Instruction: v.Instruction,
		ModelId: v.ModelID, ModelKey: v.ModelKey,
		Tools: parseStringJSON(v.Tools), CapabilityPresets: parseStringJSON(v.CapabilityPresets),
		ToolPolicies: parsePolicyJSON(v.ToolPolicies), MaxTokens: int32(v.MaxTokens),
		McpBindings: parseMCPBindings(v.MCPBindings), Skills: parseSkillBindings(v.Skills),
		PublishNote: v.PublishNote, PublishedBy: v.PublishedBy, SourceRevision: v.SourceRevision,
	}
	if v.PublishedAt != nil {
		out.PublishedAt = timestamppb.New(*v.PublishedAt)
	}
	if !v.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(v.CreatedAt)
	}
	return out
}

func agentVersionSummaryProto(v *domain.AgentVersion) *agentv1.AgentVersionSummary {
	out := &agentv1.AgentVersionSummary{
		Id: v.ID, Version: v.Version, ModelId: v.ModelID, ModelKey: v.ModelKey,
		PublishNote: v.PublishNote, PublishedBy: v.PublishedBy, SourceRevision: v.SourceRevision,
	}
	if v.PublishedAt != nil {
		out.PublishedAt = timestamppb.New(*v.PublishedAt)
	}
	if !v.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(v.CreatedAt)
	}
	return out
}

func agentProto(row *domain.Agent, manager, owner bool) *agentv1.Agent {
	if row == nil {
		return nil
	}
	out := &agentv1.Agent{
		Id: row.ID, BotUserId: row.BotUserID, OwnerUserId: row.OwnerUserID,
		Name: row.Name, Description: row.Description, Status: row.Status,
		CurrentVersion: row.CurrentVersion, CanManage: manager, CanOwn: owner,
	}
	if manager {
		out.Draft = agentDraftProto(row)
	}
	return out
}

func agentInputFromProto(req *agentv1.AgentInput) admin.Input {
	input := admin.Input{
		Name: req.GetName(), Description: req.GetDescription(), Instruction: req.GetInstruction(),
		ModelID: req.GetModelId(), Tools: req.GetTools(), CapabilityPresets: req.GetCapabilityPresets(),
		ToolPolicies: req.GetToolPolicies(), MaxTokens: int(req.GetMaxTokens()),
		Revision: req.GetRevision(),
	}
	for _, binding := range req.GetMcpBindings() {
		input.MCPBindings = append(input.MCPBindings, admin.MCPBindingInput{
			ServerID: binding.GetServerId(), Tool: binding.GetTool(), Policy: binding.GetPolicy(),
		})
	}
	for _, skill := range req.GetSkills() {
		input.Skills = append(input.Skills, admin.SkillBindingInput{
			SkillID: skill.GetSkillId(), Version: skill.GetVersion(),
		})
	}
	return input
}

func agentDraftInputFromProto(req *agentv1.UpdateAgentDraftRequest) admin.Input {
	input := admin.Input{
		Name: req.GetName(), Description: req.GetDescription(), Instruction: req.GetInstruction(),
		ModelID: req.GetModelId(), Tools: req.GetTools(), CapabilityPresets: req.GetCapabilityPresets(),
		ToolPolicies: req.GetToolPolicies(), MaxTokens: int(req.GetMaxTokens()),
		Revision: req.GetRevision(),
	}
	for _, binding := range req.GetMcpBindings() {
		input.MCPBindings = append(input.MCPBindings, admin.MCPBindingInput{
			ServerID: binding.GetServerId(), Tool: binding.GetTool(), Policy: binding.GetPolicy(),
		})
	}
	for _, skill := range req.GetSkills() {
		input.Skills = append(input.Skills, admin.SkillBindingInput{
			SkillID: skill.GetSkillId(), Version: skill.GetVersion(),
		})
	}
	return input
}

func (s *AgentHTTP) ListAgents(ctx context.Context, req *agentv1.ListAgentsRequest) (*agentv1.ListAgentsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := httpapi.Offset(req.GetCursor())
	if err != nil {
		return nil, kerrors.BadRequest("INVALID_CURSOR", "invalid cursor")
	}
	rows, total, scanned, err := s.agents.List(ctx, uid, limit, offset)
	if err != nil {
		return nil, agentRPCError(err)
	}
	items := make([]*agentv1.Agent, 0, len(rows))
	for i := range rows {
		owner := rows[i].OwnerUserID == uid
		items = append(items, agentProto(&rows[i], owner, owner))
	}
	hasMore := int64(scanned) < total
	next := ""
	if hasMore {
		next = httpapi.EncodeCursor(scanned)
	}
	return &agentv1.ListAgentsReply{
		Items: items, Total: int32(total), HasMore: hasMore, NextCursor: next,
	}, nil
}

func (s *AgentHTTP) CreateAgent(ctx context.Context, req *agentv1.AgentInput) (*agentv1.Agent, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.agents.Create(ctx, uid, agentInputFromProto(req))
	if err != nil {
		return nil, agentRPCError(err)
	}
	return agentProto(row, true, true), nil
}

func (s *AgentHTTP) GetAgent(ctx context.Context, req *agentv1.GetAgentRequest) (*agentv1.Agent, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	row, published, manager, owner, err := s.agents.Detail(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := agentProto(row, manager, owner)
	if published != nil {
		out.Published = agentVersionProto(published)
		out.DraftChanged = row.DraftRevision > published.SourceRevision
	} else {
		out.DraftChanged = true
	}
	return out, nil
}

func (s *AgentHTTP) UpdateAgentDraft(ctx context.Context, req *agentv1.UpdateAgentDraftRequest) (*agentv1.Agent, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.agents.UpdateDraft(ctx, uid, req.GetId(), agentDraftInputFromProto(req))
	if err != nil {
		return nil, agentRPCError(err)
	}
	return agentProto(row, true, true), nil
}

func (s *AgentHTTP) PublishAgent(ctx context.Context, req *agentv1.PublishAgentRequest) (*agentv1.AgentVersion, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	version, err := s.agents.Publish(ctx, uid, req.GetId(), req.GetRevision(), req.GetNote())
	if err != nil {
		return nil, agentRPCError(err)
	}
	return agentVersionProto(version), nil
}

func (s *AgentHTTP) RestoreAgent(ctx context.Context, req *agentv1.RestoreAgentRequest) (*agentv1.Agent, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.agents.Restore(ctx, uid, req.GetId(), req.GetVersion(), req.GetRevision())
	if err != nil {
		return nil, agentRPCError(err)
	}
	return agentProto(row, true, true), nil
}

func (s *AgentHTTP) SetAgentStatus(ctx context.Context, req *agentv1.SetAgentStatusRequest) (*agentv1.SetAgentStatusReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.agents.SetStatus(ctx, uid, req.GetId(), req.GetStatus()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.SetAgentStatusReply{Status: req.GetStatus()}, nil
}

func (s *AgentHTTP) DeleteAgent(ctx context.Context, req *agentv1.GetAgentRequest) (*agentv1.DeleteAgentReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.agents.Delete(ctx, uid, req.GetId()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.DeleteAgentReply{Status: "deleted"}, nil
}

func (s *AgentHTTP) ListAgentVersions(ctx context.Context, req *agentv1.GetAgentRequest) (*agentv1.ListAgentVersionsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.agents.Versions(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := make([]*agentv1.AgentVersionSummary, 0, len(rows))
	for i := range rows {
		out = append(out, agentVersionSummaryProto(&rows[i]))
	}
	return &agentv1.ListAgentVersionsReply{Versions: out}, nil
}

func (s *AgentHTTP) GetAgentVersion(ctx context.Context, req *agentv1.GetAgentVersionRequest) (*agentv1.AgentVersion, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	row, err := s.agents.Version(ctx, uid, req.GetId(), req.GetVersion())
	if err != nil {
		return nil, agentRPCError(err)
	}
	return agentVersionProto(row), nil
}

func (s *AgentHTTP) ListAgentShares(ctx context.Context, req *agentv1.GetAgentRequest) (*agentv1.ListAgentSharesReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.agents.Shares(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := make([]*agentv1.AgentShare, 0, len(rows))
	for i := range rows {
		out = append(out, &agentv1.AgentShare{Kind: rows[i].Kind, Target: rows[i].Target, Role: rows[i].Role})
	}
	return &agentv1.ListAgentSharesReply{Shares: out}, nil
}

func (s *AgentHTTP) PutAgentShare(ctx context.Context, req *agentv1.PutAgentShareRequest) (*agentv1.PutAgentShareReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.agents.Share(ctx, uid, req.GetId(), req.GetKind(), req.GetTarget(), req.GetRole(), req.GetEnabled()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.PutAgentShareReply{
		Kind: req.GetKind(), Target: req.GetTarget(), Role: req.GetRole(), Enabled: req.GetEnabled(),
	}, nil
}

func (s *AgentHTTP) ListAgentRuns(ctx context.Context, req *agentv1.ListAgentRunsRequest) (*agentv1.ListAgentRunsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := httpapi.Offset(req.GetCursor())
	if err != nil {
		return nil, kerrors.BadRequest("INVALID_CURSOR", "invalid cursor")
	}
	runs, total, err := s.runs.List(ctx, uid, req.GetId(), limit, offset)
	if err != nil {
		return nil, agentRPCError(err)
	}
	items := make([]*agentv1.AgentRun, 0, len(runs))
	for i := range runs {
		items = append(items, agentRunProto(&runs[i]))
	}
	hasMore := int64(offset+len(runs)) < total
	return &agentv1.ListAgentRunsReply{
		Items: items, Total: int32(total), HasMore: hasMore,
		NextCursor: agentNextCursor(offset, len(runs), total),
	}, nil
}
