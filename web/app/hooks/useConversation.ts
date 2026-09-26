import { useCallback, useEffect, useRef, useState } from "react";
import { messageApi, conversationApi, groupApi, fileApi } from "@/lib/api";
import { realtimeClient } from "@/lib/realtime";
import { generateClientMsgId } from "@/lib/uuid";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import { ChatMessage, Conversation, User } from "@/types";
import { toast } from "sonner";

const MSG_TYPE_TEXT = 1;
const MSG_TYPE_IMAGE = 2;
const MSG_TYPE_FILE = 3;

export function useConversation(topic: string | null) {
  const user = useAuthStore((s) => s.user);
  const [loadingMore, setLoadingMore] = useState(false);
  const loadedRef = useRef<Set<string>>(new Set());
  const inFlightRef = useRef<Set<string>>(new Set());
  const loadingMoreRef = useRef(false);
  // Subscribe to the conversation object so status updates trigger re-renders
  const conversation = useChatStore(
    useCallback(
      (s) => (topic ? s.conversations.get(topic) : undefined),
      [topic]
    )
  );

  const loadLatest = useCallback(async () => {
    if (!topic || !user) return;
    // Guard against concurrent initial loads: store actions replace the
    // conversation object (zustand Object.is), which re-renders this hook,
    // and we must not start a new pull for every render.
    if (loadedRef.current.has(topic) || inFlightRef.current.has(topic)) {
      return;
    }
    inFlightRef.current.add(topic);
    useChatStore.getState().setMessagesLoading(topic, true);
    try {
      // endSeq=0 anchors on the latest page of history.
      const reply = await messageApi.pullMessages(topic, 0, 20, 0);
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
      useChatStore.getState().upsertConversation({
        topic,
        messages: msgs,
        hasMore: reply.hasMore,
      });
    } catch (err) {
      toast.error(
        `Failed to load messages: ${
          err instanceof Error ? err.message : "unknown error"
        }`
      );
    } finally {
      useChatStore.getState().setMessagesLoading(topic, false);
      loadedRef.current.add(topic);
      inFlightRef.current.delete(topic);
    }
  }, [topic, user]);

  const loadMore = useCallback(async () => {
    if (!conversation || !topic || conversation.isLoading || !conversation.hasMore)
      return;
    // Page backwards from the oldest message currently held. topicSeq may be
    // a proto-JSON string; coerce for the exclusive end_seq bound.
    const oldest = conversation.messages[0]?.topicSeq;
    const endSeq = oldest !== undefined ? Number(oldest) : 0;
    if (endSeq <= 0) return; // no messages yet; nothing older exists
    if (loadingMoreRef.current) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    try {
      const reply = await messageApi.pullMessages(topic, 0, 20, endSeq);
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
          : m.senderId === user?.userId
          ? "sent"
          : "delivered",
      }));
      useChatStore.getState().prependMessages(topic, msgs, reply.hasMore);
    } catch (err) {
      toast.error(
        `Failed to load older messages: ${
          err instanceof Error ? err.message : "unknown error"
        }`
      );
    } finally {
      loadingMoreRef.current = false;
      setLoadingMore(false);
    }
  }, [conversation, topic, user]);

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
      const { ok } = realtimeClient.sendMessage(
        topic,
        msg.msgType ?? MSG_TYPE_TEXT,
        new TextEncoder().encode(msg.content),
        msg.clientMsgId
      );
      if (!ok) {
        useChatStore
          .getState()
          .updateMessageStatus(topic, msg.clientMsgId, { status: "failed" });
        toast.error("Still disconnected — message not sent");
      }
    },
    [topic]
  );

  // sendFileMessage uploads an image/file attachment, then sends it as a
  // media message whose JSON content carries the file metadata.
  const sendFileMessage = useCallback(
    async (file: File) => {
      if (!topic) return;
      const isImage = file.type.startsWith("image/");
      const msgType = isImage ? MSG_TYPE_IMAGE : MSG_TYPE_FILE;
      const clientMsgId = generateClientMsgId();

      let uploaded;
      try {
        uploaded = await fileApi.upload(file);
      } catch (err) {
        toast.error(
          `Upload failed: ${err instanceof Error ? err.message : "unknown"}`
        );
        return;
      }

      const content = JSON.stringify({
        url: uploaded.url,
        name: uploaded.name,
        size: uploaded.size,
        mime: uploaded.mime,
      });
      const { ok } = realtimeClient.sendMessage(
        topic,
        msgType,
        new TextEncoder().encode(content),
        clientMsgId
      );
      const msg: ChatMessage = {
        clientMsgId,
        topic,
        senderId: user?.userId || 0,
        content,
        msgType,
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

  // Reads latest state so the callback identity stays stable regardless of
  // conversation object churn.
  const markRead = useCallback(() => {
    if (!topic) return;
    const conv = useChatStore.getState().conversations.get(topic);
    if (!conv || conv.lastSeq === 0 || conv.unreadCount === 0) return;
    conversationApi.markRead(topic).catch(() => {
      // ignore
    });
    realtimeClient.sendReadReceipt(topic, conv.lastSeq);
    useChatStore.getState().markTopicRead(topic, conv.lastSeq);
  }, [topic]);

  // Group conversations need a member table to render sender names; cached
  // per topic in the chat store (rebuilt from the server on each page load).
  const group = useChatStore(
    useCallback((s) => (topic ? s.groups.get(topic) : undefined), [topic])
  );

  // Mount/topic effect: stable deps only (topic + stable callbacks), so
  // store notifications don't re-trigger it.
  useEffect(() => {
    if (!topic) return;
    useChatStore.getState().setActiveTopic(topic);
    loadLatest();
    markRead();
    return () => {
      useChatStore.getState().setActiveTopic(null);
    };
  }, [topic, loadLatest, markRead]);

  // Mark read when unread messages arrive while this topic is open.
  const unreadCount = conversation?.unreadCount ?? 0;
  const lastSeq = conversation?.lastSeq ?? 0;
  useEffect(() => {
    if (unreadCount > 0) markRead();
  }, [unreadCount, lastSeq, markRead]);

  // Load the group member table once the group metadata is available.
  useEffect(() => {
    if (!topic || conversation?.type !== "group" || !group) return;
    let cancelled = false;
    groupApi
      .listMembers(group.groupId)
      .then((reply) => {
        if (!cancelled) {
          useChatStore.getState().setMemberNames(topic, reply.members);
        }
      })
      .catch(() => {
        // Sender names fall back to "Unknown"; not worth blocking the chat.
      });
    return () => {
      cancelled = true;
    };
  }, [topic, conversation?.type, group]);

  return {
    conversation,
    loadingMore,
    loadMore,
    sendMessage,
    sendFileMessage,
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
