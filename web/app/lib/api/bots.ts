import { api } from "@/lib/api";

export interface Bot {
  id: string;
  botUserId: string;
  ownerUserId: string;
  username: string;
  name: string;
  description: string;
  status: "active" | "disabled";
  eventSubscriptions: string[];
  createdAt: string;
  updatedAt: string;
}

export interface BotToken {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  expiresAt?: string;
  revokedAt?: string;
  lastUsedAt?: string;
  createdAt: string;
}

export interface BotWebhook {
  id: string;
  url: string;
  secretPrefix: string;
  events: string[];
  enabled: boolean;
  verifiedAt?: string;
  lastSuccessAt?: string;
  consecutiveFailures: number;
  disabledAt?: string;
  createdAt: string;
}

export interface BotDelivery {
  id: string;
  eventId: string;
  eventType: string;
  topic: string;
  msgId: string;
  status: string;
  attempts: number;
  nextAttemptAt: string;
  lastStatusCode?: number;
  lastError?: string;
  responseSnippet?: string;
  createdAt: string;
  deliveredAt?: string;
}

export interface BotList {
  items: Bot[];
  total: number;
  nextCursor?: string;
  hasMore: boolean;
}

export const botsApi = {
  list: () => api.get<BotList>("/v1/bots"),
  directory: () => api.get<BotList>("/v1/bots/directory"),
  create: (input: {
    name: string;
    description?: string;
    username?: string;
    eventSubscriptions?: string[];
  }) => api.post<Bot>("/v1/bots", input),
  detail: (id: string) => api.get<Bot>(`/v1/bots/${id}`),
  update: (
    id: string,
    input: {
      name: string;
      description?: string;
      status?: string;
      eventSubscriptions?: string[];
    }
  ) => api.patch<Bot>(`/v1/bots/${id}`, input),
  remove: (id: string) => api.del<{ status: string }>(`/v1/bots/${id}`),

  listTokens: (id: string) =>
    api.get<{ tokens: BotToken[] }>(`/v1/bots/${id}/tokens`),
  createToken: (id: string, input: { name?: string; expiresInDays?: number }) =>
    api.post<{ token: BotToken; secret: string }>(
      `/v1/bots/${id}/tokens`,
      input
    ),
  revokeToken: (id: string, tokenId: string) =>
    api.del<{ status: string }>(`/v1/bots/${id}/tokens/${tokenId}`),

  getWebhook: (id: string) =>
    api.get<BotWebhook>(`/v1/bots/${id}/webhook`),
  putWebhook: (
    id: string,
    input: { url: string; events: string[]; enabled: boolean; rotate?: boolean }
  ) =>
    api.put<{ webhook: BotWebhook; secret?: string }>(
      `/v1/bots/${id}/webhook`,
      input
    ),
  deleteWebhook: (id: string) =>
    api.del<{ status: string }>(`/v1/bots/${id}/webhook`),
  rotateSecret: (id: string) =>
    api.post<{ webhook: BotWebhook; secret: string }>(
      `/v1/bots/${id}/webhook/rotate`,
      {}
    ),
  testWebhook: (id: string) =>
    api.post<{ ok: boolean; statusCode: number; snippet?: string; error?: string }>(
      `/v1/bots/${id}/webhook/test`,
      {}
    ),

  deliveries: (id: string, status = "") =>
    api.get<{
      items: BotDelivery[];
      total: number;
    }>(`/v1/bots/${id}/deliveries${status ? `?status=${status}` : ""}`),
  replayDelivery: (id: string, deliveryId: string) =>
    api.post<{ status: string }>(
      `/v1/bots/${id}/deliveries/${deliveryId}/replay`,
      {}
    ),

  usage: (id: string) =>
    api.get<{ days: number; deliveries: Record<string, number> }>(
      `/v1/bots/${id}/usage`
    ),
};
