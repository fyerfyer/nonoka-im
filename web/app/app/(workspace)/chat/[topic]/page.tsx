"use client";

import { useParams } from "next/navigation";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import { useConversation } from "@/hooks/useConversation";
import { ChatHeader } from "@/components/chat/ChatHeader";
import { MessageList } from "@/components/chat/MessageList";
import { MessageInput } from "@/components/chat/MessageInput";
import { usePresence } from "@/hooks/usePresence";

export default function ChatDetailPage() {
  const params = useParams();
  const topic = decodeURIComponent((params?.topic as string) || "");
  const user = useAuthStore((s) => s.user);
  const connectionState = useChatStore((s) => s.connectionState);

  const {
    conversation,
    loadingMore,
    loadMore,
    loadUntil,
    sendMessage,
    mentionMembers,
    sendFileMessage,
    retryMessage,
    recallMessage,
  } = useConversation(topic || null);
  const peerOnline = usePresence(
    conversation?.type === "p2p" ? conversation.peerId : undefined,
    connectionState === "authed"
  );

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-chat-canvas">
      <ChatHeader
        conversation={conversation}
        connectionState={connectionState}
        peerOnline={peerOnline}
      />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <MessageList
          conversation={conversation}
          currentUser={user}
          loadingMore={loadingMore}
          onLoadMore={loadMore}
          onJumpToSeq={loadUntil}
          onRecall={recallMessage}
          onRetry={retryMessage}
          members={mentionMembers()}
        />
        <MessageInput
          onSend={sendMessage}
          onSendFile={(file, onProgress) => sendFileMessage(file, onProgress)}
          members={mentionMembers()}
          disabled={connectionState !== "authed"}
        />
      </div>
    </div>
  );
}
