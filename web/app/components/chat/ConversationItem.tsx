"use client";

import { Conversation } from "@/types";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { Users, AtSign, Bot } from "lucide-react";
import { useChatStore } from "@/stores/chatStore";
import { useAgentNames } from "@/hooks/useAgentNames";

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
  const mentioned = useChatStore((s) =>
    s.mentionedTopics.has(conversation.topic)
  );
  const agentNames = useAgentNames();
  // The server conversation list does not flag bot peers, so resolve them
  // against the agent directory: p2p conversations with a bot peer take the
  // agent's display name (and the AI badge) over the raw bot username.
  const agentName =
    conversation.type === "p2p"
      ? agentNames.get(conversation.peerId)?.name
      : undefined;
  const title =
    agentName || conversation.name || conversation.peerUsername || "Unknown";
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
              ? "bg-primary/10 text-primary"
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
          <span className="flex min-w-0 items-center gap-1.5">
            <span
              className={cn(
                "truncate text-sm font-medium",
                isActive && "text-primary"
              )}
            >
              {title}
            </span>
            {(conversation.peerIsBot || agentName) && (
              <Badge variant="secondary" className="px-1.5 text-[10px]">
                <Bot className="h-2.5 w-2.5" />
                AI
              </Badge>
            )}
          </span>
          {time && (
            <span className="shrink-0 text-[10px] text-muted-foreground">
              {time}
            </span>
          )}
        </div>
        <div className="flex items-center justify-between gap-2">
          <span
            className={cn(
              "truncate text-xs",
              mentioned ? "font-medium text-foreground" : "text-muted-foreground"
            )}
          >
            {mentioned && (
              <span className="mr-1 inline-flex items-center rounded bg-warning/15 px-1 py-px text-[9px] font-semibold text-warning">
                <AtSign className="mr-px h-2.5 w-2.5" />
                Mentioned
              </span>
            )}
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
