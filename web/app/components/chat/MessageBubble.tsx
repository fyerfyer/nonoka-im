"use client";

import { ChatMessage } from "@/types";
import { cn } from "@/lib/utils";
import { Check, CheckCheck, Loader2, AlertCircle, Undo2 } from "lucide-react";

interface MessageBubbleProps {
  message: ChatMessage;
  isMe: boolean;
  showSender?: boolean;
  senderName?: string;
  onRecall?: () => void;
}

export function MessageBubble({
  message,
  isMe,
  showSender,
  senderName,
  onRecall,
}: MessageBubbleProps) {
  const time = formatTime(message.timestamp);

  const canRecall = isMe && onRecall && message.status !== "recalled" && !message.recalled;

  return (
    <div
      className={cn(
        "group flex w-full items-center",
        isMe ? "justify-end" : "justify-start"
      )}
    >
      {canRecall && (
        <button
          className="mr-1 rounded-full p-1 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 hover:bg-muted"
          title="Recall message"
          onClick={onRecall}
        >
          <Undo2 className="h-3.5 w-3.5" />
        </button>
      )}
      <div
        className={cn(
          "flex max-w-[75%] flex-col",
          isMe ? "items-end" : "items-start"
        )}
      >
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
          <p className={cn("whitespace-pre-wrap break-words", message.recalled && "italic opacity-70") }>
            {message.recalled ? "This message was recalled" : message.content}
          </p>
          <div
            className={cn(
              "mt-1 flex items-center gap-1 text-[10px] opacity-70",
              isMe ? "justify-end" : "justify-start"
            )}
          >
            <span>{time}</span>
            {isMe && <StatusIcon status={message.status} />}
          </div>
        </div>
      </div>
    </div>
  );
}

function StatusIcon({ status }: { status: ChatMessage["status"] }) {
  if (status === "sending") {
    return <Loader2 className="h-3 w-3 animate-spin" />;
  }
  if (status === "failed") {
    return <AlertCircle className="h-3 w-3 text-destructive" />;
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
