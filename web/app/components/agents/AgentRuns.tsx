"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Ban, Loader2, Play, RefreshCw, RotateCw, Wrench } from "lucide-react";
import { agentApi, type AgentRun } from "@/lib/api/agents";
import { agentKeys, agentRunsQuery } from "@/lib/queries/agents";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { errorText, formatDuration, formatTime } from "./agent-utils";

const statusClass: Record<string, string> = {
  completed: "bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-400",
  failed: "bg-red-100 text-red-800 dark:bg-red-500/15 dark:text-red-400",
  cancelled: "bg-muted text-muted-foreground",
  running: "bg-blue-100 text-blue-800 dark:bg-blue-500/15 dark:text-blue-400",
  waiting_approval: "bg-amber-100 text-amber-800 dark:bg-amber-500/15 dark:text-amber-400",
};

export function AgentRuns({ agentId }: { agentId: number }) {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<number | null>(null);
  const [filter, setFilter] = useState("all");
  const runs = useQuery(agentRunsQuery(agentId));
  const detail = useQuery({
    queryKey: agentKeys.run(selected ?? 0),
    queryFn: () => agentApi.runDetail(selected!),
    enabled: !!selected,
    refetchInterval: (query) => {
      const data = query.state.data as AgentRun | undefined;
      if (!data) return 2000;
      return ["completed", "failed", "cancelled"].includes(data.status) ? false : 2000;
    },
  });

  const act = useMutation({
    mutationFn: async ({ run, action }: { run: AgentRun; action: "cancel" | "retry" | "skip" }) => {
      if (action === "cancel") await agentApi.cancelRun(run.id);
      else await agentApi.runAction(run.id, action);
      return action;
    },
    onSuccess: (action) => {
      toast.success(
        action === "cancel" ? "Run cancelled" : action === "retry" ? "Run queued" : "Run skipped"
      );
      queryClient.invalidateQueries({ queryKey: agentKeys.runs(agentId) });
      if (selected) queryClient.invalidateQueries({ queryKey: agentKeys.run(selected) });
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const filtered = (runs.data?.items ?? []).filter(
    (run) => filter === "all" || run.status === filter
  );

  return (
    <div className="max-w-6xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-1 rounded-lg border p-1">
          {["all", "running", "waiting_approval", "completed", "failed", "cancelled"].map(
            (status) => (
              <button
                key={status}
                className={cn(
                  "rounded-md px-2.5 py-1 text-xs capitalize",
                  filter === status
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
                onClick={() => setFilter(status)}
              >
                {status.replace("_", " ")}
              </button>
            )
          )}
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => queryClient.invalidateQueries({ queryKey: agentKeys.runs(agentId) })}
        >
          <RefreshCw />
          Refresh
        </Button>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] text-left text-sm">
            <thead className="border-b bg-muted/30 text-xs text-muted-foreground">
              <tr>
                <th className="px-4 py-2.5">Run</th>
                <th>Actor</th>
                <th>Status</th>
                <th>Error</th>
                <th>Tokens</th>
                <th>Cost</th>
                <th>Duration</th>
                <th>Created</th>
                <th className="pr-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((run) => (
                <tr
                  key={run.id}
                  className={cn(
                    "cursor-pointer border-b last:border-0 hover:bg-muted/30",
                    selected === run.id && "bg-muted/40"
                  )}
                  onClick={() => setSelected(selected === run.id ? null : run.id)}
                >
                  <td className="px-4 py-3 font-mono text-xs">#{run.id}</td>
                  <td className="text-xs">#{run.actorUserId}</td>
                  <td>
                    <Badge
                      variant="secondary"
                      className={cn("text-[10px]", statusClass[run.status])}
                    >
                      {run.status}
                    </Badge>
                  </td>
                  <td className="max-w-32 truncate text-xs text-destructive">
                    {run.errorCode || "-"}
                  </td>
                  <td className="text-xs">{run.totalTokens}</td>
                  <td className="text-xs">
                    {typeof run.estimatedCostUsd === "number" && run.estimatedCostUsd > 0
                      ? `$${run.estimatedCostUsd.toFixed(6)}`
                      : "-"}
                  </td>
                  <td className="text-xs">{formatDuration(run.startedAt, run.finishedAt)}</td>
                  <td className="text-xs text-muted-foreground">
                    {formatTime(run.createdAt)}
                  </td>
                  <td className="pr-4 text-right" onClick={(event) => event.stopPropagation()}>
                    <div className="flex justify-end gap-1">
                      {["queued", "running", "waiting_approval"].includes(run.status) && (
                        <Button
                          size="xs"
                          variant="outline"
                          disabled={act.isPending}
                          onClick={() => act.mutate({ run, action: "cancel" })}
                        >
                          <Ban />
                          Cancel
                        </Button>
                      )}
                      {run.status === "failed" && (
                        <>
                          <Button
                            size="xs"
                            variant="outline"
                            disabled={act.isPending}
                            onClick={() => act.mutate({ run, action: "retry" })}
                          >
                            <RotateCw />
                            Retry
                          </Button>
                          <Button
                            size="xs"
                            variant="ghost"
                            disabled={act.isPending}
                            onClick={() => act.mutate({ run, action: "skip" })}
                          >
                            Skip
                          </Button>
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {runs.isPending && (
          <div className="flex justify-center p-6">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        )}
        {!runs.isPending && filtered.length === 0 && (
          <p className="p-6 text-center text-sm text-muted-foreground">No runs.</p>
        )}
      </div>

      {selected && (
        <div className="rounded-xl border bg-card p-4">
          {detail.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          ) : detail.data ? (
            <div className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <h3 className="text-sm font-semibold">
                    Run #{detail.data.id} · {detail.data.status}
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    {detail.data.errorClass || "run"} · {formatTime(detail.data.createdAt)}
                  </p>
                </div>
                <div className="flex gap-1">
                  {detail.data.status === "failed" && (
                    <Button
                      size="xs"
                      variant="outline"
                      onClick={() => act.mutate({ run: detail.data!, action: "retry" })}
                    >
                      <Play />
                      Retry
                    </Button>
                  )}
                  {["queued", "running", "waiting_approval"].includes(detail.data.status) && (
                    <Button
                      size="xs"
                      variant="outline"
                      onClick={() => act.mutate({ run: detail.data!, action: "cancel" })}
                    >
                      <Ban />
                      Cancel
                    </Button>
                  )}
                </div>
              </div>

              <dl className="grid grid-cols-2 gap-3 text-xs sm:grid-cols-4">
                <div>
                  <dt className="text-muted-foreground">Attempts</dt>
                  <dd>{detail.data.attempts ?? "-"}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Agent version</dt>
                  <dd>{detail.data.agentVersion ? `v${detail.data.agentVersion}` : "-"}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Reply message</dt>
                  <dd>{detail.data.replyMsgId ? `#${detail.data.replyMsgId}` : "-"}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Tokens</dt>
                  <dd>
                    {detail.data.inputTokens} in · {detail.data.outputTokens} out
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Model</dt>
                  <dd className="truncate" title={detail.data.modelKey}>{detail.data.modelKey || "-"}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Estimated cost</dt>
                  <dd>
                    {typeof detail.data.estimatedCostUsd === "number"
                      ? `$${detail.data.estimatedCostUsd.toFixed(6)}`
                      : "-"}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Retry after</dt>
                  <dd>{detail.data.retryAfterAt ? formatTime(detail.data.retryAfterAt) : "-"}</dd>
                </div>
              </dl>

              {detail.data.lastError && (
                <p className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">
                  {detail.data.lastError}
                </p>
              )}

              <div className="grid gap-4 lg:grid-cols-2">
                <div>
                  <h4 className="mb-2 flex items-center gap-1.5 text-xs font-semibold">
                    <Wrench className="h-3.5 w-3.5" />
                    Timeline
                  </h4>
                  <div className="space-y-1.5">
                    {(detail.data.steps ?? []).map((step) => (
                      <div key={step.id} className="rounded-md border p-2 text-xs">
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-mono">{step.type}</span>
                          <span className="text-[10px] text-muted-foreground">
                            {formatTime(step.createdAt)}
                          </span>
                        </div>
                        {(step.tool || step.summary) && (
                          <p className="mt-1 truncate text-muted-foreground">
                            {step.tool} {step.summary}
                          </p>
                        )}
                      </div>
                    ))}
                    {(detail.data.steps ?? []).length === 0 && (
                      <p className="text-xs text-muted-foreground">No timeline events.</p>
                    )}
                  </div>
                </div>
                <div>
                  <h4 className="mb-2 text-xs font-semibold">Tool ledger</h4>
                  <div className="space-y-1.5">
                    {(detail.data.toolInvocations ?? []).map((tool) => (
                      <div key={tool.id} className="rounded-md border p-2 text-xs">
                        <div className="flex items-center justify-between gap-2">
                          <span className="font-mono">{tool.tool}</span>
                          <Badge variant="secondary" className="text-[9px]">
                            {tool.status}
                          </Badge>
                        </div>
                        {tool.resultSummary && (
                          <p className="mt-1 line-clamp-2 text-muted-foreground">
                            {tool.resultSummary}
                          </p>
                        )}
                      </div>
                    ))}
                    {(detail.data.toolInvocations ?? []).length === 0 && (
                      <p className="text-xs text-muted-foreground">No side-effecting calls.</p>
                    )}
                  </div>
                </div>
              </div>
            </div>
          ) : (
            <p className="text-sm text-destructive">Failed to load run detail.</p>
          )}
        </div>
      )}
    </div>
  );
}
