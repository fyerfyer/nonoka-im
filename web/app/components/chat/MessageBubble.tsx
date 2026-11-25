"use client";

import { useState } from "react";
import { ChatMessage } from "@/types";
import { cn } from "@/lib/utils";
import { formatSize, parseMediaContent, MediaMeta } from "@/lib/media";
import { useAuthedMedia } from "@/hooks/useAuthedMedia";
import { segmentMentions, MentionMember } from "@/lib/mentions";
import { AgentApprovalCard, MSG_TYPE_AGENT_CARD } from "./AgentApprovalCard";
import { SourceChips } from "./SourceChips";
import { agentApi, type RunCitation } from "@/lib/api/agents";
import { toast } from "sonner";
import {
  Bot,
  Check,
  CheckCheck,
  Loader2,
  AlertCircle,
  Undo2,
  FileIcon,
  Copy,
  Square,
  Wrench,
  ShieldAlert,
  Download,
  RotateCcw,
  X,
} from "lucide-react";

const MSG_TYPE_IMAGE = 2;
const MSG_TYPE_FILE = 3;

interface MessageBubbleProps {
  message: ChatMessage;
  isMe: boolean;
  showSender?: boolean;
  senderName?: string;
  senderIsAgent?: boolean;
  members?: MentionMember[];
  currentUserId?: number;
  onRecall?: () => void;
  onRetry?: () => void;
  onJumpToCitation?: (citation: RunCitation) => void;
}

