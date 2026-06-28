import { useCallback, useEffect, useRef } from "react";
import { conversationApi, groupApi } from "@/lib/api";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import { Conversation, Group } from "@/types";
import { toast } from "sonner";

export function useConversations() {
  const user = useAuthStore((s) => s.user);
  const mounted = useRef(false);

  const refresh = useCallback(async () => {
    if (!user) return;
    try {
      const [convReply, groupReply] = await Promise.all([
        conversationApi.listConversations(),
        groupApi.listMyGroups().catch(() => ({ groups: [] as Group[] })),
      ]);

      useChatStore.getState().setGroups(groupReply.groups);

      const conversations: Conversation[] = convReply.conversations.map(
        (c) => ({
          topic: c.topic,
          type: c.type === "group" ? "group" : "p2p",
          peerId: c.peerId,
          peerUsername: c.peerUsername,
          name:
            c.type === "group"
              ? groupReply.groups.find((g) => g.topic === c.topic)?.name ||
                c.peerUsername
              : c.peerUsername,
          lastMsgPreview: c.lastMsgPreview,
          lastMsgAt: c.lastMsgAt,
          lastSeq: c.lastSeq,
          lastReadSeq: c.lastReadSeq,
          unreadCount: c.unreadCount,
          messages: [],
          hasMore: true,
          isLoading: false,
        })
      );

      // Ensure every group the user belongs to appears in the conversation list,
      // even if it has no messages yet.
      for (const g of groupReply.groups) {
        if (!conversations.some((c) => c.topic === g.topic)) {
          conversations.push({
            topic: g.topic,
            type: "group",
            peerId: 0,
            peerUsername: "",
            name: g.name,
            lastSeq: 0,
            lastReadSeq: 0,
            unreadCount: 0,
            messages: [],
            hasMore: true,
            isLoading: false,
          });
        }
      }

      useChatStore.getState().setConversations(conversations);
    } catch (err) {
      toast.error(
        `Failed to load conversations: ${
          err instanceof Error ? err.message : "unknown error"
        }`
      );
    }
  }, [user]);

  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      refresh();
    }
  }, [refresh]);

  return { refresh };
}
