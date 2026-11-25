package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/audit"
)

func agentAuditFields(a domain.Agent) map[string]any {
	if a.ID == 0 {
		return nil
	}
	return map[string]any{
		"name": a.Name, "status": a.Status, "currentVersion": a.CurrentVersion,
		"draftRevision": a.DraftRevision, "modelId": a.DraftModelID,
		"instructionLength": len(a.DraftInstruction), "instructionSha256": hashText(a.DraftInstruction),
		"descriptionLength": len(a.Description),
		"tools":             jsonStrings(a.DraftTools),
		"capabilityPresets": jsonStrings(a.DraftCapabilityPresets),
		"toolPolicies":      jsonStringMap(a.DraftToolPolicies),
		"mcpBindingCount":   len(jsonRawList(a.DraftMCPBindings)),
		"skillBindingCount": len(jsonRawList(a.DraftSkills)),
	}
}

func versionAuditFields(v domain.AgentVersion) map[string]any {
	return map[string]any{
		"version": v.Version, "modelId": v.ModelID, "modelKey": v.ModelKey,
		"maxTokens": v.MaxTokens, "publishNote": v.PublishNote,
		"instructionLength": len(v.Instruction), "instructionSha256": hashText(v.Instruction),
		"tools":             jsonStrings(v.Tools),
		"capabilityPresets": jsonStrings(v.CapabilityPresets),
		"toolPolicies":      jsonStringMap(v.ToolPolicies),
		"mcpBindingCount":   len(jsonRawList(v.MCPBindings)),
		"skillBindingCount": len(jsonRawList(v.Skills)),
	}
}

func mcpAuditFields(s domain.MCPServer) map[string]any {
	return map[string]any{
		"name": s.Name, "displayName": s.DisplayName, "authType": s.AuthType,
		"url": redactURL(s.URL), "hasCredential": s.CredentialEnc != "",
		"visibility": s.Visibility, "reviewStatus": s.ReviewStatus, "enabled": s.Enabled,
		"healthStatus": s.HealthStatus, "toolsRevision": s.ToolsRevision,
	}
}

func skillAuditFields(s domain.Skill) map[string]any {
	return map[string]any{
		"slug": s.Slug, "displayName": s.DisplayName, "visibility": s.Visibility,
		"reviewStatus": s.ReviewStatus, "latestVersion": s.LatestVersion,
	}
}

func skillVersionAuditFields(v domain.SkillVersion) map[string]any {
	return map[string]any{
		"version": v.Version, "checksum": v.Checksum, "status": v.Status,
		"reviewStatus": v.ReviewStatus, "contentLength": len(v.Content),
	}
}

func skillAuditPtr(s *domain.Skill) map[string]any {
	if s == nil {
		return nil
	}
	return skillAuditFields(*s)
}

func approvalAuditFields(a domain.AgentApproval) map[string]any {
	return map[string]any{
		"runId": a.RunID, "tool": a.Tool, "argsHash": a.ArgsHash,
		"status": a.Status, "approverId": a.ApproverID,
	}
}

func hashText(v string) string {
	if v == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func jsonStrings(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func jsonStringMap(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	var out map[string]string
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func jsonRawList(raw string) []json.RawMessage {
	if raw == "" {
		return nil
	}
	var out []json.RawMessage
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func uintKey(v uint64) string {
	return strconv.FormatUint(v, 10)
}

func mcpAuditPtr(s *domain.MCPServer) map[string]any {
	if s == nil {
		return nil
	}
	return mcpAuditFields(*s)
}

func (s *AgentService) record(ctx context.Context, actor int64, action, resourceType, resourceID string, before, after map[string]any) error {
	if s.Audit == nil {
		return nil
	}
	return s.Audit.Record(ctx, audit.Event{
		ActorUserID: actor, Action: action, ResourceType: resourceType, ResourceID: resourceID,
		Before: before, After: after, CreatedAt: time.Now(),
	})
}

func recordAudit(ctx context.Context, svc *audit.Service, actor int64, action, resourceType, resourceID string, before, after map[string]any) error {
	if svc == nil {
		return nil
	}
	return svc.Record(ctx, audit.Event{
		ActorUserID: actor, Action: action, ResourceType: resourceType, ResourceID: resourceID,
		Before: before, After: after, CreatedAt: time.Now(),
	})
}
