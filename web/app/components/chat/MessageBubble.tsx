"use client";

import { ChatMessage } from "@/types";
import { cn } from "@/lib/utils";
import { absoluteFileUrl, formatSize, parseMediaContent } from "@/lib/media";
import { segmentMentions, MentionMember } from "@/lib/mentions";
import { toast } from "sonner";
import {
  Check,
  CheckCheck,
  Loader2,
  AlertCircle,
  Undo2,
  FileIcon,
  Copy,
} from "lucide-react";

const MSG_TYPE_IMAGE = 2;
const MSG_TYPE_FILE = 3;

interface MessageBubbleProps {
  message: ChatMessage;
  isMe: boolean;
  showSender?: boolean;
  senderName?: string;
  members?: MentionMember[];
  currentUserId?: number;
  onRecall?: () => void;
  onRetry?: () => void;
}

export function MessageBubble({
  message,
  isMe,
  showSender,
  senderName,
  members,
  currentUserId,
  onRecall,
  onRetry,
}: MessageBubbleProps) {
  const time = formatTime(message.timestamp);

  const canRecall = isMe && onRecall && message.status !== "recalled" && !message.recalled;
  const canCopy =
    !message.recalled && message.msgType !== MSG_TYPE_IMAGE && message.msgType !== MSG_TYPE_FILE;

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
          <span className="text-muted-foreground mb-1 px-1 text-xs">
            {senderName}
          </span>
        )}
        <div
          title={new Date(message.timestamp * 1000).toLocaleString()}
          className={cn(
            "relative rounded-2xl px-4 py-2.5 text-sm shadow-sm transition-all",
            isMe
              ? "bg-primary text-primary-foreground rounded-br-md"
              : "bg-muted rounded-bl-md"
          )}
        >
          <MessageBody
            message={message}
            isMe={isMe}
            members={members}
            currentUserId={currentUserId}
          />
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
          </div>
        </div>
      </div>
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
    const href = absoluteFileUrl(meta.url);
    if (message.msgType === MSG_TYPE_IMAGE) {
      return (
        <a href={href} target="_blank" rel="noopener noreferrer" title={meta.name}>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={href}
            alt={meta.name}
            className="max-h-60 max-w-full rounded-lg"
          />
        </a>
      );
    }
    return (
      <a
        href={href}
        target="_blank"
        rel="noopener noreferrer"
        className="flex items-center gap-2 rounded-lg bg-background/60 px-1 py-1"
      >
        <FileIcon className="h-5 w-5 shrink-0" />
        <span className="min-w-0">
          <span className="block max-w-[200px] truncate text-sm underline underline-offset-2">
            {meta.name}
          </span>
          <span className="block text-[10px] opacity-70">
            {formatSize(meta.size)}
          </span>
        </span>
      </a>
    );
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
                "rounded bg-amber-200/80 px-0.5 text-amber-900 decoration-amber-900/60"
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
