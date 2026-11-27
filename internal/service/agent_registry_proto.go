package service

import (
	"context"
	"encoding/json"

	agentv1 "nonoka-im/api/agent/v1"
	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/tools/catalog"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func mcpServerProto(server *domain.MCPServer, tools []domain.MCPTool) *agentv1.MCPServer {
	if server == nil {
		return nil
	}
	out := &agentv1.MCPServer{
		Id: server.ID, OwnerUserId: server.OwnerUserID, Name: server.Name,
		DisplayName: server.DisplayName, Description: server.Description,
		Url: server.URL, Transport: server.Transport, AuthType: server.AuthType,
		HasCredential: server.CredentialEnc != "", Visibility: server.Visibility,
		ReviewStatus: server.ReviewStatus, ReviewReason: server.ReviewReason,
		HealthStatus: server.HealthStatus, LastError: server.LastError,
		Enabled: server.Enabled, ToolsRevision: server.ToolsRevision,
	}
	if server.LastCheckedAt != nil {
		out.LastCheckedAt = timestamppb.New(*server.LastCheckedAt)
	}
	if !server.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(server.CreatedAt)
	}
	if !server.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(server.UpdatedAt)
	}
	for i := range tools {
		out.Tools = append(out.Tools, mcpToolProto(&tools[i]))
	}
	return out
}

func mcpToolProto(tool *domain.MCPTool) *agentv1.MCPTool {
	if tool == nil {
		return nil
	}
	return &agentv1.MCPTool{
		Id: tool.ID, ServerId: tool.ServerID, Name: tool.Name, Description: tool.Description,
		Risk: tool.Risk, SchemaHash: tool.SchemaHash, InputSchema: tool.InputSchema,
		Annotations: tool.Annotations, Enabled: tool.Enabled,
	}
}

func mcpToolsProto(rows []domain.MCPTool) []*agentv1.MCPTool {
	out := make([]*agentv1.MCPTool, 0, len(rows))
	for i := range rows {
		out = append(out, mcpToolProto(&rows[i]))
	}
	return out
}

func mcpConnectionProto(conn *domain.MCPConnection) *agentv1.MCPConnection {
	if conn == nil {
		return nil
	}
	out := &agentv1.MCPConnection{
		Id: conn.ID, ServerId: conn.ServerID, AuthType: conn.AuthType,
		Status: conn.Status, HasCredential: conn.CredentialEnc != "",
	}
	if conn.ExpiresAt != nil {
		out.ExpiresAt = timestamppb.New(*conn.ExpiresAt)
	}
	if conn.LastUsedAt != nil {
		out.LastUsedAt = timestamppb.New(*conn.LastUsedAt)
	}
	if !conn.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(conn.CreatedAt)
	}
	if !conn.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(conn.UpdatedAt)
	}
	return out
}

func (s *AgentHTTP) ListMCPServers(ctx context.Context, _ *agentv1.ListMCPServersRequest) (*agentv1.ListMCPServersReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.mcp.List(ctx, uid)
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := make([]*agentv1.MCPServer, 0, len(rows))
	for i := range rows {
		tools, _ := s.mcp.ListTools(ctx, uid, rows[i].ID)
		out = append(out, mcpServerProto(&rows[i], tools))
	}
	return &agentv1.ListMCPServersReply{Servers: out}, nil
}

func (s *AgentHTTP) RegisterMCPServer(ctx context.Context, req *agentv1.RegisterMCPServerRequest) (*agentv1.MCPServer, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = req.GetEnabled()
	}
	server, err := s.mcp.Register(ctx, uid, admin.MCPInput{
		Name: req.GetName(), DisplayName: req.GetDisplayName(), Description: req.GetDescription(),
		URL: req.GetUrl(), Transport: req.GetTransport(), AuthType: req.GetAuthType(),
		HeaderName: req.GetHeaderName(), Credential: req.GetCredential(),
		Visibility: req.GetVisibility(), Enabled: enabled,
	})
	if err != nil {
		return nil, agentRPCError(err)
	}
	tools, _ := s.mcp.ListTools(ctx, uid, server.ID)
	return mcpServerProto(server, tools), nil
}

