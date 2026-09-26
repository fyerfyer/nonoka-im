"use client";

import { useState } from "react";
import { Conversation, ConnectionState } from "@/types";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { GroupMembersDialog } from "./GroupMembersDialog";
import { useChatStore } from "@/stores/chatStore";
import { ArrowLeft, MoreVertical, Users } from "lucide-react";
import { useRouter } from "next/navigation";
import { cn } from "@/lib/utils";

interface ChatHeaderProps {
  conversation?: Conversation;
  connectionState: ConnectionState;
  peerOnline?: boolean;
}

export function ChatHeader({
  conversation,
  connectionState,
  peerOnline,
}: ChatHeaderProps) {
  const router = useRouter();
  const [membersOpen, setMembersOpen] = useState(false);
  const group = useChatStore((s) =>
    conversation?.type === "group" ? s.groups.get(conversation.topic) : undefined
  );

  const title = conversation
    ? conversation.name ||
      (conversation.type === "p2p"
        ? conversation.peerUsername || "Unknown"
        : "Group")
    : "Chat";

  const subtitle = getSubtitle(conversation, connectionState, peerOnline);
  const isPeerOnline =
    conversation?.type === "p2p" &&
    connectionState === "authed" &&
    peerOnline === true;

  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b bg-background/95 px-3 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="flex items-center gap-3">
        <Button
          variant="ghost"
          size="icon"
          className="md:hidden"
          onClick={() => router.push("/chat")}
        >
          <ArrowLeft className="h-5 w-5" />
        </Button>
        <Avatar className="h-9 w-9">
          <AvatarFallback
            className={cn(
              "text-sm font-semibold",
              conversation?.type === "group"
                ? "bg-indigo-100 text-indigo-700"
                : "bg-primary/10 text-primary"
            )}
          >
            {conversation?.type === "group" ? (
              <Users className="h-4 w-4" />
            ) : (
              title.slice(0, 2).toUpperCase()
            )}
          </AvatarFallback>
        </Avatar>
        <div className="flex flex-col">
          <h2 className="max-w-[180px] truncate text-sm font-semibold sm:max-w-xs">
            {title}
          </h2>
          <span
            className={cn(
              "text-xs",
              isPeerOnline
                ? "text-emerald-600"
                : "text-muted-foreground"
            )}
          >
            {subtitle}
          </span>
        </div>
      </div>
      {conversation?.type === "group" && (
        <>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" title="Group options">
                <MoreVertical className="h-5 w-5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setMembersOpen(true)}>
                <Users className="h-4 w-4" />
                Members
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          {group && (
            <GroupMembersDialog
              group={group}
              open={membersOpen}
              onOpenChange={setMembersOpen}
            />
          )}
        </>
      )}
    </header>
  );
}

function getSubtitle(
  conversation: Conversation | undefined,
  state: ConnectionState,
  peerOnline: boolean | undefined
): string {
  if (state !== "authed") return getConnectionLabel(state);
  if (conversation?.type === "group") return "Group conversation";
  if (peerOnline === undefined) return "Checking status...";
  return peerOnline ? "Online" : "Offline";
}

function getConnectionLabel(state: ConnectionState): string {
  switch (state) {
    case "connecting":
      return "Connecting...";
    case "connected":
      return "Authenticating...";
    case "authed":
      return "Connected";
    case "reconnecting":
      return "Reconnecting...";
    case "disconnected":
      return "Offline";
    default:
      return "";
  }
}
