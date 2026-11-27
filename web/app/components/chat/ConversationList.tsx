"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { MessageSquare, Plus, Users } from "lucide-react";
import { useConversations } from "@/hooks/useConversations";
import { useChatStore } from "@/stores/chatStore";
import { ConversationItem } from "./ConversationItem";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { NewChatDialog } from "./NewChatDialog";
import { NewGroupDialog } from "./NewGroupDialog";
import { ConnectionBar } from "./ConnectionBar";
import { EmptyState } from "@/components/workspace/EmptyState";

export function ConversationList() {
  const chatStore = useChatStore();
  const router = useRouter();
  const params = useParams();
  useConversations();
  const [newChatOpen, setNewChatOpen] = useState(false);
  const [newGroupOpen, setNewGroupOpen] = useState(false);

  const activeTopic =
    typeof params?.topic === "string" ? params.topic : null;

  const conversations = Array.from(chatStore.conversations.values()).sort(
    (a, b) => (b.lastMsgAt || 0) - (a.lastMsgAt || 0)
  );

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-chat-list">
      <ConnectionBar />
      <div className="flex h-14 shrink-0 items-center justify-between border-b px-4">
        <div>
          <h1 className="text-sm font-semibold">Messages</h1>
          <p className="text-[11px] text-muted-foreground">
            {conversations.length === 0
              ? "No conversations yet"
              : `${conversations.length} conversation${conversations.length === 1 ? "" : "s"}`}
          </p>
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            title="New group"
            aria-label="New group"
            onClick={() => setNewGroupOpen(true)}
          >
            <Users className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            title="New message"
            aria-label="New message"
            onClick={() => setNewChatOpen(true)}
          >
            <Plus className="h-4 w-4" />
          </Button>
        </div>
      </div>

      <ScrollArea className="min-h-0 flex-1 px-2 py-2">
        {conversations.length === 0 ? (
          <EmptyState
            icon={MessageSquare}
            title="No conversations yet"
            description="Start a direct message or create a group to bring people and agents together."
            action={
              <Button size="sm" onClick={() => setNewChatOpen(true)}>
                <Plus />
                New message
              </Button>
            }
            secondaryAction={
              <Button
                size="sm"
                variant="outline"
                onClick={() => setNewGroupOpen(true)}
              >
                <Users />
                New group
              </Button>
            }
          />
        ) : (
          <div className="flex flex-col gap-1">
            {conversations.map((conv) => (
              <ConversationItem
                key={conv.topic}
                conversation={conv}
                isActive={conv.topic === activeTopic}
                onClick={() =>
                  router.push(`/chat/${encodeURIComponent(conv.topic)}`)
                }
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
