"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Check, Loader2, ShieldAlert, X } from "lucide-react";
import { agentApi, type AgentApproval } from "@/lib/api/agents";
import { approvalsQuery, agentKeys } from "@/lib/queries/agents";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { formatTime } from "./agent-utils";

export function AgentApprovalInbox({ compact = false }: { compact?: boolean }) {
  const queryClient = useQueryClient();
  const approvals = useQuery(approvalsQuery("pending"));

  const decide = useMutation({
    mutationFn: async ({ approval, action }: { approval: AgentApproval; action: "approve" | "reject" }) => {
      if (action === "approve") await agentApi.approveApproval(approval.id);
      else await agentApi.rejectApproval(approval.id);
      return { approval, action };
    },
    onSuccess: ({ action }) => {
      toast.success(action === "approve" ? "Approval granted" : "Approval rejected");
      queryClient.invalidateQueries({ queryKey: agentKeys.approvals("pending") });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "Action failed"),
  });

  const items = approvals.data?.items ?? [];

  return (
    <div className="rounded-xl border bg-card p-4">
      <div className="flex items-center justify-between gap-2">
        <h3 className="flex items-center gap-1.5 text-sm font-semibold">
          <ShieldAlert className="h-4 w-4 text-amber-600" />
          Pending approvals
        </h3>
        <Badge variant="secondary">{items.length}</Badge>
      </div>
      {!compact && (
        <p className="mt-1 text-xs text-muted-foreground">
          Tool calls waiting for your decision. Cards in chat mirror this inbox.
        </p>
      )}
      <div className="mt-3 space-y-2">
        {approvals.isPending && (
          <Loader2 className="mx-auto h-4 w-4 animate-spin text-muted-foreground" />
        )}
        {!approvals.isPending && items.length === 0 && (
          <p className="py-3 text-center text-xs text-muted-foreground">
            Nothing waiting for you.
          </p>
        )}
        {items.slice(0, compact ? 3 : 10).map((approval) => (
          <div key={approval.id} className="rounded-lg border p-2.5 text-xs">
            <div className="flex items-center justify-between gap-2">
              <span className="font-mono font-medium">{approval.tool}</span>
              <span className="text-[10px] text-muted-foreground">
                {formatTime(approval.expiresAt)}
              </span>
            </div>
            <p className="mt-1 line-clamp-2 text-muted-foreground">
              {approval.argsSummary}
            </p>
            <div className="mt-2 flex gap-1.5">
              <Button
                size="xs"
                disabled={decide.isPending}
                onClick={() => decide.mutate({ approval, action: "approve" })}
              >
                {decide.isPending ? <Loader2 className="animate-spin" /> : <Check />}
                Approve
              </Button>
              <Button
                size="xs"
                variant="outline"
                disabled={decide.isPending}
                onClick={() => decide.mutate({ approval, action: "reject" })}
              >
                <X />
                Reject
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
