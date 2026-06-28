"use client";

import { useConversations } from "@/hooks/useConversations";
import { useChatStore } from "@/stores/chatStore";
import { useAuthStore } from "@/stores/authStore";
import { ConversationItem } from "./ConversationItem";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Plus, MessageSquare, Loader2 } from "lucide-react";
import { useRouter, useParams } from "next/navigation";
import { useState } from "react";
import { NewChatDialog } from "./NewChatDialog";
import { NewGroupDialog } from "./NewGroupDialog";
import { ConnectionBar } from "./ConnectionBar";
import { cn } from "@/lib/utils";

export function ConversationList() {
  const user = useAuthStore((s) => s.user);
  const chatStore = useChatStore();
  const router = useRouter();
  const params = useParams();
  const { refresh } = useConversations();
  const [newChatOpen, setNewChatOpen] = useState(false);
  const [newGroupOpen, setNewGroupOpen] = useState(false);

  const activeTopic =
    typeof params?.topic === "string" ? params.topic : null;

  const conversations = Array.from(chatStore.conversations.values()).sort(
    (a, b) => (b.lastMsgAt || 0) - (a.lastMsgAt || 0)
  );

  const initials = user?.username.slice(0, 2).toUpperCase() || "?";

  return (
    <div
      className={cn(
        "flex h-full w-full flex-col border-r bg-background md:w-80 lg:w-96",
        activeTopic && "hidden md:flex"
      )}
    >
      <ConnectionBar />
      <div className="flex h-14 items-center justify-between border-b px-4">
        <div className="flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-xs font-bold text-primary-foreground">
            {initials}
          </div>
          <span className="text-sm font-semibold">Messages</span>
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            onClick={() => setNewGroupOpen(true)}
          >
            <MessageSquare className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            onClick={() => setNewChatOpen(true)}
          >
            <Plus className="h-4 w-4" />
          </Button>
        </div>
      </div>

      <ScrollArea className="flex-1 px-2 py-2">
        {conversations.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 p-8 text-center">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-muted">
              <MessageSquare className="h-6 w-6 text-muted-foreground" />
            </div>
            <p className="text-sm text-muted-foreground">No conversations</p>
            <Button variant="outline" size="sm" onClick={() => setNewChatOpen(true)}>
              Start chatting
            </Button>
          </div>
        ) : (
          <div className="flex flex-col gap-1">
            {conversations.map((conv) => (
              <ConversationItem
                key={conv.topic}
                conversation={conv}
                isActive={conv.topic === activeTopic}
                onClick={() => router.push(`/chat/${encodeURIComponent(conv.topic)}`)}
              />
            ))}
          </div>
        )}
      </ScrollArea>

      <NewChatDialog open={newChatOpen} onOpenChange={setNewChatOpen} />
      <NewGroupDialog open={newGroupOpen} onOpenChange={setNewGroupOpen} />
    </div>
  );
}
