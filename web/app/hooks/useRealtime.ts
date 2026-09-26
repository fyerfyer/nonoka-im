import { useEffect, useRef } from "react";
import { realtimeClient, RealtimeEventType } from "@/lib/realtime";
import { refreshConversations } from "@/hooks/useConversations";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import { ChatMessage, User } from "@/types";
import { toast } from "sonner";

export function useRealtime() {
  const token = useAuthStore((s) => s.token);
  const user = useAuthStore((s) => s.user);
  const userRef = useRef<User | null>(user);

  useEffect(() => {
    userRef.current = user;
  }, [user]);

  useEffect(() => {
    if (!token) {
      realtimeClient.disconnect();
      return;
    }

    const chatStore = useChatStore.getState();
    chatStore.updateConnectionState("connecting");
    realtimeClient
      .connect(token)
      .then(() => {
        useChatStore.getState().updateConnectionState("authed");
      })
      .catch((err) => {
        useChatStore.getState().updateConnectionState("disconnected");
        toast.error(`Connection failed: ${(err as Error).message}`);
      });

    const handlers: Record<RealtimeEventType, (e: Event) => void> = {
      connected: () => useChatStore.getState().updateConnectionState("connected"),
      authed: () => useChatStore.getState().updateConnectionState("authed"),
      disconnected: () =>
        useChatStore.getState().updateConnectionState("disconnected"),
      reconnecting: () =>
        useChatStore.getState().updateConnectionState("reconnecting"),
      message: (e) => {
        const detail = (e as CustomEvent).detail;
        handleIncomingMessage(detail, userRef.current);
      },
      sendReceipt: (e) => {
        const detail = (e as CustomEvent).detail;
        useChatStore.getState().updateMessageStatus(
          detail.topic,
          detail.clientMsgId,
          {
            msgId: detail.msgId,
            topicSeq: detail.topicSeq,
            status: "sent",
          }
        );
      },
      deliveryReceipt: (e) => {
        const detail = (e as CustomEvent).detail;
        const conv = useChatStore.getState().conversations.get(detail.topic);
        if (!conv) return;
        const msg = conv.messages.find((m) => m.topicSeq === detail.topicSeq);
        if (msg && msg.status !== "read") {
          useChatStore.getState().updateMessageStatus(detail.topic, msg.clientMsgId, {
            status: "delivered",
          });
        }
      },
      readReceipt: (e) => {
        const detail = (e as CustomEvent).detail;
        useChatStore.getState().markTopicRead(detail.topic, detail.upToSeq);
      },
      recall: (e) => { const d = (e as CustomEvent).detail; useChatStore.getState().recallMessage(d.topic, d.topicSeq, d.msgId); },
      error: (e) => {
        const detail = (e as CustomEvent).detail;
        console.error("realtime error", detail);
      },
    };

    for (const [type, handler] of Object.entries(handlers)) {
      realtimeClient.addEventListener(type, handler);
    }

    return () => {
      for (const [type, handler] of Object.entries(handlers)) {
        realtimeClient.removeEventListener(type, handler);
      }
      realtimeClient.disconnect();
    };
  }, [token]);
}

function handleIncomingMessage(
  detail: {
    msgId: number;
    topic: string;
    senderId: number;
    msgType: number;
    content: string;
    timestamp: number;
    topicSeq: number;
    clientMsgId?: string;
    mentionedUserIds?: number[];
  },
  currentUser: User | null
) {
  const chatStore = useChatStore.getState();
  const myId = currentUser ? Number(currentUser.userId) : 0;
  const msg: ChatMessage = {
    clientMsgId: detail.clientMsgId || `${detail.topicSeq}-${detail.msgId}`,
    msgId: detail.msgId,
    topic: detail.topic,
    senderId: detail.senderId,
    content: detail.content,
    msgType: detail.msgType,
    timestamp: detail.timestamp,
    topicSeq: detail.topicSeq,
    status: detail.senderId === myId ? "sent" : "delivered",
  };

  const mentionsMe =
    myId !== 0 &&
    Array.isArray(detail.mentionedUserIds) &&
    detail.mentionedUserIds.includes(myId);

  const isActive = chatStore.activeTopic === detail.topic;
  chatStore.appendMessage(detail.topic, msg);

  // A push for a conversation we only know as a stub would render as
  // "Unknown"; refresh from the server to fill in peer info.
  const conv = chatStore.conversations.get(detail.topic);
  if (!conv?.peerUsername && !conv?.name) {
    void refreshConversations();
  }

  if (!isActive) {
    const conv = chatStore.conversations.get(detail.topic);
    if (conv) {
      chatStore.upsertConversation({
        topic: detail.topic,
        unreadCount: conv.unreadCount + 1,
      });
    }
  } else {
    // Active conversation: mark as read immediately.
    realtimeClient.sendReadReceipt(detail.topic, detail.topicSeq);
    chatStore.markTopicRead(detail.topic, detail.topicSeq);
  }

  // Someone @mentioned me: toast always; the "@我" badge only when the
  // conversation is not open (an open conversation is already being read).
  if (mentionsMe && detail.senderId !== myId) {
    const senderName =
      chatStore.memberNames.get(detail.topic)?.get(Number(detail.senderId)) ||
      conv?.peerUsername ||
      `User ${detail.senderId}`;
    const convName = conv?.name || senderName;
    toast.info(`${senderName} @了你（${convName}）`, {
      description:
        detail.msgType === 1
          ? detail.content.slice(0, 60)
          : detail.msgType === 2
          ? "[图片]"
          : "[文件]",
    });
    if (!isActive) {
      chatStore.markTopicMentioned(detail.topic);
    }
  }
}
