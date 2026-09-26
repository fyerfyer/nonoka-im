import { User, Group, GroupMember } from "@/types";

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

  const res = await fetch(url, {
    ...options,
    headers,
  });

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
