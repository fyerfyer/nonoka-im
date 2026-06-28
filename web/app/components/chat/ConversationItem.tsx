"use client";

import { Conversation } from "@/types";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";
import { Users } from "lucide-react";

interface ConversationItemProps {
  conversation: Conversation;
  isActive?: boolean;
  onClick?: () => void;
}

export function ConversationItem({
  conversation,
  isActive,
  onClick,
}: ConversationItemProps) {
  const title = conversation.name || conversation.peerUsername || "Unknown";
  const preview = conversation.lastMsgPreview || "No messages yet";
  const time = conversation.lastMsgAt
    ? formatTime(conversation.lastMsgAt)
    : "";

  return (
    <button
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-3 rounded-xl px-3 py-3 text-left transition-colors",
        isActive
          ? "bg-primary/10"
          : "hover:bg-muted"
      )}
    >
      <Avatar className="h-12 w-12 shrink-0">
        <AvatarFallback
          className={cn(
            "text-sm font-semibold",
            conversation.type === "group"
              ? "bg-indigo-100 text-indigo-700"
              : "bg-primary/10 text-primary"
          )}
        >
          {conversation.type === "group" ? (
            <Users className="h-5 w-5" />
          ) : (
            title.slice(0, 2).toUpperCase()
          )}
        </AvatarFallback>
      </Avatar>
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center justify-between gap-2">
          <span
            className={cn(
              "truncate text-sm font-medium",
              isActive && "text-primary"
            )}
          >
            {title}
          </span>
          {time && (
            <span className="shrink-0 text-[10px] text-muted-foreground">
              {time}
            </span>
          )}
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="truncate text-xs text-muted-foreground">
            {preview}
          </span>
          {conversation.unreadCount > 0 && (
            <span className="flex h-5 min-w-[20px] shrink-0 items-center justify-center rounded-full bg-primary px-1.5 text-[10px] font-semibold text-primary-foreground">
              {conversation.unreadCount > 99
                ? "99+"
                : conversation.unreadCount}
            </span>
          )}
        </div>
      </div>
    </button>
  );
}

function formatTime(timestamp: number): string {
  const date = new Date(timestamp * 1000);
  const now = new Date();
  const isToday = date.toDateString() === now.toDateString();
  if (isToday) {
    return date.toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
    });
  }
  return date.toLocaleDateString([], { month: "short", day: "numeric" });
}
