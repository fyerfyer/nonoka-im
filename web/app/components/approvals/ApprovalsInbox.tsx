"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import {
  Check,
  ChevronRight,
  Loader2,
  MessageSquare,
  ShieldCheck,
  X,
} from "lucide-react";
import { agentApi, type AgentApproval, type AgentRun } from "@/lib/api/agents";
import { approvalsQuery, agentKeys } from "@/lib/queries/agents";
import { useAuthStore } from "@/stores/authStore";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { PageHeader } from "@/components/workspace/PageHeader";
import { EmptyState } from "@/components/workspace/EmptyState";
import { cn } from "@/lib/utils";
import { formatTime } from "@/components/agents/agent-utils";

const FILTERS = [
  { key: "pending", label: "Pending" },
  { key: "approved", label: "Approved" },
  { key: "rejected", label: "Rejected" },
  { key: "all", label: "All" },
] as const;

type Filter = (typeof FILTERS)[number]["key"];

function statusClass(status: string): string {
  switch (status) {
    case "pending":
      return "bg-warning/10 text-warning";
    case "approved":
    case "consumed":
      return "bg-success/10 text-success";
    case "rejected":
    case "expired":
      return "bg-destructive/10 text-destructive";
    default:
      return "";
  }
}

export function ApprovalsInbox() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const token = useAuthStore((state) => state.token);
  const [filter, setFilter] = useState<Filter>("pending");
  const [context, setContext] = useState<AgentRun | null>(null);
  const [contextLoading, setContextLoading] = useState(false);

  const approvals = useQuery({
    ...approvalsQuery(filter === "all" ? "" : filter),
    enabled: !!token,
  });

  const decide = useMutation({
    mutationFn: async ({
      approval,
      action,
    }: {
      approval: AgentApproval;
      action: "approve" | "reject";
    }) => {
      if (action === "approve") await agentApi.approveApproval(approval.id);
      else await agentApi.rejectApproval(approval.id);
      return action;
    },
    onSuccess: (action) => {
      toast.success(
        action === "approve" ? "Approval granted" : "Approval rejected"
      );
      void queryClient.invalidateQueries({
        queryKey: [...agentKeys.all, "approvals"],
      });
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "Action failed"),
  });

  const items = useMemo(
    () => approvals.data?.items ?? [],
    [approvals.data?.items]
  );

  const openContext = async (approval: AgentApproval) => {
    setContextLoading(true);
    try {
      setContext(await agentApi.runDetail(approval.runId));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to load run context"
      );
    } finally {
      setContextLoading(false);
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <PageHeader
        title="Approvals"
        description="Review tool calls that need a human decision."
        icon={ShieldCheck}
      />

      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-4 py-2.5 sm:px-6">
        {FILTERS.map((item) => (
          <button
            key={item.key}
            className={cn(
              "rounded-lg px-2.5 py-1 text-xs font-medium transition-colors",
              filter === item.key
                ? "bg-primary/10 text-primary"
                : "text-muted-foreground hover:bg-muted hover:text-foreground"
            )}
            onClick={() => setFilter(item.key)}
          >
            {item.label}
          </button>
        ))}
        {approvals.isFetching && (
          <Loader2 className="ml-auto h-3.5 w-3.5 animate-spin text-muted-foreground" />
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-4xl space-y-3 px-4 py-5 sm:px-6">
          {approvals.isError && (
            <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
              Failed to load approvals: {approvals.error.message}
            </div>
          )}

          {!approvals.isPending && items.length === 0 && (
            <div className="rounded-xl border border-dashed bg-card/40">
              <EmptyState
                icon={ShieldCheck}
                title={
                  filter === "pending"
                    ? "You're all caught up"
                    : "No approvals in this view"
                }
                description={
                  filter === "pending"
                    ? "When an agent asks to run a write action, it will appear here and in the conversation."
                    : "Try a different status filter."
                }
              />
            </div>
          )}

          {items.map((approval) => (
            <article
              key={approval.id}
              className="rounded-xl border bg-card p-4"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm font-medium">
                      {approval.tool}
                    </span>
                    <Badge
                      variant="secondary"
                      className={cn("text-[10px]", statusClass(approval.status))}
                    >
                      {approval.status}
                    </Badge>
                    <span className="text-[10px] text-muted-foreground">
                      Run #{approval.runId}
                    </span>
                  </div>
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    {approval.argsSummary || "No summary provided."}
                  </p>
                  <p className="text-[10px] text-muted-foreground">
                    Requested {formatTime(approval.createdAt)} · expires{" "}
                    {formatTime(approval.expiresAt)}
                  </p>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {approval.status === "pending" && (
                    <>
                      <Button
                        size="sm"
                        disabled={decide.isPending}
                        onClick={() =>
                          decide.mutate({ approval, action: "approve" })
                        }
                      >
                        {decide.isPending ? (
                          <Loader2 className="animate-spin" />
                        ) : (
                          <Check />
                        )}
                        Approve
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={decide.isPending}
                        onClick={() =>
                          decide.mutate({ approval, action: "reject" })
                        }
                      >
                        <X />
                        Reject
                      </Button>
                    </>
                  )}
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={contextLoading}
                    onClick={() => void openContext(approval)}
                  >
                    <MessageSquare />
                    Context
                    <ChevronRight />
                  </Button>
                </div>
              </div>
            </article>
          ))}
        </div>
      </div>

      <Dialog open={!!context} onOpenChange={(open) => !open && setContext(null)}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Run #{context?.id} context</DialogTitle>
          </DialogHeader>
          {context ? (
            <div className="space-y-3 text-sm">
              <dl className="grid grid-cols-2 gap-3 text-xs">
                <div>
                  <dt className="text-muted-foreground">Status</dt>
                  <dd className="mt-0.5">{context.status}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Agent</dt>
                  <dd className="mt-0.5">#{context.agentId}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Topic</dt>
                  <dd className="mt-0.5 truncate font-mono">{context.topic}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Tokens</dt>
                  <dd className="mt-0.5">{context.totalTokens}</dd>
                </div>
              </dl>
              {context.lastError && (
                <p className="rounded-lg border border-destructive/30 bg-destructive/5 p-2 text-xs text-destructive">
                  {context.lastError}
                </p>
              )}
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => setContext(null)}>
                  Close
                </Button>
                <Button
                  onClick={() =>
                    router.push(`/chat/${encodeURIComponent(context.topic)}`)
                  }
                >
                  <MessageSquare />
                  Open conversation
                </Button>
              </div>
            </div>
          ) : (
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
