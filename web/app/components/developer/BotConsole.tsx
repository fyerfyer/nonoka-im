"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  Bot as BotIcon,
  Check,
  Clipboard,
  Code2,
  KeyRound,
  Loader2,
  Plus,
  RefreshCw,
  Send,
  Trash2,
  Webhook,
} from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "@/hooks/useAuth";
import {
  botsApi,
  type Bot,
  type BotDelivery,
  type BotToken,
  type BotWebhook,
} from "@/lib/api/bots";
import { AppError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

const EVENT_OPTIONS = [
  { key: "message.mention", label: "Mentions" },
  { key: "message.direct", label: "Direct messages" },
];

type Tab = "overview" | "credentials" | "webhook" | "deliveries";

export function BotConsole() {
  const router = useRouter();
  const { isReady, isAuthenticated } = useAuth();

  const [bots, setBots] = useState<Bot[]>([]);
  const [selectedId, setSelectedId] = useState<string>("");
  const [tab, setTab] = useState<Tab>("overview");
  const [loading, setLoading] = useState(true);
  const [detail, setDetail] = useState<Bot | null>(null);
  const [tokens, setTokens] = useState<BotToken[]>([]);
  const [webhook, setWebhook] = useState<BotWebhook | null>(null);
  const [deliveries, setDeliveries] = useState<BotDelivery[]>([]);
  const [usage, setUsage] = useState<Record<string, number>>({});
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);

  const [newName, setNewName] = useState("");
  const [newDescription, setNewDescription] = useState("");
  const [newUsername, setNewUsername] = useState("");

  const [tokenName, setTokenName] = useState("default");
  const [tokenDays, setTokenDays] = useState("0");
  const [webhookURL, setWebhookURL] = useState("");
  const [webhookEvents, setWebhookEvents] = useState<string[]>([
    "message.mention",
    "message.direct",
  ]);
  const [webhookEnabled, setWebhookEnabled] = useState(true);
  const [deliveryFilter, setDeliveryFilter] = useState("");

  const selected = useMemo(
    () => bots.find((item) => item.id === selectedId) ?? detail,
    [bots, selectedId, detail]
  );

  const loadBots = useCallback(async () => {
    setLoading(true);
    try {
      const response = await botsApi.list();
      setBots(response.items);
      setSelectedId((current) => {
        if (current && response.items.some((item) => item.id === current)) {
          return current;
        }
        return response.items[0]?.id ?? "";
      });
    } catch (error) {
      if (!(error instanceof AppError && error.status === 401)) {
        toast.error(error instanceof Error ? error.message : "Failed to load bots");
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    // Data loading is async; the lint rule cannot see through the promise.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (isAuthenticated) void loadBots();
  }, [isAuthenticated, loadBots]);

  const loadDetail = useCallback(async (id: string) => {
    if (!id) return;
    try {
      const [bot, tokenList, deliveryList, usageReply] = await Promise.all([
        botsApi.detail(id),
        botsApi.listTokens(id),
        botsApi.deliveries(id),
        botsApi.usage(id),
      ]);
      setDetail(bot);
      setTokens(tokenList.tokens);
      setDeliveries(deliveryList.items);
      setUsage(usageReply.deliveries);
      try {
        const hook = await botsApi.getWebhook(id);
        setWebhook(hook);
        setWebhookURL(hook.url);
        setWebhookEvents(hook.events);
        setWebhookEnabled(hook.enabled);
      } catch (error) {
        if (error instanceof AppError && error.status === 404) {
          setWebhook(null);
          setWebhookURL("");
          setWebhookEvents(["message.mention", "message.direct"]);
          setWebhookEnabled(true);
        } else {
          throw error;
        }
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to load bot");
    }
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (selectedId) void loadDetail(selectedId);
  }, [selectedId, loadDetail]);

  const createBot = async () => {
    if (!newName.trim()) {
      toast.error("Name is required");
      return;
    }
    setBusy(true);
    try {
      const bot = await botsApi.create({
        name: newName.trim(),
        description: newDescription.trim(),
        username: newUsername.trim() || undefined,
      });
      setNewName("");
      setNewDescription("");
      setNewUsername("");
      toast.success("Bot created");
      await loadBots();
      setSelectedId(bot.id);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Create failed");
    } finally {
      setBusy(false);
    }
  };

  const createToken = async () => {
    if (!selected) return;
    setBusy(true);
    try {
      const response = await botsApi.createToken(selected.id, {
        name: tokenName.trim() || "default",
        expiresInDays: Number(tokenDays) || 0,
      });
      setSecret(response.secret);
      setTokens((current) => [response.token, ...current]);
      toast.success("Token created — copy it now");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Token create failed");
    } finally {
      setBusy(false);
    }
  };

  const revokeToken = async (tokenId: string) => {
    if (!selected) return;
    if (!window.confirm("Revoke this token? Requests using it will fail.")) return;
    try {
      await botsApi.revokeToken(selected.id, tokenId);
      setTokens((current) =>
        current.map((token) =>
          token.id === tokenId
            ? { ...token, revokedAt: new Date().toISOString() }
            : token
        )
      );
      toast.success("Token revoked");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Revoke failed");
    }
  };

  const saveWebhook = async () => {
    if (!selected) return;
    if (!webhookURL.trim()) {
      toast.error("Webhook URL is required");
      return;
    }
    setBusy(true);
    try {
      const response = await botsApi.putWebhook(selected.id, {
        url: webhookURL.trim(),
        events: webhookEvents,
        enabled: webhookEnabled,
      });
      setWebhook(response.webhook);
      if (response.secret) setSecret(response.secret);
      toast.success("Webhook saved");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Webhook save failed");
    } finally {
      setBusy(false);
    }
  };

  const testWebhook = async () => {
    if (!selected) return;
    setBusy(true);
    try {
      const result = await botsApi.testWebhook(selected.id);
      if (result.ok) {
        toast.success(`Webhook OK (${result.statusCode})`);
      } else {
        toast.error(result.error || `Webhook returned ${result.statusCode}`);
      }
      await loadDetail(selected.id);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Test failed");
    } finally {
      setBusy(false);
    }
  };

  const rotateSecret = async () => {
    if (!selected) return;
    setBusy(true);
    try {
      const response = await botsApi.rotateSecret(selected.id);
      setWebhook(response.webhook);
      setSecret(response.secret);
      toast.success("Secret rotated — update your receiver");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Rotate failed");
    } finally {
      setBusy(false);
    }
  };

  const replayDelivery = async (deliveryId: string) => {
    if (!selected) return;
    try {
      await botsApi.replayDelivery(selected.id, deliveryId);
      toast.success("Delivery queued");
      await loadDetail(selected.id);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Replay failed");
    }
  };

  const disableBot = async () => {
    if (!selected) return;
    if (!window.confirm(`Disable ${selected.name}? Tokens and webhooks stop working.`)) return;
    try {
      await botsApi.remove(selected.id);
      toast.success("Bot disabled");
      await loadBots();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Disable failed");
    }
  };

  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success("Copied");
    } catch {
      toast.error("Copy failed");
    }
  };

  if (!isReady || !isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin" />
      </div>
    );
  }

  const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://127.0.0.1:8000";
  const quickstart = `NONOKA_BASE_URL=${apiBase}
NONOKA_BOT_TOKEN=${tokens.find((token) => !token.revokedAt)?.prefix ?? "nb_..."}…
NONOKA_WEBHOOK_SECRET=${webhook?.secretPrefix ?? "whsec_..."}…

go run ./examples/bot`;

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-10 flex h-14 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur sm:px-6">
        <Button variant="ghost" size="icon" onClick={() => router.push("/chat")}>
          <ArrowLeft />
        </Button>
        <Code2 className="h-5 w-5 text-primary" />
        <h1 className="text-base font-semibold">Developer console</h1>
        <span className="hidden text-xs text-muted-foreground sm:inline">
          External bots · webhooks · credentials
        </span>
      </header>

      {secret && (
        <div className="border-b bg-amber-50 px-4 py-3 sm:px-6">
          <div className="mx-auto flex max-w-7xl flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-amber-800">
                Shown once
              </p>
              <code className="break-all text-sm text-amber-950">{secret}</code>
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="outline" onClick={() => void copy(secret)}>
                <Clipboard />
                Copy
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setSecret("")}>
                Dismiss
              </Button>
            </div>
          </div>
        </div>
      )}

      <div className="mx-auto grid max-w-7xl gap-5 px-4 py-5 lg:grid-cols-[300px_minmax(0,1fr)] sm:px-6">
        <aside className="space-y-4">
          <div className="rounded-xl border bg-card p-4">
            <p className="text-sm font-semibold">Create a bot</p>
            <div className="mt-3 space-y-2">
              <Input
                placeholder="Display name"
                value={newName}
                onChange={(event) => setNewName(event.target.value)}
              />
              <Input
                placeholder="Username (optional)"
                value={newUsername}
                onChange={(event) => setNewUsername(event.target.value)}
              />
              <Input
                placeholder="Description (optional)"
                value={newDescription}
                onChange={(event) => setNewDescription(event.target.value)}
              />
              <Button className="w-full" size="sm" onClick={() => void createBot()} disabled={busy}>
                <Plus />
                Create bot
              </Button>
            </div>
          </div>

          <div className="rounded-xl border bg-card">
            <div className="flex items-center justify-between border-b px-4 py-3">
              <p className="text-sm font-semibold">Your bots</p>
              <Button size="icon-sm" variant="ghost" onClick={() => void loadBots()}>
                <RefreshCw />
              </Button>
            </div>
            <div className="max-h-[520px] overflow-auto p-2">
              {loading ? (
                <div className="flex justify-center py-8">
                  <Loader2 className="h-5 w-5 animate-spin" />
                </div>
              ) : bots.length === 0 ? (
                <p className="px-2 py-6 text-center text-xs text-muted-foreground">
                  No external bots yet.
                </p>
              ) : (
                bots.map((bot) => (
                  <button
                    key={bot.id}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm transition-colors",
                      bot.id === selectedId
                        ? "bg-primary/10 text-primary"
                        : "hover:bg-muted"
                    )}
                    onClick={() => setSelectedId(bot.id)}
                  >
                    <BotIcon className="h-4 w-4 shrink-0" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{bot.name}</span>
                      <span className="block truncate text-[11px] text-muted-foreground">
                        @{bot.username}
                      </span>
                    </span>
                    <Badge variant={bot.status === "active" ? "default" : "secondary"}>
                      {bot.status}
                    </Badge>
                  </button>
                ))
              )}
            </div>
          </div>
        </aside>

        <main className="min-w-0">
          {!selected ? (
            <div className="flex min-h-[420px] flex-col items-center justify-center rounded-xl border border-dashed">
              <BotIcon className="h-8 w-8 text-muted-foreground" />
              <p className="mt-3 text-sm font-medium">Create or select a bot</p>
              <p className="mt-1 text-xs text-muted-foreground">
                Tokens, webhooks and delivery logs live here.
              </p>
            </div>
          ) : (
            <div className="space-y-4">
              <div className="flex flex-col gap-3 rounded-xl border bg-card p-4 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <div className="flex items-center gap-2">
                    <h2 className="text-lg font-semibold">{selected.name}</h2>
                    <Badge variant={selected.status === "active" ? "default" : "secondary"}>
                      {selected.status}
                    </Badge>
                  </div>
                  <p className="mt-1 text-sm text-muted-foreground">
                    @{selected.username} · bot user #{selected.botUserId}
                  </p>
                  {selected.description && (
                    <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
                      {selected.description}
                    </p>
                  )}
                </div>
                <Button variant="outline" size="sm" onClick={() => void disableBot()}>
                  <Trash2 />
                  Disable
                </Button>
              </div>

              <div className="flex flex-wrap gap-1 rounded-lg border p-1">
                {(
                  [
                    ["overview", "Overview"],
                    ["credentials", "Credentials"],
                    ["webhook", "Webhook"],
                    ["deliveries", "Deliveries"],
                  ] as const
                ).map(([key, label]) => (
                  <button
                    key={key}
                    className={cn(
                      "rounded-md px-3 py-1.5 text-sm transition-colors",
                      tab === key
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    )}
                    onClick={() => setTab(key)}
                  >
                    {label}
                  </button>
                ))}
              </div>

              {tab === "overview" && (
                <div className="grid gap-4 lg:grid-cols-2">
                  <div className="rounded-xl border bg-card p-4">
                    <p className="text-sm font-semibold">Delivery usage</p>
                    <div className="mt-3 grid grid-cols-2 gap-2 text-sm">
                      {["succeeded", "failed", "pending", "delivering"].map((status) => (
                        <div key={status} className="rounded-lg bg-muted/60 px-3 py-2">
                          <p className="text-xs capitalize text-muted-foreground">{status}</p>
                          <p className="text-lg font-semibold">{usage[status] ?? 0}</p>
                        </div>
                      ))}
                    </div>
                  </div>
                  <div className="rounded-xl border bg-card p-4">
                    <p className="text-sm font-semibold">Quickstart</p>
                    <pre className="mt-3 overflow-auto rounded-lg bg-muted p-3 text-[11px] leading-relaxed">
{quickstart}
                    </pre>
                    <Button
                      className="mt-3"
                      size="sm"
                      variant="outline"
                      onClick={() => void copy(quickstart)}
                    >
                      <Clipboard />
                      Copy
                    </Button>
                  </div>
                </div>
              )}

              {tab === "credentials" && (
                <div className="space-y-4">
                  <div className="rounded-xl border bg-card p-4">
                    <p className="text-sm font-semibold">Create a token</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Tokens are shown once. Use them as <code>Authorization: Bearer nb_…</code>.
                    </p>
                    <div className="mt-3 flex flex-col gap-2 sm:flex-row">
                      <Input
                        placeholder="Token name"
                        value={tokenName}
                        onChange={(event) => setTokenName(event.target.value)}
                      />
                      <Input
                        className="sm:w-40"
                        type="number"
                        min={0}
                        value={tokenDays}
                        onChange={(event) => setTokenDays(event.target.value)}
                        placeholder="Expires in days"
                      />
                      <Button onClick={() => void createToken()} disabled={busy}>
                        <KeyRound />
                        Create
                      </Button>
                    </div>
                  </div>
                  <div className="overflow-hidden rounded-xl border bg-card">
                    <table className="w-full text-left text-sm">
                      <thead className="bg-muted/60 text-xs text-muted-foreground">
                        <tr>
                          <th className="px-3 py-2 font-medium">Name</th>
                          <th className="px-3 py-2 font-medium">Prefix</th>
                          <th className="px-3 py-2 font-medium">Last used</th>
                          <th className="px-3 py-2 font-medium">Status</th>
                          <th className="px-3 py-2" />
                        </tr>
                      </thead>
                      <tbody>
                        {tokens.map((token) => (
                          <tr key={token.id} className="border-t">
                            <td className="px-3 py-2">{token.name}</td>
                            <td className="px-3 py-2 font-mono text-xs">{token.prefix}…</td>
                            <td className="px-3 py-2 text-xs text-muted-foreground">
                              {token.lastUsedAt ? new Date(token.lastUsedAt).toLocaleString() : "Never"}
                            </td>
                            <td className="px-3 py-2">
                              {token.revokedAt ? (
                                <Badge variant="secondary">revoked</Badge>
                              ) : (
                                <Badge variant="default">active</Badge>
                              )}
                            </td>
                            <td className="px-3 py-2 text-right">
                              {!token.revokedAt && (
                                <Button
                                  size="sm"
                                  variant="ghost"
                                  onClick={() => void revokeToken(token.id)}
                                >
                                  Revoke
                                </Button>
                              )}
                            </td>
                          </tr>
                        ))}
                        {tokens.length === 0 && (
                          <tr>
                            <td colSpan={5} className="px-3 py-8 text-center text-xs text-muted-foreground">
                              No tokens yet.
                            </td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {tab === "webhook" && (
                <div className="space-y-4">
                  <div className="rounded-xl border bg-card p-4">
                    <div className="flex items-center gap-2">
                      <Webhook className="h-4 w-4 text-primary" />
                      <p className="text-sm font-semibold">Delivery endpoint</p>
                    </div>
                    <div className="mt-3 space-y-3">
                      <div>
                        <Label htmlFor="webhook-url">URL</Label>
                        <Input
                          id="webhook-url"
                          className="mt-1"
                          placeholder="https://example.com/nonoka/webhook"
                          value={webhookURL}
                          onChange={(event) => setWebhookURL(event.target.value)}
                        />
                      </div>
                      <div className="flex flex-wrap gap-3">
                        {EVENT_OPTIONS.map((option) => (
                          <label key={option.key} className="flex items-center gap-2 text-sm">
                            <input
                              type="checkbox"
                              checked={webhookEvents.includes(option.key)}
                              onChange={(event) =>
                                setWebhookEvents((current) =>
                                  event.target.checked
                                    ? [...current, option.key]
                                    : current.filter((item) => item !== option.key)
                                )
                              }
                            />
                            {option.label}
                          </label>
                        ))}
                        <label className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            checked={webhookEnabled}
                            onChange={(event) => setWebhookEnabled(event.target.checked)}
                          />
                          Enabled
                        </label>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <Button onClick={() => void saveWebhook()} disabled={busy}>
                          <Check />
                          Save
                        </Button>
                        <Button variant="outline" onClick={() => void testWebhook()} disabled={busy}>
                          <Send />
                          Send test
                        </Button>
                        {webhook && (
                          <Button variant="outline" onClick={() => void rotateSecret()} disabled={busy}>
                            Rotate secret
                          </Button>
                        )}
                      </div>
                      {webhook && (
                        <p className="text-xs text-muted-foreground">
                          Secret {webhook.secretPrefix} · last success{" "}
                          {webhook.lastSuccessAt
                            ? new Date(webhook.lastSuccessAt).toLocaleString()
                            : "never"}{" "}
                          · failures {webhook.consecutiveFailures}
                        </p>
                      )}
                    </div>
                  </div>
                </div>
              )}

              {tab === "deliveries" && (
                <div className="overflow-hidden rounded-xl border bg-card">
                  <div className="flex flex-wrap items-center justify-between gap-2 border-b p-3">
                    <p className="text-sm font-semibold">Delivery log</p>
                    <div className="flex gap-1">
                      {["", "failed", "succeeded", "pending"].map((status) => (
                        <button
                          key={status || "all"}
                          className={cn(
                            "rounded-md px-2 py-1 text-xs",
                            deliveryFilter === status
                              ? "bg-primary text-primary-foreground"
                              : "text-muted-foreground hover:bg-muted"
                          )}
                          onClick={async () => {
                            setDeliveryFilter(status);
                            if (!selected) return;
                            try {
                              const response = await botsApi.deliveries(selected.id, status);
                              setDeliveries(response.items);
                            } catch (error) {
                              toast.error(error instanceof Error ? error.message : "Load failed");
                            }
                          }}
                        >
                          {status || "all"}
                        </button>
                      ))}
                    </div>
                  </div>
                  <table className="w-full text-left text-sm">
                    <thead className="bg-muted/60 text-xs text-muted-foreground">
                      <tr>
                        <th className="px-3 py-2 font-medium">Time</th>
                        <th className="px-3 py-2 font-medium">Type</th>
                        <th className="px-3 py-2 font-medium">Status</th>
                        <th className="px-3 py-2 font-medium">Attempts</th>
                        <th className="px-3 py-2 font-medium">Response</th>
                        <th className="px-3 py-2" />
                      </tr>
                    </thead>
                    <tbody>
                      {deliveries.map((delivery) => (
                        <tr key={delivery.id} className="border-t">
                          <td className="whitespace-nowrap px-3 py-2 text-xs text-muted-foreground">
                            {new Date(delivery.createdAt).toLocaleString()}
                          </td>
                          <td className="px-3 py-2 text-xs">{delivery.eventType}</td>
                          <td className="px-3 py-2">
                            <Badge
                              variant={
                                delivery.status === "succeeded"
                                  ? "default"
                                  : delivery.status === "failed"
                                  ? "destructive"
                                  : "secondary"
                              }
                            >
                              {delivery.status}
                            </Badge>
                          </td>
                          <td className="px-3 py-2 text-xs">{delivery.attempts}</td>
                          <td className="max-w-[260px] px-3 py-2 text-xs text-muted-foreground">
                            <span className="line-clamp-2">
                              {delivery.lastStatusCode
                                ? `${delivery.lastStatusCode} `
                                : ""}
                              {delivery.lastError || delivery.responseSnippet || "—"}
                            </span>
                          </td>
                          <td className="px-3 py-2 text-right">
                            {delivery.status === "failed" && (
                              <Button
                                size="sm"
                                variant="ghost"
                                onClick={() => void replayDelivery(delivery.id)}
                              >
                                Replay
                              </Button>
                            )}
                          </td>
                        </tr>
                      ))}
                      {deliveries.length === 0 && (
                        <tr>
                          <td colSpan={6} className="px-3 py-8 text-center text-xs text-muted-foreground">
                            No deliveries yet.
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}
        </main>
      </div>
    </div>
  );
}
