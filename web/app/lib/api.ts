import { User, Group, GroupMember } from "@/types";
import { useChatStore } from "@/stores/chatStore";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL || "http://127.0.0.1:8000";

export interface ApiError {
  code: string;
  message: string;
  status: number;
}

export class AppError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number
  ) {
    super(message);
    this.name = "AppError";
  }
}

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("token");
}

function getRefreshToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("refreshToken");
}

// setAuthTokens persists a freshly issued token pair.
export function setAuthTokens(token: string, refreshToken: string): void {
  if (typeof window === "undefined") return;
  localStorage.setItem("token", token);
  if (refreshToken) {
    localStorage.setItem("refreshToken", refreshToken);
  }
}

// DEVICE_ID identifies this client type to the backend; refresh tokens are
// scoped per (user, device) so it must stay consistent between login and
// refresh calls.
export const DEVICE_ID = "web";

function getStoredUser(): { userId: number } | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = localStorage.getItem("user");
    return raw ? (JSON.parse(raw) as { userId: number }) : null;
  } catch {
    return null;
  }
}

// forceReLogin drops local credentials and bounces to the login page. Used
// when the refresh token is missing or rejected (session truly expired).
export function forceReLogin(): void {
  if (typeof window === "undefined") return;
  localStorage.removeItem("token");
  localStorage.removeItem("refreshToken");
  localStorage.removeItem("user");
  // Wipe in-memory conversation state so a stale session's data is never
  // rendered behind the login page.
  useChatStore.getState().reset();
  if (!window.location.pathname.startsWith("/login")) {
    window.location.assign("/login");
  }
}

let refreshInflight: Promise<string> | null = null;

// refreshAuth exchanges the stored refresh token for a new token pair.
// Concurrent callers share one in-flight request.
export function refreshAuth(): Promise<string> {
  if (refreshInflight) return refreshInflight;
  refreshInflight = (async () => {
    const refreshToken = getRefreshToken();
    const user = getStoredUser();
    if (!refreshToken || !user) {
      throw new AppError("NO_REFRESH_TOKEN", "no refresh token", 401);
    }
    const reply = await api.post<LoginReply>("/v1/auth/refresh", {
      userId: user.userId,
      deviceId: DEVICE_ID,
      refreshToken,
    });
    setAuthTokens(reply.token, reply.refreshToken);
    return reply.token;
  })().finally(() => {
    refreshInflight = null;
  });
  return refreshInflight;
}

const AUTH_PUBLIC_PATHS = ["/v1/auth/login", "/v1/auth/register", "/v1/auth/refresh"];

function buildURL(path: string): string {
  const base = API_BASE_URL.replace(/\/$/, "");
  const p = path.startsWith("/") ? path : `/${path}`;
  return `${base}${p}`;
}

async function request<T>(
  path: string,
  options: RequestInit = {},
  allowAuthRetry = true
): Promise<T> {
  const url = buildURL(path);
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "application/json",
    ...(options.headers as Record<string, string>),
  };

  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  let res: Response;
  try {
    res = await fetch(url, {
      ...options,
      headers,
    });
  } catch (error) {
    // A refused connection is a deployment/startup problem, not an invalid
    // username or password. Convert the browser's opaque "Failed to fetch"
    // into an actionable message while preserving a stable error code.
    const detail = error instanceof Error ? error.message : "network error";
    throw new AppError(
      "API_UNREACHABLE",
      `Unable to reach the Nonoka server at ${API_BASE_URL}. Start the backend and try again (${detail}).`,
      0
    );
  }

  let body: unknown;
  const contentType = res.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    body = await res.json().catch(() => ({}));
  } else if (res.status !== 204) {
    body = { message: await res.text() };
  }

  if (!res.ok) {
    // Access token expired: exchange the refresh token once and retry the
    // original request with the new token. Public auth paths are excluded
    // (their 401s are credential errors, not expiry).
    if (
      res.status === 401 &&
      allowAuthRetry &&
      !AUTH_PUBLIC_PATHS.some((p) => path.startsWith(p))
    ) {
      try {
        await refreshAuth();
      } catch {
        forceReLogin();
        throw new AppError("SESSION_EXPIRED", "session expired", 401);
      }
      return request<T>(path, options, false);
    }
    const err = body as { code?: string; message?: string; reason?: string };
    throw new AppError(
      err.code || `HTTP_${res.status}`,
      err.message || err.reason || res.statusText || "request failed",
      res.status
    );
  }

  return body as T;
}

