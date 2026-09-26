import { useCallback, useEffect, useRef, useState } from "react";
import { messageApi, conversationApi, groupApi } from "@/lib/api";
import { realtimeClient } from "@/lib/realtime";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import { ChatMessage, Conversation, GroupMember, User } from "@/types";
import { toast } from "sonner";

const MSG_TYPE_TEXT = 1;

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
          content: decodeMessageContent(m.content, m.msgType),
          msgType: m.msgType,
          timestamp: m.timestamp,
          topicSeq: m.topicSeq,
          recalled: m.recalled || undefined,
          status: m.recalled
            ? "recalled"
            : m.senderId === user.userId
            ? "sent"
            : "delivered",
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
      const { clientMsgId, ok } = realtimeClient.sendText(topic, text.trim());
      const msg: ChatMessage = {
        clientMsgId,
        topic,
        senderId: user?.userId || 0,
        content: text.trim(),
        msgType: MSG_TYPE_TEXT,
        timestamp: Date.now() / 1000,
        status: ok ? "sending" : "failed",
      };
      useChatStore.getState().appendMessage(topic, msg);
      if (!ok) {
        toast.error("Message failed to send — click the warning icon to retry");
      }
    },
    [topic, user]
  );

  const retryMessage = useCallback(
    (msg: ChatMessage) => {
      if (!topic || msg.status !== "failed") return;
      useChatStore
        .getState()
        .updateMessageStatus(topic, msg.clientMsgId, { status: "sending" });
      const { ok } = realtimeClient.sendText(topic, msg.content, msg.clientMsgId);
      if (!ok) {
        useChatStore
          .getState()
          .updateMessageStatus(topic, msg.clientMsgId, { status: "failed" });
        toast.error("Still disconnected — message not sent");
      }
    },
    [topic]
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

  // Group conversations need a member table to render sender names; cached
  // per topic in the chat store (rebuilt from the server on each page load).
  const loadGroupMembers = useCallback(async () => {
    if (!topic) return;
    const group = useChatStore.getState().groups.get(topic);
    if (!group) return;
    try {
      const reply = await groupApi.listMembers(group.groupId);
      const members: GroupMember[] = reply.members;
      useChatStore.getState().setMemberNames(topic, members);
    } catch {
      // Sender names fall back to "Unknown"; not worth blocking the chat.
    }
  }, [topic]);

  useEffect(() => {
    if (!topic) return;
    useChatStore.getState().setActiveTopic(topic);
    if (!loadedRef.current.has(topic)) {
      loadMessages();
    }
    if (conversation?.type === "group") {
      void loadGroupMembers();
    }
    markRead();
    return () => {
      useChatStore.getState().setActiveTopic(null);
    };
  }, [topic, loadMessages, markRead, conversation?.type, loadGroupMembers]);

  return {
    conversation,
    loadingMore,
    loadMore,
    sendMessage,
    retryMessage,
    markRead,
    recallMessage: (msg: ChatMessage) => { if (topic && msg.topicSeq) realtimeClient.recallMessage(topic, msg.topicSeq, msg.msgId); },
  };
}

// decodeMessageContent decodes the HTTP pull payload. Protobuf `bytes`
// fields are JSON-encoded as base64, so TEXT content is always base64 —
// decode it unconditionally (a UTF-8 heuristic would wrongly leave CJK
// text like "你好" as raw base64). Non-TEXT payloads are reserved for
// media rendering (JSON) later; decode them to text for now.
function decodeMessageContent(
  content: string | Uint8Array | unknown,
  msgType: number
): string {
  if (content instanceof Uint8Array) {
    return new TextDecoder().decode(content);
  }
  if (typeof content !== "string") {
    return String(content ?? "");
  }
  try {
    const binary = atob(content);
    const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
    const decoded = new TextDecoder().decode(bytes);
    if (msgType !== MSG_TYPE_TEXT) {
      try {
        return JSON.stringify(JSON.parse(decoded));
      } catch {
        return decoded;
      }
    }
    return decoded;
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
