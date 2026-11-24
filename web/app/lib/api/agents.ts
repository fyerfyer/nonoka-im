import { api, AppError, type Page } from "@/lib/api";

export { AppError };
export type { Page };

export type Policy = "auto" | "ask" | "deny";
export type Visibility = "private" | "shared" | "public";
export type ShareRole = "viewer" | "editor";

export interface Model {
  id: number;
  serviceId: number;
  modelKey: string;
  displayName: string;
  contextWindow: number;
  maxOutputTokens: number;
  supportsTools: boolean;
  supportsVision: boolean;
  supportsReasoning: boolean;
  inputPricePerMtok: number;
  outputPricePerMtok: number;
  enabled: boolean;
}

export interface ModelService {
  id: number;
  name: string;
  displayName: string;
  gatewayType: "bifrost" | "openai_compatible";
  baseUrl?: string;
  gatewayKeyId?: string;
  enabled: boolean;
  hasApiKey: boolean;
  models: Model[];
}

export interface CapabilityPreset {
  key: string;
  name: string;
  description: string;
  defaultEnabled: boolean;
  tools: string[];
}

export interface ToolDefinition {
  key: string;
  name: string;
  description: string;
  category: string;
  risk: string;
  defaultPolicy: Policy;
  defaultEnabled: boolean;
  system: boolean;
  hidden: boolean;
  advanced: boolean;
  sideEffect: boolean;
  presets: string[];
  inputSchema?: unknown;
}

export interface MCPTool {
  id: number;
  serverId: number;
  name: string;
  description: string;
  risk: "read_only" | "write";
  schemaHash: string;
  inputSchema?: string;
  annotations?: string;
  enabled: boolean;
}

export interface MCPServer {
  id: number;
  ownerUserId: number;
  name: string;
  displayName: string;
  description: string;
  url: string;
  transport: string;
  authType: "none" | "bearer" | "header" | string;
  hasCredential: boolean;
  visibility: Visibility | string;
  reviewStatus: "pending" | "approved" | "rejected" | string;
  reviewReason?: string;
  healthStatus: "unknown" | "healthy" | "needs_auth" | "error" | string;
  lastCheckedAt?: string | null;
  lastError?: string;
  enabled: boolean;
  toolsRevision: number;
  tools?: MCPTool[];
  createdAt: string;
  updatedAt: string;
}

