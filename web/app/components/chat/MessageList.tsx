"use client";

import { useEffect, useRef } from "react";
import { ChatMessage, Conversation, User } from "@/types";
import { MessageBubble } from "./MessageBubble";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";

interface MessageListProps {
  conversation?: Conversation;
  currentUser: User | null;
  loadingMore?: boolean;
  onLoadMore?: () => void;
}

export function MessageList({
  conversation,
  currentUser,
  loadingMore,
  onLoadMore,
}: MessageListProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const messages = conversation?.messages || [];

  useEffect(() => {
    if (bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [messages.length]);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el || !onLoadMore) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && !loadingMore) {
          onLoadMore();
        }
      },
      { root: el, threshold: 0 }
    );

    const topSentinel = document.createElement("div");
    el.firstElementChild?.prepend(topSentinel);
    observer.observe(topSentinel);

    return () => {
      observer.disconnect();
      topSentinel.remove();
    };
  }, [onLoadMore, loadingMore, conversation?.topic]);

  const groupedMessages = groupByDate(messages);

  if (!conversation) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-center">
        <p className="text-muted-foreground">Select a conversation</p>
      </div>
    );
  }

  return (
    <ScrollArea className="h-full" ref={scrollRef}>
      <div className="flex flex-col gap-1 px-4 py-4">
        {loadingMore && (
          <div className="flex justify-center py-2">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        )}
        {groupedMessages.map(([dateLabel, msgs], groupIdx) => (
          <div key={dateLabel + groupIdx} className="flex flex-col gap-3">
            <div className="flex justify-center py-2">
              <span className="rounded-full bg-muted px-3 py-1 text-[10px] font-medium text-muted-foreground">
                {dateLabel}
              </span>
            </div>
            {msgs.map((msg, idx) => {
              const isMe = msg.senderId === currentUser?.userId;
              const prev = msgs[idx - 1];
              const showSender =
                conversation.type === "group" &&
                !isMe &&
                (!prev || prev.senderId !== msg.senderId);
              return (
                <MessageBubble
                  key={msg.clientMsgId}
                  message={msg}
                  isMe={isMe}
                  showSender={showSender}
                  senderName={
                    showSender
                      ? getSenderName(msg.senderId, conversation)
                      : undefined
                  }
                />
              );
            })}
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </ScrollArea>
  );
}

function groupByDate(messages: ChatMessage[]): [string, ChatMessage[]][] {
  const groups = new Map<string, ChatMessage[]>();
  for (const msg of messages) {
    const date = new Date(msg.timestamp * 1000).toLocaleDateString();
    if (!groups.has(date)) {
      groups.set(date, []);
    }
    groups.get(date)!.push(msg);
  }
  return Array.from(groups.entries());
}

function getSenderName(
  senderId: number,
  conversation: Conversation
): string {
  if (senderId === conversation.peerId) {
    return conversation.peerUsername || "Unknown";
  }
  return "Unknown";
}
