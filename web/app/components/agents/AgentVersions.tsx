"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { diffLines } from "diff";
import { History, Loader2, RotateCcw } from "lucide-react";
import {
  agentApi,
  type ManagedAgent,
  type AgentVersionSummary,
} from "@/lib/api/agents";
import { agentKeys, agentVersionQuery, agentVersionsQuery } from "@/lib/queries/agents";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { errorText, formatTime } from "./agent-utils";

export function AgentVersions({ agent }: { agent: ManagedAgent }) {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<number | null>(null);
  const versions = useQuery(agentVersionsQuery(agent.id));
  const detail = useQuery(agentVersionQuery(agent.id, selected ?? 0));
  const restore = useMutation({
    mutationFn: () =>
      agentApi.restore(agent.id, selected || 0, agent.draft?.revision || 0),
    onSuccess: () => {
      toast.success("Version restored to draft — review and publish when ready");
      queryClient.invalidateQueries({ queryKey: agentKeys.detail(agent.id) });
      queryClient.invalidateQueries({ queryKey: agentKeys.versions(agent.id) });
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const diff = useMemo(() => {
    if (!detail.data) return [];
    const before = detail.data.instruction || "";
    const after = agent.draft?.instruction || "";
    if (before === after) return [];
    return diffLines(before, after);
  }, [detail.data, agent.draft?.instruction]);

  return (
    <div className="grid max-w-6xl gap-5 lg:grid-cols-[300px_minmax(0,1fr)]">
      <div className="rounded-xl border bg-card">
        <div className="flex items-center gap-2 border-b px-4 py-3">
          <History className="h-4 w-4 text-muted-foreground" />
          <h3 className="text-sm font-semibold">Versions</h3>
          <Badge variant="secondary">{versions.data?.versions.length ?? 0}</Badge>
        </div>
        <div className="max-h-[calc(100vh-17rem)] overflow-y-auto p-2">
          {versions.isPending && (
            <Loader2 className="mx-auto my-6 h-4 w-4 animate-spin text-muted-foreground" />
          )}
          {(versions.data?.versions ?? []).map((version: AgentVersionSummary) => (
            <button
              key={version.id}
              className={cn(
                "mb-1 w-full rounded-lg border p-3 text-left transition-colors",
                selected === version.version
                  ? "border-primary/50 bg-primary/5"
                  : "hover:bg-muted/50"
              )}
              onClick={() => setSelected(version.version)}
            >
              <div className="flex items-center justify-between gap-2">
                <span className="text-sm font-semibold">v{version.version}</span>
                {version.version === agent.currentVersion && (
                  <Badge variant="secondary" className="text-[9px]">
                    current
                  </Badge>
                )}
              </div>
              <p className="mt-1 truncate text-xs text-muted-foreground">
                {version.modelKey}
              </p>
              <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                {version.publishNote || "No publish note"}
              </p>
              <p className="mt-1 text-[10px] text-muted-foreground">
                {formatTime(version.publishedAt || version.createdAt)}
              </p>
            </button>
          ))}
          {!versions.isPending && (versions.data?.versions.length ?? 0) === 0 && (
            <p className="p-4 text-center text-xs text-muted-foreground">
              No published versions yet.
            </p>
          )}
        </div>
      </div>

      <div className="min-w-0 space-y-4">
        {selected === null ? (
          <div className="flex h-40 items-center justify-center rounded-xl border border-dashed text-sm text-muted-foreground">
            Select a version to inspect it.
          </div>
        ) : detail.isPending ? (
          <div className="flex h-40 items-center justify-center rounded-xl border border-dashed">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        ) : detail.data ? (
          <>
            <div className="rounded-xl border bg-card p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-base font-semibold">
                    Version {detail.data.version}
                  </h3>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Published {formatTime(detail.data.publishedAt)} by user #
                    {detail.data.publishedBy} · source revision{" "}
                    {detail.data.sourceRevision}
                  </p>
                  {detail.data.publishNote && (
                    <p className="mt-2 text-sm">{detail.data.publishNote}</p>
                  )}
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={restore.isPending || !agent.canManage}
                  onClick={() => restore.mutate()}
                >
                  {restore.isPending ? <Loader2 className="animate-spin" /> : <RotateCcw />}
                  Restore to draft
                </Button>
              </div>
              <div className="mt-3 flex flex-wrap gap-1.5 text-xs">
                <Badge variant="secondary">model {detail.data.modelKey}</Badge>
                <Badge variant="secondary">
                  {detail.data.capabilityPresets.length} presets
                </Badge>
                <Badge variant="secondary">
                  {detail.data.mcpBindings.length} MCP tools
                </Badge>
                <Badge variant="secondary">{detail.data.skills.length} skills</Badge>
                <Badge variant="secondary">max {detail.data.maxTokens} tokens</Badge>
              </div>
            </div>

            <div className="rounded-xl border bg-card">
              <div className="border-b px-4 py-3">
                <h3 className="text-sm font-semibold">Diff against current draft</h3>
                <p className="text-xs text-muted-foreground">
                  Restore copies this version into the editable draft; it does not publish.
                </p>
              </div>
              {diff.length === 0 ? (
                <p className="p-4 text-sm text-muted-foreground">
                  No instruction changes.
                </p>
              ) : (
                <div className="max-h-[50vh] overflow-auto p-4 font-mono text-xs">
                  {diff.map((part, index) => (
                    <div
                      key={index}
                      className={cn(
                        "whitespace-pre-wrap px-2 py-0.5",
                        part.added && "bg-emerald-100 text-emerald-900 dark:bg-emerald-500/15 dark:text-emerald-300",
                        part.removed && "bg-red-100 text-red-900 line-through dark:bg-red-500/15 dark:text-red-300"
                      )}
                    >
                      {part.value}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </>
        ) : (
          <p className="text-sm text-destructive">Failed to load version.</p>
        )}
      </div>
    </div>
  );
}
