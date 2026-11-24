"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity, Coins, MessageSquare, ShieldAlert, Sparkles } from "lucide-react";
import type { ManagedAgent } from "@/lib/api/agents";
import { agentRunsQuery, approvalsQuery, catalogQuery } from "@/lib/queries/agents";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { AgentApprovalInbox } from "./AgentApprovalInbox";
import { formatDuration, formatTime } from "./agent-utils";

export function AgentOverview({
  agent,
  onTab,
}: {
  agent: ManagedAgent;
  onTab: (tab: string) => void;
}) {
  const runs = useQuery(agentRunsQuery(agent.id));
  const approvals = useQuery(approvalsQuery("pending"));
  const catalog = useQuery(catalogQuery);
  const recent = (runs.data?.items ?? []).slice(0, 5);
  const totalTokens = (runs.data?.items ?? []).reduce(
    (sum, run) => sum + (run.totalTokens || 0),
    0
  );
  const model = catalog.data?.services
    .flatMap((service) => service.models)
    .find((item) => item.id === agent.published?.modelId);
  const estimatedCost = (runs.data?.items ?? []).reduce((sum, run) => {
    if (!model) return sum;
    return (
      sum +
      ((run.inputTokens || 0) / 1_000_000) * model.inputPricePerMtok +
      ((run.outputTokens || 0) / 1_000_000) * model.outputPricePerMtok
    );
  }, 0);
  const pendingApprovals = (approvals.data?.items ?? []).filter(
    (approval) => approval.approverId === agent.ownerUserId
  ).length;

  const cards = [
    {
      label: "Current version",
      value: agent.currentVersion ? `v${agent.currentVersion}` : "Unpublished",
      icon: Sparkles,
      hint: agent.published?.modelKey || "Publish to start chatting",
    },
    {
      label: "Draft",
      value: agent.draftChanged ? "Unpublished changes" : "Up to date",
      icon: Activity,
      hint: agent.draft ? `Revision ${agent.draft.revision}` : "No draft",
    },
    {
      label: "Recent spend",
      value: model ? `$${estimatedCost.toFixed(4)}` : "-",
      icon: Coins,
      hint: `${totalTokens.toLocaleString()} tokens · ${runs.data?.items.length ?? 0} runs`,
    },
    {
      label: "Pending approvals",
      value: String(pendingApprovals),
      icon: ShieldAlert,
      hint: "Across your inbox",
    },
  ];

  return (
    <div className="max-w-5xl space-y-5">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {cards.map((card) => (
          <div key={card.label} className="rounded-xl border bg-card p-4">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">{card.label}</span>
              <card.icon className="h-4 w-4 text-muted-foreground" />
            </div>
            <p className="mt-2 truncate text-lg font-semibold">{card.value}</p>
            <p className="mt-0.5 truncate text-[11px] text-muted-foreground">
              {card.hint}
            </p>
          </div>
        ))}
      </div>

      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="space-y-4">
          <div className="rounded-xl border bg-card">
            <div className="flex items-center justify-between border-b px-4 py-3">
              <h3 className="text-sm font-semibold">Recent runs</h3>
              <Button variant="ghost" size="xs" onClick={() => onTab("runs")}>
                View all
              </Button>
            </div>
            <div className="divide-y">
              {recent.map((run) => (
                <button
                  key={run.id}
                  className="flex w-full items-center justify-between gap-3 px-4 py-2.5 text-left text-xs hover:bg-muted/40"
                  onClick={() => onTab("runs")}
                >
                  <span className="font-mono">#{run.id}</span>
                  <Badge
                    variant="secondary"
                    className={cn(
                      "text-[10px] capitalize",
                      run.status === "failed" && "bg-red-100 text-red-800 dark:bg-red-500/15 dark:text-red-400"
                    )}
                  >
                    {run.status}
                  </Badge>
                  <span className="text-muted-foreground">
                    {formatDuration(run.startedAt, run.finishedAt)}
                  </span>
                  <span className="hidden text-muted-foreground sm:inline">
                    {formatTime(run.createdAt)}
                  </span>
                </button>
              ))}
              {recent.length === 0 && (
                <p className="p-4 text-sm text-muted-foreground">
                  No runs yet. Publish and start a chat, or use the Test tab.
                </p>
              )}
            </div>
          </div>

          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-sm font-semibold">Published instructions</h3>
            <p className="mt-2 max-h-56 overflow-y-auto whitespace-pre-wrap text-sm text-muted-foreground">
              {agent.published?.instruction || "Not published yet."}
            </p>
            <div className="mt-3 flex flex-wrap gap-1.5">
              {(agent.published?.capabilityPresets ?? []).map((preset) => (
                <Badge key={preset} variant="secondary" className="text-[10px]">
                  {preset}
                </Badge>
              ))}
              {(agent.published?.mcpBindings ?? []).map((binding) => (
                <Badge key={`${binding.serverId}/${binding.tool}`} variant="secondary" className="text-[10px]">
                  {binding.tool}
                </Badge>
              ))}
              {(agent.published?.skills ?? []).map((skill) => (
                <Badge key={skill.skillId} variant="secondary" className="text-[10px]">
                  skill {skill.name || skill.skillId} v{skill.version}
                </Badge>
              ))}
            </div>
          </div>
        </div>

        <div className="space-y-4">
          <AgentApprovalInbox compact />
          <div className="rounded-xl border bg-card p-4">
            <h3 className="text-sm font-semibold">Quick actions</h3>
            <div className="mt-3 grid gap-2">
              <Button variant="outline" size="sm" onClick={() => onTab("test")}>
                <MessageSquare />
                Test draft
              </Button>
              <Button variant="outline" size="sm" onClick={() => onTab("build")}>
                Edit configuration
              </Button>
              <Button variant="outline" size="sm" onClick={() => onTab("versions")}>
                View versions
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