export interface MCPConnection {
  id: number;
  serverId: number;
  authType: string;
  status: string;
  hasCredential: boolean;
  expiresAt?: string | null;
  lastUsedAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface SkillVersion {
  id: number;
  skillId: number;
  version: number;
  checksum: string;
  status: string;
  reviewStatus: string;
  reviewReason?: string;
  createdBy: number;
  createdAt: string;
  updatedAt: string;
  content?: string;
  reviewAt?: string | null;
}

export interface Skill {
  id: number;
  ownerUserId: number;
  slug: string;
  displayName: string;
  description: string;
  visibility: Visibility | string;
  reviewStatus: string;
  latestVersion: number;
  versions?: SkillVersion[];
  createdAt: string;
  updatedAt: string;
}

export interface MCPBinding {
  serverId: number;
  server?: string;
  tool: string;
  exposed?: string;
  schemaHash?: string;
  policy?: Policy;
}

export interface SkillBinding {
  skillId: number;
  name?: string;
  version: number;
  checksum?: string;
}

export interface AgentDraft {
  instruction: string;
  modelServiceId: number;
  modelId: number;
  tools: string[];
  capabilityPresets: string[];
  toolPolicies: Record<string, Policy>;
  maxTokens: number;
  mcpBindings: MCPBinding[];
  skills: SkillBinding[];
  revision: number;
}

export interface AgentVersion {
  id: number;
  agentId: number;
  version: number;
  instruction: string;
  modelServiceId: number;
  modelId: number;
  modelKey: string;
  tools: string[];
  capabilityPresets: string[];
  toolPolicies: Record<string, Policy>;
  maxTokens: number;
  mcpBindings: MCPBinding[];
  skills: SkillBinding[];
  publishNote: string;
  publishedBy: number;
  publishedAt?: string | null;
  sourceRevision: number;
  createdAt: string;
}

export interface AgentVersionSummary {
  id: number;
  version: number;
  modelServiceId: number;
  modelId: number;
  modelKey: string;
  publishNote: string;
  publishedBy: number;
  publishedAt?: string | null;
  sourceRevision: number;
  createdAt: string;
}

export interface ManagedAgent {
  id: number;
  botUserId: number;
  ownerUserId: number;
  name: string;
  description: string;
  status: "draft" | "active" | "disabled";
  currentVersion: number;
  canManage: boolean;
  canOwn: boolean;
  draftChanged: boolean;
  draft?: AgentDraft;
  published?: AgentVersion;
}

export interface AgentShare {
  kind: "user" | "group";
  target: string;
  role: ShareRole;
}

export interface AgentRunStep {
  id: number;
  seq: number;
  type: string;
  tool?: string;
  toolCallId?: string;
  summary?: string;
  status?: string;
  createdAt: string;
}

export interface ToolInvocation {
  id: number;
  toolCallId: string;
  tool: string;
  status: string;
  resultSummary?: string;
  errorCode?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AgentRun {
  id: number;
  eventId?: string;
  agentId: number;
  agentVersion?: number;
  sessionId?: number;
  actorUserId: number;
  topic: string;
  msgId?: number;
  topicSeq?: number;
  status: string;
  errorCode?: string;
  errorClass?: string;
  attempts?: number;
  replyMsgId?: number;
  kind: "im" | "test";
  inputText?: string;
  outputText?: string;
  steps?: AgentRunStep[];
  toolInvocations?: ToolInvocation[];
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  createdAt: string;
  startedAt?: string | null;
  finishedAt?: string | null;
}

export interface AgentApproval {
  id: number;
  runId: number;
  toolCallId: string;
  tool: string;
  argsSummary: string;
  argsHash: string;
  approverId: number;
  status: string;
  expiresAt: string;
  decidedAt?: string | null;
  decidedBy?: number;
  consumedAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface AgentInput {
  name: string;
  description: string;
  instruction: string;
  modelServiceId: number;
  modelId: number;
  tools: string[];
  capabilityPresets: string[];
  toolPolicies: Record<string, Policy>;
  maxTokens?: number;
  mcpBindings?: MCPBinding[];
  skills?: SkillBinding[];
}

export interface ModelServiceInput {
  name: string;
  displayName?: string;
  gatewayType?: "bifrost" | "openai_compatible";
  baseUrl?: string;
  apiKey?: string;
  updateApiKey?: boolean;
  enabled?: boolean;
  allowAll?: boolean;
  providerModels?: Record<string, string[]>;
}

export interface ModelInput {
  modelKey: string;
  displayName?: string;
  contextWindow?: number;
  maxOutputTokens?: number;
  supportsTools?: boolean;
  supportsVision?: boolean;
  supportsReasoning?: boolean;
  inputPricePerMtok?: number;
  outputPricePerMtok?: number;
  enabled?: boolean;
}

export interface Catalog {
  services: ModelService[];
  canManageModels: boolean;
  presets: CapabilityPreset[];
  tools: ToolDefinition[];
  mcps: MCPServer[];
  skills: Skill[];
}

export interface RunEvent {
  runId: number;
  seq: number;
  type:
    | "started"
    | "delta"
    | "tool_started"
    | "tool_finished"
    | "approval_required"
    | "approval_resolved"
    | "completed"
    | "failed"
    | "cancelled"
    | "heartbeat";
  topic: string;
  agentId: number;
  botUserId: number;
  actorUserId: number;
  triggerMsgId?: number;
  test?: boolean;
  delta?: string;
  tool?: string;
  toolCallId?: string;
  summary?: string;
  status?: string;
  approvalId?: number;
  replyMsgId?: number;
  clientMessageId?: string;
  errorCode?: string;
  errorClass?: string;
  retryable?: boolean;
  inputTokens?: number;
  outputTokens?: number;
  timestampMs?: number;
}

export const agentApi = {
  list: (limit = 50, cursor = "") =>
    api.get<Page<ManagedAgent>>(
      `/v1/agents?limit=${limit}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`
    ),
  detail: (id: number) => api.get<ManagedAgent>(`/v1/agents/${id}`),
  create: (input: AgentInput) => api.post<ManagedAgent>("/v1/agents", input),
  saveDraft: (id: number, input: AgentInput & { revision: number }) =>
    api.put<ManagedAgent>(`/v1/agents/${id}/draft`, input),
  publish: (id: number, revision: number, note = "") =>
    api.post<AgentVersion>(`/v1/agents/${id}/publish`, { revision, note }),
  restore: (id: number, version: number, revision: number) =>
    api.post<ManagedAgent>(`/v1/agents/${id}/restore`, { version, revision }),
  versions: (id: number) =>
    api.get<{ versions: AgentVersionSummary[] }>(`/v1/agents/${id}/versions`),
  version: (id: number, version: number) =>
    api.get<AgentVersion>(`/v1/agents/${id}/versions/${version}`),
  setStatus: (id: number, status: "active" | "disabled") =>
    api.post<{ status: string }>(`/v1/agents/${id}/status`, { status }),
  delete: (id: number) => api.del<{ status: string }>(`/v1/agents/${id}`),
  test: (id: number, message: string) =>
    api.post<AgentRun>(`/v1/agents/${id}/test`, { message }),

  shares: (id: number) =>
    api.get<{ shares: AgentShare[] }>(`/v1/agents/${id}/shares`),
  share: (
    id: number,
    kind: AgentShare["kind"],
    target: string,
    role: ShareRole,
    enabled: boolean
  ) => api.put<unknown>(`/v1/agents/${id}/shares`, { kind, target, role, enabled }),

  runs: (id: number, limit = 50, cursor = "") =>
    api.get<Page<AgentRun>>(
      `/v1/agents/${id}/runs?limit=${limit}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`
    ),
  runDetail: (id: number) => api.get<AgentRun>(`/v1/agent-runs/${id}`),
  runEvents: (id: number, afterSeq = 0) =>
    api.get<{ events: RunEvent[]; nextSeq: number }>(
      `/v1/agent-runs/${id}/events?afterSeq=${afterSeq}`
    ),
  cancelRun: (id: number) => api.post<unknown>(`/v1/agent-runs/${id}/cancel`, {}),
  runAction: (id: number, action: "retry" | "skip") =>
    api.post<unknown>(`/v1/agent-runs/${id}/${action}`, {}),

  approvals: (status = "pending") =>
    api.get<Page<AgentApproval>>(`/v1/agent-approvals?status=${status}`),
  approvalDetail: (id: number) =>
    api.get<AgentApproval>(`/v1/agent-approvals/${id}`),
  approveApproval: (id: number) =>
    api.post<{ status: string }>(`/v1/agent-approvals/${id}/approve`, {}),
  rejectApproval: (id: number) =>
    api.post<{ status: string }>(`/v1/agent-approvals/${id}/reject`, {}),

  agentTools: () =>
    api.get<{ presets: CapabilityPreset[]; tools: ToolDefinition[] }>("/v1/agent-tools"),

  listModelServices: () =>
    api.get<{ services: ModelService[]; canManage: boolean }>("/v1/model-services"),
  createModelService: (input: ModelServiceInput) =>
    api.post<ModelService>("/v1/model-services", input),
  updateModelService: (id: number, input: ModelServiceInput) =>
    api.patch<ModelService>(`/v1/model-services/${id}`, input),
  deleteModelService: (id: number) =>
    api.del<{ status: string }>(`/v1/model-services/${id}`),
  syncModelService: (id: number) =>
    api.post<{ models: Model[] }>(`/v1/model-services/${id}/sync`, {}),
  createModel: (serviceId: number, input: ModelInput) =>
    api.post<Model>(`/v1/model-services/${serviceId}/models`, input),
  updateModel: (id: number, input: ModelInput) =>
    api.patch<Model>(`/v1/model-models/${id}`, input),
  deleteModel: (id: number) =>
    api.del<{ status: string }>(`/v1/model-models/${id}`),
  listGatewayModels: () =>
    api.get<{ models: { name: string; provider: string }[] }>(
      "/v1/model-gateway/models"
    ),

  listMCPServers: () => api.get<{ servers: MCPServer[] }>("/v1/mcp/servers"),
  getMCPServer: (id: number) => api.get<MCPServer>(`/v1/mcp/servers/${id}`),
  registerMCPServer: (input: {
    name: string;
    displayName?: string;
    description?: string;
    url: string;
    transport?: string;
    authType?: "none" | "bearer" | "header";
    headerName?: string;
    credential?: string;
    visibility?: Visibility;
    enabled?: boolean;
  }) => api.post<MCPServer>("/v1/mcp/servers", input),
  updateMCPServer: (
    id: number,
    input: {
      name: string;
      displayName?: string;
      description?: string;
      url: string;
      transport?: string;
      authType?: "none" | "bearer" | "header";
      headerName?: string;
      credential?: string;
      visibility?: Visibility;
      enabled?: boolean;
    }
  ) => api.patch<MCPServer>(`/v1/mcp/servers/${id}`, input),
  deleteMCPServer: (id: number) =>
    api.del<{ status: string }>(`/v1/mcp/servers/${id}`),
  discoverMCPServer: (id: number) =>
    api.post<{ tools: MCPTool[] }>(`/v1/mcp/servers/${id}/discover`, {}),
  testMCPServer: (id: number) =>
    api.post<{ status: string }>(`/v1/mcp/servers/${id}/test`, {}),
  setMCPServerStatus: (id: number, enabled: boolean) =>
    api.post<{ id: number; enabled: boolean }>(`/v1/mcp/servers/${id}/status`, {
      enabled,
    }),
  reviewMCPServer: (id: number, status: "approved" | "rejected", reason = "") =>
    api.post<unknown>(`/v1/mcp/servers/${id}/review`, { status, reason }),
  listMCPTools: (id: number) =>
    api.get<{ tools: MCPTool[] }>(`/v1/mcp/servers/${id}/tools`),
  listMCPConnections: () =>
    api.get<{ connections: MCPConnection[] }>("/v1/mcp/connections"),
  connectMCPServer: (
    id: number,
    input: { authType?: string; headerName?: string; credential: string }
  ) => api.put<MCPConnection>(`/v1/mcp/connections/${id}`, input),
  disconnectMCPServer: (id: number) =>
    api.del<{ status: string }>(`/v1/mcp/connections/${id}`),

  listSkills: () => api.get<{ skills: Skill[] }>("/v1/skills"),
  getSkill: (id: number) => api.get<Skill>(`/v1/skills/${id}`),
  createSkill: (input: {
    slug: string;
    displayName?: string;
    description?: string;
    visibility?: Visibility;
    content: string;
  }) => api.post<Skill>("/v1/skills", input),
  addSkillVersion: (id: number, content: string) =>
    api.post<SkillVersion>(`/v1/skills/${id}/versions`, { content }),
  updateSkill: (
    id: number,
    input: { displayName?: string; description?: string; visibility?: string }
  ) => api.patch<Skill>(`/v1/skills/${id}`, input),
  deleteSkill: (id: number) => api.del<{ status: string }>(`/v1/skills/${id}`),
  reviewSkill: (id: number, approved: boolean) =>
    api.post<unknown>(`/v1/skills/${id}/review`, { approved }),
  reviewSkillVersion: (
    id: number,
    version: number,
    approved: boolean,
    reason = ""
  ) =>
    api.post<unknown>(`/v1/skills/${id}/versions/${version}/review`, {
      approved,
      reason,
    }),
};
