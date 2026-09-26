"use client";

import { useParams, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import { useConversation } from "@/hooks/useConversation";
import { ConversationList } from "@/components/chat/ConversationList";
import { ChatHeader } from "@/components/chat/ChatHeader";
import { MessageList } from "@/components/chat/MessageList";
import { MessageInput } from "@/components/chat/MessageInput";
import { usePresence } from "@/hooks/usePresence";

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("token");
}

export default function ChatDetailPage() {
  const [mounted, setMounted] = useState(false);
  const params = useParams();
  const topic = decodeURIComponent((params?.topic as string) || "");
  const router = useRouter();
  const token = useAuthStore((s) => s.token);
  const user = useAuthStore((s) => s.user);
  const connectionState = useChatStore((s) => s.connectionState);

  const { conversation, loadingMore, loadMore, sendMessage, retryMessage, recallMessage } = useConversation(
    topic || null
  );
  const peerOnline = usePresence(
    conversation?.type === "p2p" ? conversation.peerId : undefined,
    connectionState === "authed"
  );

  useEffect(() => {
    setMounted(true);
    if (!getToken()) {
      router.replace("/login");
    }
  }, [router, token]);

  if (!mounted || !getToken() || !topic) {
    return (
      <div className="flex h-screen w-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    );
  }

  return (
    <div className="flex h-screen w-full overflow-hidden bg-background">
      <ConversationList />
      <main className="flex w-full flex-1 flex-col md:w-auto">
        <ChatHeader
          conversation={conversation}
          connectionState={connectionState}
          peerOnline={peerOnline}
        />
        <div className="flex flex-1 flex-col overflow-hidden">
          <MessageList
            conversation={conversation}
            currentUser={user}
            loadingMore={loadingMore}
            onLoadMore={loadMore}
            onRecall={recallMessage}
            onRetry={retryMessage}
          />
          <MessageInput
            onSend={sendMessage}
            disabled={connectionState !== "authed"}
          />
        </div>
      </main>
    </div>
  );
}
