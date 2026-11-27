"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  ArrowUpRight,
  Bot,
  Cpu,
  ExternalLink,
  Network,
  RefreshCw,
  ShieldAlert,
} from "lucide-react";
import { agentApi } from "@/lib/api/agents";
import { approvalsQuery, catalogQuery } from "@/lib/queries/agents";
import { gatewaysQuery } from "@/lib/queries/system";
import { Button, buttonVariants } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { formatTime } from "@/components/agents/agent-utils";

const BIFROST_DASHBOARD_URL =
  process.env.NEXT_PUBLIC_BIFROST_DASHBOARD_URL || "http://localhost:8088";

export function AdminOverview() {
  const gateways = useQuery(gatewaysQuery);
  const catalog = useQuery(catalogQuery);
  const approvals = useQuery(approvalsQuery("pending"));
  const outbox = useQuery({
    queryKey: ["admin", "dlq", "overview"],
    queryFn: () => agentApi.failedOutbox(100),
    refetchInterval: 15_000,
  });

  const models = catalog.data?.models ?? [];
  const enabledModels = models.filter((model) => model.enabled && model.curated);
  const gatewayNodes = gateways.data?.gateways ?? [];
  const connections = gatewayNodes.reduce(
    (total, node) => total + node.connections,
    0
  );

  const cards = [
    {
      label: "Live gateways",
      value: String(gatewayNodes.length),
      hint: gateways.data?.degraded
        ? "Registry degraded"
        : `${connections} active connections`,
      icon: Network,
      tone: gateways.data?.degraded ? "warning" : "success",
    },
    {
      label: "Enabled models",
      value: String(enabledModels.length),
      hint: `${models.length} total in catalog`,
      icon: Cpu,
      tone: "default",
    },
    {
      label: "Pending approvals",
      value: String(approvals.data?.items.length ?? 0),
      hint: "Waiting for your decision",
      icon: ShieldAlert,
      tone: (approvals.data?.items.length ?? 0) > 0 ? "warning" : "success",
    },
    {
      label: "DLQ failures",
      value: String(outbox.data?.items.length ?? 0),
      hint: "Agent trigger dead letters",
      icon: Activity,
      tone: (outbox.data?.items.length ?? 0) > 0 ? "danger" : "success",
    },
  ];

  return (
    <div className="space-y-6">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {cards.map((card) => (
          <div key={card.label} className="rounded-xl border bg-card p-4">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">{card.label}</span>
              <card.icon className="h-4 w-4 text-muted-foreground" />
            </div>
            <p className="mt-2 text-2xl font-semibold tracking-tight">
              {card.value}
            </p>
            <p
              className={cn(
                "mt-0.5 text-[11px]",
                card.tone === "warning"
                  ? "text-warning"
                  : card.tone === "danger"
                    ? "text-destructive"
                    : card.tone === "success"
                      ? "text-success"
                      : "text-muted-foreground"
              )}
            >
              {card.hint}
            </p>
          </div>
        ))}
      </div>

      <section className="space-y-3">
        <div className="flex flex-wrap items-end justify-between gap-2">
          <div>
            <h2 className="text-sm font-semibold">Gateway registry</h2>
            <p className="text-xs text-muted-foreground">
              Live nodes discovered from Redis heartbeats. Dispatch strategy:{" "}
              <span className="font-mono">
                {gateways.data?.strategy || "consistent_hash"}
              </span>
              {gateways.data ? ` · TTL ${gateways.data.nodeTtlSeconds}s` : ""}
            </p>
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => void gateways.refetch()}
              disabled={gateways.isFetching}
            >
              <RefreshCw className={cn(gateways.isFetching && "animate-spin")} />
              Refresh
            </Button>
            <a
              href={BIFROST_DASHBOARD_URL}
              target="_blank"
              rel="noreferrer"
              className={buttonVariants({ variant: "outline", size: "sm" })}
            >
              <ExternalLink />
              Bifrost
            </a>
          </div>
        </div>

        {gateways.isError ? (
          <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
            Failed to load gateway registry: {gateways.error.message}
          </div>
        ) : gateways.data?.degraded ? (
          <div className="rounded-xl border border-warning/40 bg-warning/10 p-4 text-sm">
            <p className="font-medium text-warning">Registry degraded</p>
            <p className="mt-1 text-xs text-muted-foreground">
              {gateways.data.warning ||
                "The gateway registry is unavailable. Nodes may still serve traffic."}
            </p>
          </div>
        ) : null}

        <div className="overflow-hidden rounded-xl border bg-card">
          {gatewayNodes.length === 0 ? (
            <p className="p-6 text-center text-sm text-muted-foreground">
              No live gateway nodes reported.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[720px] text-left text-sm">
                <thead className="border-b bg-muted/30 text-xs text-muted-foreground">
                  <tr>
                    <th className="px-4 py-2.5">Node</th>
                    <th>WebSocket</th>
                    <th>gRPC</th>
                    <th>Connections</th>
                    <th>Heartbeat</th>
                    <th className="pr-4 text-right">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {gatewayNodes.map((node) => (
                    <tr key={node.nodeId} className="border-b last:border-0">
                      <td className="px-4 py-3 font-mono text-xs">
                        {node.nodeId}
                      </td>
                      <td className="font-mono text-xs text-muted-foreground">
                        {node.url}
                      </td>
                      <td className="font-mono text-xs text-muted-foreground">
                        {node.grpcAddr}
                      </td>
                      <td className="text-xs">{node.connections}</td>
                      <td className="text-xs text-muted-foreground">
                        {formatTime(node.heartbeatAt)}
                      </td>
                      <td className="pr-4 text-right">
                        <Badge variant="secondary" className="text-[10px]">
                          <span className="mr-1 h-1.5 w-1.5 rounded-full bg-success" />
                          alive
                        </Badge>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </section>

      <section className="grid gap-3 lg:grid-cols-2">
        <div className="rounded-xl border bg-card p-4">
          <div className="flex items-center justify-between">
            <h2 className="flex items-center gap-2 text-sm font-semibold">
              <Bot className="h-4 w-4 text-primary" />
              Model catalog
            </h2>
            <a
              href={BIFROST_DASHBOARD_URL}
              target="_blank"
              rel="noreferrer"
              className="text-xs text-primary hover:underline"
            >
              Manage in Bifrost <ArrowUpRight className="inline h-3 w-3" />
            </a>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            Provider keys, routing and budgets live in Bifrost. This workspace
            only curates which synced models agents may use.
          </p>
          <div className="mt-3 flex flex-wrap gap-1.5">
            {enabledModels.slice(0, 8).map((model) => (
              <Badge key={model.id} variant="secondary" className="text-[10px]">
                {model.icon ? `${model.icon} ` : ""}
                {model.displayName || model.modelKey}
              </Badge>
            ))}
            {enabledModels.length === 0 && (
              <span className="text-xs text-muted-foreground">
                No curated models yet.
              </span>
            )}
          </div>
        </div>

        <div className="rounded-xl border bg-card p-4">
          <h2 className="text-sm font-semibold">Operator checklist</h2>
          <ul className="mt-2 space-y-2 text-xs text-muted-foreground">
            <li className="flex items-start gap-2">
              <span className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
              Configure provider credentials and budgets in Bifrost.
            </li>
            <li className="flex items-start gap-2">
              <span className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
              Sync the catalog, then enable and curate the models agents can use.
            </li>
            <li className="flex items-start gap-2">
              <span className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
              Review MCP servers and Skills before sharing agents that bind them.
            </li>
            <li className="flex items-start gap-2">
              <span className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
              Watch DLQ failures and replay only after the root cause is fixed.
            </li>
          </ul>
        </div>
      </section>
    </div>
  );
}
