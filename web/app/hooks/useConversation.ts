import { useCallback, useEffect, useRef, useState } from "react";
import { messageApi, conversationApi } from "@/lib/api";
import { realtimeClient } from "@/lib/realtime";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import { ChatMessage, Conversation, User } from "@/types";
import { toast } from "sonner";

export function useConversation(topic: string | null) {
  const user = useAuthStore((s) => s.user);
  const [loadingMore, setLoadingMore] = useState(false);
  const loadedRef = useRef<Set<string>>(new Set());
  // Subscribe to the conversation object so status updates trigger re-renders
  const conversation = useChatStore(
    useCallback(
      (s) => (topic ? s.conversations.get(topic) : undefined),
      [topic]
    )
  );

  const loadMessages = useCallback(
    async (lastSeq = 0) => {
      if (!topic || !user) return;
      useChatStore.getState().setMessagesLoading(topic, true);
      try {
        const reply = await messageApi.pullMessages(topic, lastSeq, 20);
        const msgs: ChatMessage[] = reply.messages.map((m) => ({
          clientMsgId: m.clientMsgId || `${m.topicSeq}-${m.msgId}`,
          msgId: m.msgId,
          topic: m.topic,
          senderId: m.senderId,
          content: decodeMessageContent(m.content),
          timestamp: m.timestamp,
          topicSeq: m.topicSeq,
          status: m.senderId === user.userId ? "sent" : "delivered",
        }));

        if (lastSeq === 0) {
          useChatStore.getState().upsertConversation({
            topic,
            messages: msgs,
            hasMore: reply.hasMore,
          });
        } else {
          useChatStore.getState().prependMessages(topic, msgs);
        }
      } catch (err) {
        toast.error(
          `Failed to load messages: ${
            err instanceof Error ? err.message : "unknown error"
          }`
        );
      } finally {
        useChatStore.getState().setMessagesLoading(topic, false);
        loadedRef.current.add(topic);
      }
    },
    [topic, user]
  );

  const loadMore = useCallback(async () => {
    if (!conversation || !topic || conversation.isLoading || !conversation.hasMore)
      return;
    setLoadingMore(true);
    const oldest = conversation.messages[0]?.topicSeq || 0;
    await loadMessages(oldest > 0 ? oldest - 1 : 0);
    setLoadingMore(false);
  }, [conversation, topic, loadMessages]);

  const sendMessage = useCallback(
    (text: string) => {
      if (!topic || !text.trim()) return;
      const { clientMsgId } = realtimeClient.sendText(topic, text.trim());
      const msg: ChatMessage = {
        clientMsgId,
        topic,
        senderId: user?.userId || 0,
        content: text.trim(),
        timestamp: Date.now() / 1000,
        status: "sending",
      };
      useChatStore.getState().appendMessage(topic, msg);
    },
    [topic, user]
  );

  const markRead = useCallback(() => {
    if (!topic || !conversation) return;
    const upToSeq = conversation.lastSeq;
    if (upToSeq === 0) return;
    if (conversation.unreadCount > 0) {
      conversationApi.markRead(topic).catch(() => {
        // ignore
      });
      realtimeClient.sendReadReceipt(topic, upToSeq);
      useChatStore.getState().markTopicRead(topic, upToSeq);
    }
  }, [topic, conversation]);

  useEffect(() => {
    if (!topic) return;
    useChatStore.getState().setActiveTopic(topic);
    if (!loadedRef.current.has(topic)) {
      loadMessages();
    }
    markRead();
    return () => {
      useChatStore.getState().setActiveTopic(null);
    };
  }, [topic, loadMessages, markRead]);

  return {
    conversation,
    loadingMore,
    loadMore,
    sendMessage,
    markRead,
    recallMessage: (msg: ChatMessage) => { if (topic && msg.topicSeq) realtimeClient.recallMessage(topic, msg.topicSeq, msg.msgId); },
  };
}

function decodeMessageContent(content: string | Uint8Array | unknown): string {
  if (content instanceof Uint8Array) {
    return new TextDecoder().decode(content);
  }
  if (typeof content !== "string") {
    return String(content ?? "");
  }
  // Protobuf bytes are JSON-encoded as base64; try to decode.
  try {
    const decoded = atob(content);
    // Heuristic: if decoded result is printable text, use it.
    const isText = /^[\x20-\x7E\s]*$/.test(decoded);
    return isText ? decoded : content;
  } catch {
    return content;
  }
}

export function getConversationTitle(
  conv: Conversation | undefined,
  currentUser: User | null
): string {
  if (!conv) return "";
  if (conv.name) return conv.name;
  if (conv.type === "p2p") {
    return conv.peerUsername || `User ${conv.peerId}` || "Unknown";
  }
  return "Group";
}