export function MessageBubble({
  message,
  isMe,
  showSender,
  senderName,
  senderIsAgent,
  members,
  currentUserId,
  onRecall,
  onRetry,
  onJumpToCitation,
}: MessageBubbleProps) {
  const time = formatTime(message.timestamp);
  const [cancelling, setCancelling] = useState(false);
  const [loadedCitations, setLoadedCitations] = useState<RunCitation[] | undefined>(undefined);
  const [loadingSources, setLoadingSources] = useState(false);
  const citations = message.citations ?? loadedCitations;

  const loadSources = async () => {
    if (!message.msgId || loadingSources) return;
    setLoadingSources(true);
    try {
      const run = await agentApi.runByReply(Number(message.msgId));
      setLoadedCitations(run.citations ?? []);
    } catch {
      setLoadedCitations([]);
    } finally {
      setLoadingSources(false);
    }
  };

  const cancelRun = async () => {
    if (!message.runId) return;
    setCancelling(true);
    try {
      await agentApi.cancelRun(message.runId);
      toast.success("Agent run cancelled");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Cancel failed");
    } finally {
      setCancelling(false);
    }
  };

  const canRecall = isMe && onRecall && message.status !== "recalled" && !message.recalled;
  const isMedia = message.msgType === MSG_TYPE_IMAGE || message.msgType === MSG_TYPE_FILE;
  const canCopy =
    !message.recalled &&
    message.msgType !== MSG_TYPE_IMAGE &&
    message.msgType !== MSG_TYPE_FILE &&
    message.msgType !== MSG_TYPE_AGENT_CARD;

  const copyText = async () => {
    try {
      await navigator.clipboard.writeText(message.content);
      toast.success("Copied to clipboard");
    } catch {
      toast.error("Copy failed");
    }
  };

  return (
    <div
      className={cn(
        "group relative flex w-full items-center",
        isMe ? "justify-end" : "justify-start"
      )}
    >
      <div
        className={cn(
          "relative flex max-w-[75%] flex-col",
          isMe ? "items-end" : "items-start"
        )}
      >
        {(canCopy || canRecall) && (
          <div
            className={cn(
              "absolute top-1/2 flex -translate-y-1/2 flex-col gap-0.5",
              isMe ? "-left-8" : "-right-8"
            )}
          >
            {canCopy && (
              <button
                className="rounded-full p-1 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 hover:bg-muted"
                title="Copy message"
                onClick={copyText}
              >
                <Copy className="h-3.5 w-3.5" />
              </button>
            )}
            {canRecall && (
              <button
                className="rounded-full p-1 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 hover:bg-muted"
                title="Recall message"
                onClick={onRecall}
              >
                <Undo2 className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        )}
        {showSender && senderName && (
          <span className="text-muted-foreground mb-1 flex items-center gap-1 px-1 text-xs">
            {senderIsAgent && <Bot className="h-3 w-3 text-primary" />}
            {senderName}
            {senderIsAgent && (
              <span className="rounded bg-primary/10 px-1 text-[9px] font-medium text-primary">
                AI
              </span>
            )}
          </span>
        )}
        <div
          title={new Date(message.timestamp * 1000).toLocaleString()}
          className={cn(
            "relative rounded-2xl px-4 py-2.5 text-sm shadow-sm transition-all",
            isMedia && "bg-transparent px-0 py-0 shadow-none",
            !isMedia &&
              (isMe
                ? "bg-primary text-primary-foreground rounded-br-md"
                : "bg-muted rounded-bl-md")
          )}
        >
          <MessageBody
            message={message}
            isMe={isMe}
            members={members}
            currentUserId={currentUserId}
          />
          {citations && citations.length > 0 && (
            <SourceChips citations={citations} onJump={onJumpToCitation} />
          )}
          {!citations && senderIsAgent && !message.streaming && message.msgId && (
            <button
              type="button"
              className="mt-1 text-[10px] text-muted-foreground underline-offset-2 hover:underline"
              onClick={loadSources}
              disabled={loadingSources}
            >
              {loadingSources ? "Loading sources…" : "Show sources"}
            </button>
          )}
          {message.streaming && message.streamStatus !== "waiting_approval" && (
            <StreamingStatus message={message} />
          )}
          {message.streamStatus === "waiting_approval" && message.approval && (
            <div className="mt-2 flex items-start gap-2 rounded-md border border-amber-300/60 bg-amber-50/60 p-2 text-xs dark:border-amber-500/30 dark:bg-amber-500/10">
              <ShieldAlert className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-600" />
              <div className="min-w-0">
                <p className="font-medium">Waiting for approval</p>
                <p className="truncate text-muted-foreground" title={message.approval.summary}>
                  {message.approval.tool} · {message.approval.summary || "Review the approval card"}
                </p>
              </div>
            </div>
          )}
          {message.streamStatus === "failed" && (
            <p className="mt-2 flex items-start gap-1 text-xs text-destructive">
              <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>{message.errorMessage || message.errorCode || "Agent run failed"}</span>
            </p>
          )}
          {message.streamStatus === "cancelled" && (
            <p className="mt-2 text-xs text-muted-foreground">Run cancelled.</p>
          )}
          <div
            className={cn(
              "mt-1 flex items-center gap-1 text-[10px] opacity-70",
              isMe ? "justify-end" : "justify-start"
            )}
          >
            <span>{time}</span>
            {isMe && (
              <StatusIcon status={message.status} onRetry={onRetry} />
            )}
            {message.streaming && message.runId && (
              <button
                type="button"
                title="Stop agent"
                className="ml-1 rounded p-0.5 hover:bg-background/30"
                onClick={cancelRun}
                disabled={cancelling}
              >
                {cancelling ? <Loader2 className="h-3 w-3 animate-spin" /> : <Square className="h-3 w-3" />}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function StreamingStatus({ message }: { message: ChatMessage }) {
  const tools = message.toolCalls || [];
  if (tools.length === 0) {
    if (message.content) return null;
    return (
      <p className="mt-2 flex items-center gap-1.5 text-xs text-muted-foreground">
        <Loader2 className="h-3 w-3 animate-spin" />
        Thinking…
      </p>
    );
  }
  return (
    <div className="mt-2 space-y-1 text-xs text-muted-foreground">
      {tools.map((tool, i) => (
        <div key={`${tool.toolCallId || tool.tool}-${i}`} className="flex items-center gap-1.5">
          {tool.status === "running" ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : tool.status === "error" ? (
            <AlertCircle className="h-3 w-3 text-destructive" />
          ) : (
            <Check className="h-3 w-3 text-emerald-600" />
          )}
          <Wrench className="h-3 w-3" />
          <span className="truncate">{tool.summary || tool.tool}</span>
        </div>
      ))}
    </div>
  );
}

// MessageBody renders text, image, and file message content. Media messages
// carry JSON metadata in content; malformed payloads fall back to plain text.
function MessageBody({
  message,
  isMe,
  members,
  currentUserId,
}: {
  message: ChatMessage;
  isMe: boolean;
  members?: MentionMember[];
  currentUserId?: number;
}) {
  if (message.recalled) {
    return (
      <p className="italic opacity-70">This message was recalled</p>
    );
  }

  if (message.msgType === MSG_TYPE_IMAGE || message.msgType === MSG_TYPE_FILE) {
    const meta = parseMediaContent(message.content);
    if (!meta) {
      return (
        <p className="whitespace-pre-wrap break-words">{message.content}</p>
      );
    }
    if (message.msgType === MSG_TYPE_IMAGE) {
      return <ImageMessage meta={meta} />;
    }
    return <FileMessage meta={meta} />;
  }

  if (message.msgType === MSG_TYPE_AGENT_CARD) {
    return <AgentApprovalCard content={message.content} />;
  }

  // Text: highlight @mentions of known group members; mentions of the
  // current user get an extra background emphasis.
  const segments = segmentMentions(message.content, members || []);
  return (
    <p className="whitespace-pre-wrap break-words">
      {segments.map((seg, i) =>
        seg.mentionedUserId !== undefined ? (
          <span
            key={i}
            className={cn(
              "font-semibold",
              isMe
                ? "underline decoration-primary-foreground/60 underline-offset-2"
                : "text-primary",
              seg.mentionedUserId === currentUserId &&
                (isMe
                  ? "rounded bg-white/25 px-0.5"
                  : "rounded bg-primary/15 px-0.5")
            )}
          >
            {seg.text}
          </span>
        ) : (
          <span key={i}>{seg.text}</span>
        )
      )}
    </p>
  );
}

function ImageMessage({ meta }: { meta: MediaMeta }) {
  const { src, loading, error, retry } = useAuthedMedia(meta.fileId ?? null, meta.url);
  const [open, setOpen] = useState(false);
  if (loading) {
    return <div className="h-40 w-56 animate-pulse rounded-xl bg-muted" />;
  }
  if (error || !src) {
    return (
      <div className="flex w-56 flex-col items-start gap-1 rounded-xl border border-dashed p-3 text-xs text-muted-foreground">
        <span className="flex items-center gap-1 text-destructive">
          <AlertCircle className="h-3.5 w-3.5" />
          Image failed to load
        </span>
        <span className="truncate" title={meta.name}>{meta.name}</span>
        <button className="flex items-center gap-1 hover:underline" onClick={retry}>
          <RotateCcw className="h-3 w-3" /> Retry
        </button>
      </div>
    );
  }
  return (
    <>
      <button type="button" className="block overflow-hidden rounded-xl" onClick={() => setOpen(true)} title={meta.name}>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src={src} alt={meta.name} className="max-h-64 max-w-full bg-muted object-contain" />
      </button>
      {open && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-6" onClick={() => setOpen(false)}>
          <button className="absolute right-5 top-5 rounded-full bg-black/50 p-2 text-white" onClick={() => setOpen(false)} title="Close">
            <X className="h-5 w-5" />
          </button>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src={src} alt={meta.name} className="max-h-full max-w-full rounded-lg object-contain" onClick={(e) => e.stopPropagation()} />
        </div>
      )}
    </>
  );
}

function FileMessage({ meta }: { meta: MediaMeta }) {
  const { src, loading, error, retry } = useAuthedMedia(meta.fileId ?? null, meta.url);
  if (loading) {
    return <div className="h-14 w-60 animate-pulse rounded-xl bg-muted" />;
  }
  if (error || !src) {
    return (
      <div className="flex w-60 flex-col gap-1 rounded-xl border border-dashed p-3 text-xs text-muted-foreground">
        <span className="flex items-center gap-1 text-destructive">
          <AlertCircle className="h-3.5 w-3.5" /> File unavailable
        </span>
        <span className="truncate" title={meta.name}>{meta.name}</span>
        <button className="flex items-center gap-1 hover:underline" onClick={retry}>
          <RotateCcw className="h-3 w-3" /> Retry
        </button>
      </div>
    );
  }
  return (
    <a
      href={src}
      download={meta.name}
      target="_blank"
      rel="noopener noreferrer"
      className="flex w-60 items-center gap-3 rounded-xl border bg-card p-2.5 text-left shadow-sm"
    >
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
        <FileIcon className="h-5 w-5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-xs font-medium">{meta.name}</span>
        <span className="block text-[10px] text-muted-foreground">{formatSize(meta.size)}</span>
      </span>
      <Download className="h-4 w-4 shrink-0 text-muted-foreground" />
    </a>
  );
}

function StatusIcon({
  status,
  onRetry,
}: {
  status: ChatMessage["status"];
  onRetry?: () => void;
}) {
  if (status === "sending") {
    return <Loader2 className="h-3 w-3 animate-spin" />;
  }
  if (status === "failed") {
    return (
      <button
        type="button"
        title="Send failed — click to retry"
        onClick={(e) => {
          e.stopPropagation();
          onRetry?.();
        }}
        className="cursor-pointer"
      >
        <AlertCircle className="h-3 w-3 text-destructive" />
      </button>
    );
  }
  if (status === "read") {
    return <CheckCheck className="h-3 w-3" />;
  }
  return <Check className="h-3 w-3" />;
}

function formatTime(timestamp: number): string {
  const date = new Date(timestamp * 1000);
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}
