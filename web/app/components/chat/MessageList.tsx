"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ChatMessage, Conversation, User } from "@/types";
import { MessageBubble } from "./MessageBubble";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { useChatStore } from "@/stores/chatStore";
import { useRunStreamStore } from "@/stores/runStreamStore";
import { useAgentNames } from "@/hooks/useAgentNames";
import { MentionMember } from "@/lib/mentions";

const NEAR_BOTTOM_PX = 100;
// Cap the "Agent is processing" hint so a silently-failed run never leaves
// a permanent spinner in the conversation.
const AGENT_PENDING_TIMEOUT_MS = 60_000;

interface MessageListProps {
  conversation?: Conversation;
  currentUser: User | null;
  loadingMore?: boolean;
  onLoadMore?: () => void;
  onRecall?: (message: ChatMessage) => void;
  onRetry?: (message: ChatMessage) => void;
  members?: MentionMember[];
}

export function MessageList({
  conversation,
  currentUser,
  loadingMore,
  onLoadMore,
  onRecall,
  onRetry,
  members,
}: MessageListProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const messages = useMemo(
    () => conversation?.messages || [],
    [conversation?.messages]
  );
  const memberNames = useChatStore((s) =>
    conversation?.type === "group"
      ? s.memberNames.get(conversation.topic)
      : undefined
  );
  const agentNames = useAgentNames();
  const agentTyping = useRunStreamStore((s) =>
    conversation?.topic ? s.typingTopics.get(conversation.topic) : undefined
  );

  // After the user messages a bot, show a lightweight "working" hint until
  // the bot replies or the cap elapses. The timeout flag is only mutated
  // from timer callbacks to keep effects free of synchronous setState.
  const lastMsg = messages[messages.length - 1];
  const ownAwaitingReply =
    !!conversation?.peerIsBot &&
    !!currentUser &&
    !!lastMsg &&
    lastMsg.senderId === currentUser.userId &&
    lastMsg.status !== "failed";
  const [replyTimedOut, setReplyTimedOut] = useState(false);
  useEffect(() => {
    if (!ownAwaitingReply) return;
    const reset = setTimeout(() => setReplyTimedOut(false), 0);
    const expire = setTimeout(
      () => setReplyTimedOut(true),
      AGENT_PENDING_TIMEOUT_MS
    );
    return () => {
      clearTimeout(reset);
      clearTimeout(expire);
    };
  }, [ownAwaitingReply, lastMsg?.clientMsgId]);
  const agentPending = ownAwaitingReply && !replyTimedOut;

  // Track whether the user is already near the bottom; only then do new
  // messages pull the view down.
  const nearBottomRef = useRef(true);
  // Bookmarks for scroll anchoring across prepends (history loads).
  const prevFirstKeyRef = useRef<string | undefined>(undefined);
  const prevLastKeyRef = useRef<string | undefined>(undefined);
  const prevCountRef = useRef(0);
  const prevHeightRef = useRef(0);

  const getViewport = () =>
    scrollRef.current?.querySelector<HTMLElement>(
      "[data-slot='scroll-area-viewport']"
    ) ?? null;

  useEffect(() => {
    const el = getViewport();
    if (!el) return;
    const onScroll = () => {
      nearBottomRef.current =
        el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX;
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);

  useLayoutEffect(() => {
    const el = getViewport();
    const firstKey = messages[0]?.clientMsgId;
    const lastKey = messages[messages.length - 1]?.clientMsgId;

    if (el) {
      const hadContent = prevCountRef.current > 0;
      const prepended =
        hadContent &&
        firstKey !== prevFirstKeyRef.current &&
        messages.length > prevCountRef.current;
      const lastChanged = lastKey !== prevLastKeyRef.current;
      const lastMsg = messages[messages.length - 1];
      const isOwn =
        !!lastMsg && !!currentUser && Number(lastMsg.senderId) === Number(currentUser.userId);
      const isStreaming = !!lastMsg?.streaming;

      if (prepended) {
        // History was prepended: keep the viewport anchored on the
        // previously visible content instead of jumping.
        el.scrollTop += el.scrollHeight - prevHeightRef.current;
      } else if (lastChanged && (nearBottomRef.current || isOwn || isStreaming)) {
        bottomRef.current?.scrollIntoView({ behavior: isStreaming ? "auto" : "smooth" });
        nearBottomRef.current = true;
      } else if (isStreaming && nearBottomRef.current) {
        bottomRef.current?.scrollIntoView({ behavior: "auto" });
      }
    }

    prevFirstKeyRef.current = firstKey;
    prevLastKeyRef.current = lastKey;
    prevCountRef.current = messages.length;
    if (el) prevHeightRef.current = el.scrollHeight;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messages]);

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
              const sender = showSender
                ? resolveSender(msg.senderId, conversation, memberNames, agentNames)
                : undefined;
              return (
                <MessageBubble
                  key={msg.clientMsgId}
                  message={msg}
                  isMe={isMe}
                  showSender={showSender}
                  senderName={sender?.name}
                  senderIsAgent={sender?.isAgent}
                  members={members}
                  currentUserId={
                    currentUser ? Number(currentUser.userId) : undefined
                  }
                  onRecall={() => onRecall?.(msg)}
                  onRetry={() => onRetry?.(msg)}
                />
              );
            })}
          </div>
        ))}
        {(agentPending || !!agentTyping) && (
          <div className="flex items-center gap-2 px-1 py-1 text-xs text-muted-foreground">
            <Loader2 className="h-3 w-3 animate-spin" />
            Agent is processing…
          </div>
        )}
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

function resolveSender(
  senderId: number,
  conversation: Conversation,
  memberNames: Map<number, string> | undefined,
  agentNames: Map<number, { name: string }>
): { name: string; isAgent: boolean } {
  // Bot users resolve to their agent display name; member tables only carry
  // the raw "agent_xxx" username.
  const agent = agentNames.get(Number(senderId));
  if (agent) return { name: agent.name, isAgent: true };
  if (conversation.type === "group") {
    // senderId may be a proto-JSON string at runtime; coerce for the lookup.
    const name = memberNames?.get(Number(senderId)) || "Unknown";
    return { name, isAgent: false };
  }
  if (senderId === conversation.peerId) {
    return { name: conversation.peerUsername || "Unknown", isAgent: false };
  }
  return { name: "Unknown", isAgent: false };
}
