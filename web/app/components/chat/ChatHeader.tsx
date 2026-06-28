"use client";

import { Conversation, User, ConnectionState } from "@/types";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { ArrowLeft, MoreVertical, Users } from "lucide-react";
import { useRouter } from "next/navigation";
import { cn } from "@/lib/utils";

interface ChatHeaderProps {
  conversation?: Conversation;
  currentUser: User | null;
  connectionState: ConnectionState;
}

export function ChatHeader({
  conversation,
  currentUser,
  connectionState,
}: ChatHeaderProps) {
  const router = useRouter();

  const title = conversation
    ? conversation.name ||
      (conversation.type === "p2p"
        ? conversation.peerUsername || "Unknown"
        : "Group")
    : "Chat";

  const subtitle = getConnectionLabel(connectionState);

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
              connectionState === "authed"
                ? "text-emerald-600"
                : "text-muted-foreground"
            )}
          >
            {subtitle}
          </span>
        </div>
      </div>
      <Button variant="ghost" size="icon">
        <MoreVertical className="h-5 w-5" />
      </Button>
    </header>
  );
}

function getConnectionLabel(state: ConnectionState): string {
  switch (state) {
    case "connecting":
      return "Connecting...";
    case "connected":
      return "Authenticating...";
    case "authed":
      return "Online";
    case "reconnecting":
      return "Reconnecting...";
    case "disconnected":
      return "Offline";
    default:
      return "";
  }
}
