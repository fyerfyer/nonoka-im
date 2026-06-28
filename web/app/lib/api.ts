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

function buildURL(path: string): string {
  const base = API_BASE_URL.replace(/\/$/, "");
  const p = path.startsWith("/") ? path : `/${path}`;
  return `${base}${p}`;
}

async function request<T>(
  path: string,
  options: RequestInit = {}
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
  login: (req: LoginRequest) => api.post<LoginReply>("/v1/auth/login", req),
  register: (req: RegisterRequest) =>
    api.post<RegisterReply>("/v1/auth/register", req),
};

export const userApi = {
  searchUsers: (username: string, limit = 20) =>
    api.get<SearchUsersReply>(
      `/v1/users/search?username=${encodeURIComponent(username)}&limit=${limit}`
    ),
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

export const messageApi = {
  pullMessages: (topic: string, lastSeq: number, limit = 20) =>
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
      }>;
      hasMore: boolean;
      nextSeq: number;
    }>(
      `/v1/message/pull?topic=${encodeURIComponent(
        topic
      )}&lastSeq=${lastSeq}&limit=${limit}`
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
