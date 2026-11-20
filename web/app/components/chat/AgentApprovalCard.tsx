"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Bot, Check, Loader2, ShieldAlert, X } from "lucide-react";
import { agentApi, AppError } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const MSG_TYPE_AGENT_CARD = 5;

export interface AgentCardContent {
  version: number;
  run_id: number;
  approval_id: number;
  tool: string;
  summary: string;
  target: string;
  status: string;
  requested_by: number;
  expires_at: number;
}

// parseAgentCardContent decodes the card payload. History pulls normalize
// content to a JSON string already; the base64 branch covers payloads that
// reached us undecoded (same encoding family as other message types).
export function parseAgentCardContent(raw: string): AgentCardContent | null {
  try {
    const parsed = JSON.parse(raw) as AgentCardContent;
    return isCard(parsed) ? parsed : null;
  } catch {
    // fall through to base64 attempt
  }
  try {
    const decoded = atob(raw);
    const parsed = JSON.parse(decoded) as AgentCardContent;
    return isCard(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function isCard(value: AgentCardContent): boolean {
  return (
    !!value &&
    typeof value === "object" &&
    typeof value.approval_id === "number" &&
    typeof value.tool === "string"
  );
}

const statusStyles: Record<string, string> = {
  pending: "bg-amber-100 text-amber-800 dark:bg-amber-500/15 dark:text-amber-400",
  approved: "bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-400",
  rejected: "bg-red-100 text-red-800 dark:bg-red-500/15 dark:text-red-400",
};

export function AgentApprovalCard({ content }: { content: string }) {
  const card = parseAgentCardContent(content);
  const [overrideStatus, setOverrideStatus] = useState<string | null>(null);
  const [busy, setBusy] = useState<"approve" | "reject" | null>(null);
  // Expiry is evaluated against a clock. The render-purity lint forbids
  // Date.now() in render, so keep a timestamp state that timers own; pending
  // cards render their Approve/Reject buttons from the first tick onwards.
  const [nowTs, setNowTs] = useState(0);
  useEffect(() => {
    const initial = setTimeout(() => setNowTs(Date.now()), 0);
    const interval = setInterval(() => setNowTs(Date.now()), 15000);
    return () => {
      clearTimeout(initial);
      clearInterval(interval);
    };
  }, []);

  // Cards are plain IM messages: their embedded status is frozen at send
  // time. Hydrate pending cards from the approvals API so a card acted on
  // from another session (or before a reload) reflects the real outcome
  // instead of drifting to a misleading "expired".
  const [serverStatus, setServerStatus] = useState<string | null>(null);
  useEffect(() => {
    if (!card || card.status !== "pending") return;
    let active = true;
    agentApi
      .approvalDetail(card.approval_id)
      .then((detail) => {
        if (active && detail.status && detail.status !== "pending") {
          setServerStatus(detail.status);
        }
      })
      .catch(() => {
        // Degrade to the embedded status on network/permission errors.
      });
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [card?.approval_id]);

  if (!card) {
    return (
      <p className="whitespace-pre-wrap break-words italic opacity-70">
        [Agent approval card — unable to parse payload]
      </p>
    );
  }

  const status = overrideStatus ?? serverStatus ?? card.status;
  const expired =
    nowTs > 0 && card.expires_at > 0 && card.expires_at * 1000 <= nowTs;
  const actionable = status === "pending" && nowTs > 0 && !expired;

  const act = async (action: "approve" | "reject") => {
    setBusy(action);
    try {
      const reply =
        action === "approve"
          ? await agentApi.approveApproval(card.approval_id)
          : await agentApi.rejectApproval(card.approval_id);
      setOverrideStatus(reply.status || (action === "approve" ? "approved" : "rejected"));
      toast.success(`Approval ${reply.status || (action === "approve" ? "approved" : "rejected")}`);
    } catch (err) {
      if (err instanceof AppError && (err.status === 401 || err.status === 403)) {
        toast.error(`Approval failed: ${err.message}`);
      } else {
        toast.error(err instanceof Error ? err.message : "Approval failed");
      }
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="w-64 rounded-lg border bg-background/80 p-3 text-foreground sm:w-72">
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="flex items-center gap-1.5 text-xs font-semibold">
          <Bot className="h-3.5 w-3.5 text-primary" />
          Agent approval
        </span>
        <Badge
          variant="secondary"
          className={cn("px-1.5 text-[10px]", statusStyles[status])}
        >
          {expired && status === "pending" ? "expired" : status}
        </Badge>
      </div>
      <dl className="space-y-1 text-xs">
        <div className="flex gap-2">
          <dt className="shrink-0 text-muted-foreground">Tool</dt>
          <dd className="break-all font-mono">{card.tool || "-"}</dd>
        </div>
        {card.summary && (
          <div className="flex gap-2">
            <dt className="shrink-0 text-muted-foreground">Summary</dt>
            <dd className="break-words">{card.summary}</dd>
          </div>
        )}
        {card.target && (
          <div className="flex gap-2">
            <dt className="shrink-0 text-muted-foreground">Target</dt>
            <dd className="truncate" title={card.target}>{card.target}</dd>
          </div>
        )}
        <div className="flex gap-2">
          <dt className="shrink-0 text-muted-foreground">Expires</dt>
          <dd>
            {card.expires_at > 0
              ? new Date(card.expires_at * 1000).toLocaleString()
              : "-"}
          </dd>
        </div>
      </dl>
      {expired && status === "pending" && (
        <p className="mt-2 flex items-center gap-1 text-[11px] text-muted-foreground">
          <ShieldAlert className="h-3 w-3" />
          This approval request has expired.
        </p>
      )}
      {actionable ? (
        <div className="mt-3 flex gap-2">
          <Button
            size="xs"
            disabled={busy !== null}
            onClick={() => act("approve")}
          >
            {busy === "approve" ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <Check />
            )}
            Approve
          </Button>
          <Button
            size="xs"
            variant="outline"
            disabled={busy !== null}
            onClick={() => act("reject")}
          >
            {busy === "reject" ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <X />
            )}
            Reject
          </Button>
        </div>
      ) : (
        <p className="mt-3 text-[11px] text-muted-foreground">
          {status === "approved" && "Approved — the agent run will resume."}
          {status === "rejected" && "Rejected — the agent run was stopped."}
        </p>
      )}
    </div>
  );
}

export { MSG_TYPE_AGENT_CARD };