func (s *AgentHTTP) GetMCPServer(ctx context.Context, req *agentv1.GetMCPServerRequest) (*agentv1.MCPServer, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	server, err := s.mcp.Get(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	tools, err := s.mcp.ListTools(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	return mcpServerProto(server, tools), nil
}

func (s *AgentHTTP) UpdateMCPServer(ctx context.Context, req *agentv1.UpdateMCPServerRequest) (*agentv1.MCPServer, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = req.GetEnabled()
	}
	server, err := s.mcp.Update(ctx, uid, req.GetId(), admin.MCPInput{
		Name: req.GetName(), DisplayName: req.GetDisplayName(), Description: req.GetDescription(),
		URL: req.GetUrl(), Transport: req.GetTransport(), AuthType: req.GetAuthType(),
		HeaderName: req.GetHeaderName(), Credential: req.GetCredential(),
		Visibility: req.GetVisibility(), Enabled: enabled,
	})
	if err != nil {
		return nil, agentRPCError(err)
	}
	tools, _ := s.mcp.ListTools(ctx, uid, req.GetId())
	return mcpServerProto(server, tools), nil
}

func (s *AgentHTTP) DeleteMCPServer(ctx context.Context, req *agentv1.GetMCPServerRequest) (*agentv1.StatusReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.mcp.Delete(ctx, uid, req.GetId()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.StatusReply{Status: "deleted"}, nil
}

func (s *AgentHTTP) DiscoverMCPServer(ctx context.Context, req *agentv1.GetMCPServerRequest) (*agentv1.MCPToolsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	tools, err := s.mcp.Discover(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.MCPToolsReply{Tools: mcpToolsProto(tools)}, nil
}

func (s *AgentHTTP) TestMCPServer(ctx context.Context, req *agentv1.GetMCPServerRequest) (*agentv1.StatusReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.mcp.Test(ctx, uid, req.GetId()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.StatusReply{Status: "healthy"}, nil
}

func (s *AgentHTTP) SetMCPServerStatus(ctx context.Context, req *agentv1.SetMCPServerStatusRequest) (*agentv1.SetMCPServerStatusReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Enabled == nil {
		return nil, kerrors.BadRequest("INVALID_ARGUMENT", "enabled is required")
	}
	if err := s.mcp.SetStatus(ctx, uid, req.GetId(), req.GetEnabled()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.SetMCPServerStatusReply{Id: req.GetId(), Enabled: req.GetEnabled()}, nil
}

func (s *AgentHTTP) ReviewMCPServer(ctx context.Context, req *agentv1.ReviewMCPServerRequest) (*agentv1.ReviewMCPServerReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.mcp.Review(ctx, uid, req.GetId(), req.GetStatus(), req.GetReason()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.ReviewMCPServerReply{Id: req.GetId(), ReviewStatus: req.GetStatus()}, nil
}

func (s *AgentHTTP) ListMCPTools(ctx context.Context, req *agentv1.GetMCPServerRequest) (*agentv1.MCPToolsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	tools, err := s.mcp.ListTools(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.MCPToolsReply{Tools: mcpToolsProto(tools)}, nil
}

func (s *AgentHTTP) ListMCPConnections(ctx context.Context, _ *agentv1.ListMCPConnectionsRequest) (*agentv1.ListMCPConnectionsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.mcp.ListConnections(ctx, uid)
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := make([]*agentv1.MCPConnection, 0, len(rows))
	for i := range rows {
		out = append(out, mcpConnectionProto(&rows[i]))
	}
	return &agentv1.ListMCPConnectionsReply{Connections: out}, nil
}

func (s *AgentHTTP) ConnectMCPServer(ctx context.Context, req *agentv1.ConnectMCPServerRequest) (*agentv1.MCPConnection, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := s.mcp.Connect(ctx, uid, req.GetId(), admin.MCPConnectionInput{
		AuthType: req.GetAuthType(), HeaderName: req.GetHeaderName(), Credential: req.GetCredential(),
	})
	if err != nil {
		return nil, agentRPCError(err)
	}
	return mcpConnectionProto(conn), nil
}

func (s *AgentHTTP) DisconnectMCPServer(ctx context.Context, req *agentv1.DisconnectMCPServerRequest) (*agentv1.StatusReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.mcp.Disconnect(ctx, uid, req.GetId()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.StatusReply{Status: "disconnected"}, nil
}

func skillProto(skill *domain.Skill, versions []*agentv1.SkillVersion) *agentv1.Skill {
	if skill == nil {
		return nil
	}
	out := &agentv1.Skill{
		Id: skill.ID, OwnerUserId: skill.OwnerUserID, Slug: skill.Slug,
		DisplayName: skill.DisplayName, Description: skill.Description,
		Visibility: skill.Visibility, ReviewStatus: skill.ReviewStatus,
		LatestVersion: skill.LatestVersion, Versions: versions,
	}
	if !skill.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(skill.CreatedAt)
	}
	if !skill.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(skill.UpdatedAt)
	}
	return out
}

func skillVersionProto(version *domain.SkillVersion, includeContent bool) *agentv1.SkillVersion {
	if version == nil {
		return nil
	}
	out := &agentv1.SkillVersion{
		Id: version.ID, SkillId: version.SkillID, Version: version.Version,
		Checksum: version.Checksum, Status: version.Status,
		ReviewStatus: version.ReviewStatus, ReviewReason: version.ReviewReason,
		CreatedBy: version.CreatedBy,
	}
	if includeContent {
		out.Content = version.Content
	}
	if version.ReviewAt != nil {
		out.ReviewAt = timestamppb.New(*version.ReviewAt)
	}
	if !version.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(version.CreatedAt)
	}
	if !version.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(version.UpdatedAt)
	}
	return out
}

func (s *AgentHTTP) ListSkills(ctx context.Context, _ *agentv1.ListSkillsRequest) (*agentv1.ListSkillsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.skills.List(ctx, uid)
	if err != nil {
		return nil, agentRPCError(err)
	}
	out := make([]*agentv1.Skill, 0, len(rows))
	for i := range rows {
		out = append(out, skillProto(&rows[i], nil))
	}
	return &agentv1.ListSkillsReply{Skills: out}, nil
}

func (s *AgentHTTP) GetSkill(ctx context.Context, req *agentv1.GetSkillRequest) (*agentv1.Skill, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	skill, versions, err := s.skills.Get(ctx, uid, req.GetId())
	if err != nil {
		return nil, agentRPCError(err)
	}
	canManage := s.skills.CanManage(ctx, uid, skill)
	out := make([]*agentv1.SkillVersion, 0, len(versions))
	for i := range versions {
		out = append(out, skillVersionProto(&versions[i], canManage))
	}
	return skillProto(skill, out), nil
}

func (s *AgentHTTP) CreateSkill(ctx context.Context, req *agentv1.CreateSkillRequest) (*agentv1.Skill, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	skill, version, err := s.skills.Create(ctx, uid, admin.SkillInput{
		Slug: req.GetSlug(), DisplayName: req.GetDisplayName(), Description: req.GetDescription(),
		Visibility: req.GetVisibility(), Content: req.GetContent(),
	})
	if err != nil {
		return nil, agentRPCError(err)
	}
	return skillProto(skill, []*agentv1.SkillVersion{skillVersionProto(version, true)}), nil
}

func (s *AgentHTTP) UpdateSkill(ctx context.Context, req *agentv1.UpdateSkillRequest) (*agentv1.Skill, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	skill, err := s.skills.UpdateMeta(ctx, uid, req.GetId(), req.GetDisplayName(), req.GetDescription(), req.GetVisibility())
	if err != nil {
		return nil, agentRPCError(err)
	}
	return skillProto(skill, nil), nil
}

func (s *AgentHTTP) DeleteSkill(ctx context.Context, req *agentv1.GetSkillRequest) (*agentv1.StatusReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.skills.Delete(ctx, uid, req.GetId()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.StatusReply{Status: "deleted"}, nil
}

func (s *AgentHTTP) AddSkillVersion(ctx context.Context, req *agentv1.AddSkillVersionRequest) (*agentv1.SkillVersion, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	version, err := s.skills.AddVersion(ctx, uid, req.GetId(), req.GetContent())
	if err != nil {
		return nil, agentRPCError(err)
	}
	return skillVersionProto(version, true), nil
}

func (s *AgentHTTP) ReviewSkill(ctx context.Context, req *agentv1.ReviewSkillRequest) (*agentv1.SkillReviewReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.skills.ReviewSkill(ctx, uid, req.GetId(), req.GetApproved()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.SkillReviewReply{Id: req.GetId(), Approved: req.GetApproved()}, nil
}

func (s *AgentHTTP) ReviewSkillVersion(ctx context.Context, req *agentv1.ReviewSkillVersionRequest) (*agentv1.SkillReviewReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.skills.ReviewVersion(ctx, uid, req.GetId(), req.GetVersion(), req.GetApproved(), req.GetReason()); err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.SkillReviewReply{Id: req.GetId(), Version: req.GetVersion(), Approved: req.GetApproved()}, nil
}

func toolInputSchema(raw json.RawMessage) *structpb.Struct {
	if len(raw) == 0 {
		return nil
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil
	}
	out, err := structpb.NewStruct(schema)
	if err != nil {
		return nil
	}
	return out
}

func (s *AgentHTTP) ListAgentTools(_ context.Context, _ *agentv1.ListAgentToolsRequest) (*agentv1.ListAgentToolsReply, error) {
	defs := catalog.BuiltinDefinitions()
	out := &agentv1.ListAgentToolsReply{Tools: make([]*agentv1.ToolDefinition, 0, len(defs))}
	for _, def := range defs {
		out.Tools = append(out.Tools, &agentv1.ToolDefinition{
			Key: def.Key, Name: def.Name, Description: def.Description,
			Category: def.Category, Risk: def.Risk, DefaultPolicy: def.DefaultPolicy,
			DefaultEnabled: def.DefaultEnabled, System: def.System, Hidden: def.Hidden,
			Advanced: def.Advanced, SideEffect: def.SideEffect, Presets: def.Presets,
			InputSchema: toolInputSchema(def.InputSchema),
		})
	}
	for _, preset := range catalog.Presets() {
		out.Presets = append(out.Presets, &agentv1.CapabilityPreset{
			Key: preset.Key, Name: preset.Name, Description: preset.Description,
			DefaultEnabled: preset.DefaultEnabled, Tools: preset.Tools,
		})
	}
	return out, nil
}

func modelProto(m domain.Model) *agentv1.Model {
	return &agentv1.Model{
		Id: m.ID, ModelKey: m.ModelKey, Provider: m.Provider, GatewayModelKey: m.GatewayModelKey,
		DisplayName: m.DisplayName, Icon: m.Icon, ContextWindow: int32(m.ContextWindow),
		MaxOutputTokens: int32(m.MaxOutputTokens), SupportsTools: m.SupportsTools,
		SupportsVision: m.SupportsVision, SupportsReasoning: m.SupportsReasoning,
		InputPricePerMtok: m.InputPricePerMTok, OutputPricePerMtok: m.OutputPricePerMTok,
		InputCacheReadPricePerMtok: m.InputCacheReadPricePerMTok,
		Enabled:                    m.Enabled, Curated: m.Curated, Recommended: m.Recommended,
		SortOrder: int32(m.SortOrder),
	}
}

func modelProtos(rows []domain.Model) []*agentv1.Model {
	out := make([]*agentv1.Model, 0, len(rows))
	for i := range rows {
		out = append(out, modelProto(rows[i]))
	}
	return out
}

func (s *AgentHTTP) ListModels(ctx context.Context, _ *agentv1.ListModelsRequest) (*agentv1.ListModelsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	models, canManage, err := s.models.Catalog(ctx, uid)
	if err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.ListModelsReply{Models: modelProtos(models), CanManage: canManage}, nil
}

func (s *AgentHTTP) SyncModels(ctx context.Context, _ *agentv1.SyncModelsRequest) (*agentv1.SyncModelsReply, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.models.SyncCatalog(ctx, uid)
	if err != nil {
		return nil, agentRPCError(err)
	}
	models, _, err := s.models.Catalog(ctx, uid)
	if err != nil {
		return nil, agentRPCError(err)
	}
	return &agentv1.SyncModelsReply{Synced: int32(n), Models: modelProtos(models)}, nil
}

func (s *AgentHTTP) UpdateModel(ctx context.Context, req *agentv1.UpdateModelRequest) (*agentv1.Model, error) {
	uid, err := s.agentRPCUser(ctx)
	if err != nil {
		return nil, err
	}
	model, err := s.models.UpdateCuration(ctx, uid, req.GetId(), domain.ModelCuration{
		DisplayName: req.GetDisplayName(), Icon: req.GetIcon(),
		SupportsTools: req.SupportsTools, SupportsVision: req.SupportsVision,
		SupportsReasoning: req.SupportsReasoning, Enabled: req.Enabled,
		Recommended: req.Recommended, SortOrder: modelSortOrder(req),
	})
	if err != nil {
		return nil, agentRPCError(err)
	}
	return modelProto(*model), nil
}

func modelSortOrder(req *agentv1.UpdateModelRequest) *int {
	if req.SortOrder == nil {
		return nil
	}
	value := int(req.GetSortOrder())
	return &value
}