export const api = {
  get: <T>(path: string, options?: RequestInit) =>
    request<T>(path, { ...options, method: "GET" }),
  post: <T>(path: string, body: unknown, options?: RequestInit) =>
    request<T>(path, {
      ...options,
      method: "POST",
      body: JSON.stringify(body),
    }),
  put: <T>(path: string, body: unknown, options?: RequestInit) =>
    request<T>(path, {
      ...options,
      method: "PUT",
      body: JSON.stringify(body),
    }),
  patch: <T>(path: string, body: unknown, options?: RequestInit) =>
    request<T>(path, {
      ...options,
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  del: <T>(path: string, options?: RequestInit) =>
    request<T>(path, { ...options, method: "DELETE" }),
};

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginReply {
  userId: number;
  token: string;
  refreshToken: string;
}

export interface RefreshRequest {
  userId: number;
  deviceId: string;
  refreshToken: string;
}

export interface RegisterRequest {
  username: string;
  password: string;
}

export interface RegisterReply {
  userId: number;
}

export interface SearchUsersReply {
  users: User[];
}

export interface UserPresenceReply {
  userId: number;
  online: boolean;
}

export interface ConversationReply {
  topic: string;
  type: string;
  peerId: number;
  peerUsername: string;
  lastMsgPreview?: string;
  lastMsgAt?: number;
  lastSeq: number;
  lastReadSeq: number;
  unreadCount: number;
}

export interface ListConversationsReply {
  conversations: ConversationReply[];
  hasMore: boolean;
}

export const authApi = {
  login: (req: LoginRequest) =>
    api.post<LoginReply>("/v1/auth/login", { ...req, deviceId: DEVICE_ID }),
  register: (req: RegisterRequest) =>
    api.post<RegisterReply>("/v1/auth/register", req),
  refresh: (req: RefreshRequest) =>
    api.post<LoginReply>("/v1/auth/refresh", req),
  logout: () => api.post<{ success: boolean }>("/v1/auth/logout", {}),
};

export const userApi = {
  searchUsers: (username: string, limit = 20) =>
    api.get<SearchUsersReply>(
      `/v1/users/search?username=${encodeURIComponent(username)}&limit=${limit}`
    ),
  getPresence: (userId: number) =>
    api.get<UserPresenceReply>(`/v1/users/${userId}/presence`),
};

export const conversationApi = {
  listConversations: (limit = 50, offset = 0) =>
    api.get<ListConversationsReply>(
      `/v1/conversations?limit=${limit}&offset=${offset}`
    ),
  markRead: (topic: string) =>
    api.post<unknown>(`/v1/conversations/${encodeURIComponent(topic)}/read`, {}),
};

export const dispatchApi = {
  getGateway: (userId: number) =>
    api.get<{ gatewayUrl: string; gatewayUrls?: string[] }>(
      `/v1/dispatch/gateway?userId=${userId}`
    ),
};

export interface UploadedFile {
  file_id: string;
  url: string;
  name: string;
  size: number;
  mime: string;
}

export const fileApi = {
  upload: async (file: File): Promise<UploadedFile> => {
    const form = new FormData();
    form.append("file", file);
    const res = await fetch(`${API_BASE_URL}/v1/files`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${localStorage.getItem("token") || ""}`,
      },
      body: form,
    });
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as {
        message?: string;
      } | null;
      throw new Error(body?.message || `upload failed (${res.status})`);
    }
    return (await res.json()) as UploadedFile;
  },
};

export const messageApi = {
  // Pull messages. Backward history page: pass endSeq (exclusive upper
  // bound); endSeq=0 with lastSeq=0 returns the latest page. Forward
  // incremental pull: lastSeq > 0, endSeq = 0.
  pullMessages: (topic: string, lastSeq: number, limit = 20, endSeq = 0) =>
    api.get<{
      messages: Array<{
        msgId: number;
        topic: string;
        senderId: number;
        msgType: number;
        content: string;
        timestamp: number;
        topicSeq: number;
        clientMsgId?: string;
        recalled?: boolean;
      }>;
      hasMore: boolean;
      nextSeq: number;
    }>(
      `/v1/message/pull?topic=${encodeURIComponent(
        topic
      )}&lastSeq=${lastSeq}&limit=${limit}&endSeq=${endSeq}`
    ),
};

export const groupApi = {
  createGroup: (name: string, memberIds: number[]) =>
    api.post<{ group_id: string; topic: string; name?: string }>("/v1/groups", {
      name,
      member_ids: memberIds,
    }),
  listMyGroups: () => api.get<{ groups: Group[] }>("/v1/groups"),
  getGroup: (groupId: string) => api.get<Group>(`/v1/groups/${groupId}`),
  addMember: (groupId: string, userId: number) =>
    api.post<unknown>(`/v1/groups/${groupId}/members`, { user_id: userId }),
  removeMember: (groupId: string, userId: number) =>
    api.del<unknown>(`/v1/groups/${groupId}/members/${userId}`),
  listMembers: (groupId: string) =>
    api.get<{ members: GroupMember[] }>(`/v1/groups/${groupId}/members`),
};

export type AgentTool = string;

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
  defaultPolicy: "auto" | "ask" | "deny";
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
  visibility: "private" | "shared" | "public" | string;
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
  visibility: "private" | "shared" | "public" | string;
  reviewStatus: string;
  latestVersion: number;
  versions?: SkillVersion[];
  createdAt: string;
  updatedAt: string;
}

export interface AgentDraft {
  instruction: string;
  modelServiceId: number;
  modelId: number;
  tools: AgentTool[];
  capabilityPresets: string[];
  toolPolicies: Record<string, "auto" | "ask" | "deny">;
  maxTokens?: number;
  mcpBindings?: { serverId: number; tool: string; policy?: "auto" | "ask" | "deny" }[];
  skills?: { skillId: number; version: number }[];
  revision: number;
}

export interface AgentPublishedVersion {
  id?: number;
  agentId?: number;
  version?: number;
  instruction?: string;
  modelServiceId?: number;
  modelId?: number;
  modelKey?: string;
  tools?: string;
  capabilityPresets?: string;
  toolPolicies?: Record<string, string>;
  maxTokens?: number;
  mcpBindings?: { serverId: number; tool: string; exposed: string; policy?: string }[];
  skills?: { skillId: number; version: number; checksum?: string }[];
  createdAt?: string;
  [key: string]: unknown;
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
  draft?: AgentDraft;
  published?: AgentPublishedVersion;
}

export interface AgentShare {
  kind: "user" | "group";
  target: string;
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
  checkpointId?: string;
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

export interface Page<T> {
  items: T[];
  nextCursor?: string;
  hasMore: boolean;
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

export interface AgentInput {
  name: string;
  description: string;
  instruction: string;
  modelServiceId: number;
  modelId: number;
  tools: AgentTool[];
  capabilityPresets: string[];
  toolPolicies: Record<string, "auto" | "ask" | "deny">;
  maxTokens?: number;
  mcpBindings?: { serverId: number; tool: string; policy?: "auto" | "ask" | "deny" }[];
  skills?: { skillId: number; version: number }[];
}

export type ListAgentsReply = Page<ManagedAgent>;

export const agentApi = {
  list: () => api.get<ListAgentsReply>("/v1/agents"),
  detail: (id: number) => api.get<ManagedAgent>(`/v1/agents/${id}`),
  create: (input: AgentInput) => api.post<ManagedAgent>("/v1/agents", input),
  saveDraft: (id: number, input: AgentInput & { revision: number }) =>
    api.put<ManagedAgent>(`/v1/agents/${id}/draft`, input),
  publish: (id: number, revision: number) =>
    api.post<unknown>(`/v1/agents/${id}/publish`, { revision }),
  setStatus: (id: number, status: "active" | "disabled") =>
    api.post<unknown>(`/v1/agents/${id}/status`, { status }),
  shares: (id: number) =>
    api.get<{ shares: AgentShare[] }>(`/v1/agents/${id}/shares`),
  share: (id: number, kind: AgentShare["kind"], target: string, enabled: boolean) =>
    api.put<unknown>(`/v1/agents/${id}/shares`, { kind, target, enabled }),
  runs: (id: number) => api.get<Page<AgentRun>>(`/v1/agents/${id}/runs`),
  runDetail: (id: number) => api.get<AgentRun>(`/v1/agent-runs/${id}`),
  runAction: (id: number, action: "retry" | "skip") =>
    api.post<unknown>(`/v1/agent-runs/${id}/${action}`, {}),
  delete: (id: number) => api.del<{ status: string }>(`/v1/agents/${id}`),
  approveApproval: (id: number) =>
    api.post<{ status: string }>(`/v1/agent-approvals/${id}/approve`, {}),
  rejectApproval: (id: number) =>
    api.post<{ status: string }>(`/v1/agent-approvals/${id}/reject`, {}),
  approvalDetail: (id: number) =>
    api.get<AgentApproval>(`/v1/agent-approvals/${id}`),
  approvals: (status = "pending") =>
    api.get<Page<AgentApproval>>(`/v1/agent-approvals?status=${status}`),

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
  deleteModel: (id: number) => api.del<{ status: string }>(`/v1/model-models/${id}`),
  listGatewayModels: () =>
    api.get<{ models: { name: string; provider: string }[] }>("/v1/model-gateway/models"),

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
    visibility?: "private" | "shared" | "public";
    enabled?: boolean;
  }) => api.post<MCPServer>("/v1/mcp/servers", input),
  updateMCPServer: (id: number, input: {
    name: string;
    displayName?: string;
    description?: string;
    url: string;
    transport?: string;
    authType?: "none" | "bearer" | "header";
    headerName?: string;
    credential?: string;
    visibility?: "private" | "shared" | "public";
    enabled?: boolean;
  }) => api.patch<MCPServer>(`/v1/mcp/servers/${id}`, input),
  deleteMCPServer: (id: number) => api.del<{ status: string }>(`/v1/mcp/servers/${id}`),
  discoverMCPServer: (id: number) =>
    api.post<{ tools: MCPTool[] }>(`/v1/mcp/servers/${id}/discover`, {}),
  testMCPServer: (id: number) =>
    api.post<{ status: string }>(`/v1/mcp/servers/${id}/test`, {}),
  setMCPServerStatus: (id: number, enabled: boolean) =>
    api.post<{ id: number; enabled: boolean }>(`/v1/mcp/servers/${id}/status`, { enabled }),
  reviewMCPServer: (id: number, status: "approved" | "rejected", reason = "") =>
    api.post<unknown>(`/v1/mcp/servers/${id}/review`, { status, reason }),
  listMCPTools: (id: number) =>
    api.get<{ tools: MCPTool[] }>(`/v1/mcp/servers/${id}/tools`),
  listMCPConnections: () =>
    api.get<{ connections: MCPConnection[] }>("/v1/mcp/connections"),
  connectMCPServer: (id: number, input: { authType?: string; headerName?: string; credential: string }) =>
    api.put<MCPConnection>(`/v1/mcp/connections/${id}`, input),
  disconnectMCPServer: (id: number) =>
    api.del<{ status: string }>(`/v1/mcp/connections/${id}`),

  listSkills: () => api.get<{ skills: Skill[] }>("/v1/skills"),
  getSkill: (id: number) => api.get<Skill>(`/v1/skills/${id}`),
  createSkill: (input: {
    slug: string;
    displayName?: string;
    description?: string;
    visibility?: "private" | "shared" | "public";
    content: string;
  }) => api.post<Skill>("/v1/skills", input),
  addSkillVersion: (id: number, content: string) =>
    api.post<SkillVersion>(`/v1/skills/${id}/versions`, { content }),
  updateSkill: (id: number, input: { displayName?: string; description?: string; visibility?: string }) =>
    api.patch<Skill>(`/v1/skills/${id}`, input),
  deleteSkill: (id: number) => api.del<{ status: string }>(`/v1/skills/${id}`),
  reviewSkill: (id: number, approved: boolean) =>
    api.post<unknown>(`/v1/skills/${id}/review`, { approved }),
  reviewSkillVersion: (id: number, version: number, approved: boolean, reason = "") =>
    api.post<unknown>(`/v1/skills/${id}/versions/${version}/review`, { approved, reason }),
};
