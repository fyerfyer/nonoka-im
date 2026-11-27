"use client";

import { useState } from "react";
import { MessageSquarePlus, MessageSquareText, Users } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/workspace/EmptyState";
import { NewChatDialog } from "./NewChatDialog";
import { NewGroupDialog } from "./NewGroupDialog";

export function ChatEmptyState() {
  const [newChatOpen, setNewChatOpen] = useState(false);
  const [newGroupOpen, setNewGroupOpen] = useState(false);

  return (
    <>
      <div className="flex h-full flex-1 items-center justify-center bg-chat-canvas">
        <EmptyState
          icon={MessageSquareText}
          title="Select a conversation"
          description="Pick a conversation from the list, or start a direct message or group."
          action={
            <Button size="sm" onClick={() => setNewChatOpen(true)}>
              <MessageSquarePlus />
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
      </div>
      <NewChatDialog open={newChatOpen} onOpenChange={setNewChatOpen} />
      <NewGroupDialog open={newGroupOpen} onOpenChange={setNewGroupOpen} />
    </>
  );
}
