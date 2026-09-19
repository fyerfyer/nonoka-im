import { create } from "zustand";
import { Conversation, ChatMessage, ConnectionState, Group } from "@/types";

interface ChatStore {
  conversations: Map<string, Conversation>;
  activeTopic: string | null;
  connectionState: ConnectionState;
  groups: Map<string, Group>;
  setConversations: (list: Conversation[]) => void;
  upsertConversation: (conv: Partial<Conversation> & { topic: string }) => void;
  setActiveTopic: (topic: string | null) => void;
  appendMessage: (topic: string, msg: ChatMessage) => void;
  updateMessageStatus: (
    topic: string,
    clientMsgId: string,
    patch: Partial<ChatMessage>
  ) => void;
  recallMessage: (topic: string, topicSeq: number, msgId?: number) => void;
  setMessagesLoading: (topic: string, flag: boolean) => void;
  prependMessages: (topic: string, msgs: ChatMessage[]) => void;
  markTopicRead: (topic: string, upToSeq: number) => void;
  updateConnectionState: (state: ConnectionState) => void;
  setGroup: (group: Group) => void;
  setGroups: (groups: Group[]) => void;
  reset: () => void;
}

function ensureConversation(
  map: Map<string, Conversation>,
  topic: string
): Conversation {
  const existing = map.get(topic);
  if (existing) return existing;
  const conv: Conversation = {
    topic,
    type: topic.startsWith("p2p_") ? "p2p" : "group",
    peerId: 0,
    peerUsername: "",
    lastSeq: 0,
    lastReadSeq: 0,
    unreadCount: 0,
    messages: [],
    hasMore: true,
    isLoading: false,
  };
  map.set(topic, conv);
  return conv;
}

const initialState = {
  conversations: new Map<string, Conversation>(),
  activeTopic: null as string | null,
  connectionState: "disconnected" as ConnectionState,
  groups: new Map<string, Group>(),
};

export const useChatStore = create<ChatStore>((set) => ({
  ...initialState,
  setConversations: (list) =>
    set((state) => {
      const map = new Map<string, Conversation>();
      for (const conv of list) {
        const existing = state.conversations.get(conv.topic);
        map.set(conv.topic, {
          ...conv,
          messages: existing?.messages || conv.messages || [],
          name: conv.name || existing?.name,
          peerUsername: conv.peerUsername || existing?.peerUsername || "",
        });
      }
      // Keep any conversations that are not in the server list (e.g. newly created)
      for (const [topic, existing] of state.conversations) {
        if (!map.has(topic)) {
          map.set(topic, existing);
        }
      }
      return { conversations: map };
    }),
  upsertConversation: (conv) =>
    set((state) => {
      const map = new Map(state.conversations);
      const existing = ensureConversation(map, conv.topic);
      Object.assign(existing, conv);
      return { conversations: map };
    }),
  setActiveTopic: (topic) => set({ activeTopic: topic }),
  appendMessage: (topic, msg) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = ensureConversation(map, topic);
      const exists = conv.messages.some(
        (m) =>
          m.clientMsgId === msg.clientMsgId ||
          (msg.topicSeq !== undefined && m.topicSeq === msg.topicSeq)
      );
      if (!exists) {
        conv.messages.push(msg);
      }
      conv.lastMsgPreview = msg.content;
      conv.lastMsgAt = msg.timestamp;
      if (msg.topicSeq !== undefined && msg.topicSeq > conv.lastSeq) {
        conv.lastSeq = msg.topicSeq;
      }
      return { conversations: map };
    }),
  updateMessageStatus: (topic, clientMsgId, patch) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      const idx = conv.messages.findIndex((m) => m.clientMsgId === clientMsgId);
      if (idx >= 0) {
        const newMessages = [...conv.messages];
        newMessages[idx] = { ...newMessages[idx], ...patch };
        map.set(topic, { ...conv, messages: newMessages });
      }
      return { conversations: map };
    }),
  recallMessage: (topic, topicSeq, msgId) => set((state) => {
    const map = new Map(state.conversations); const conv = map.get(topic); if (!conv) return state;
    const messages = conv.messages.map((m) => m.topicSeq === topicSeq || (!!msgId && m.msgId === msgId) ? { ...m, content: "This message was recalled", status: "recalled" as const, recalled: true } : m);
    map.set(topic, { ...conv, messages, lastMsgPreview: "This message was recalled" }); return { conversations: map };
  }),
  setMessagesLoading: (topic, flag) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = ensureConversation(map, topic);
      conv.isLoading = flag;
      return { conversations: map };
    }),
  prependMessages: (topic, msgs) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = ensureConversation(map, topic);
      const existingSeqs = new Set(
        conv.messages
          .map((m) => m.topicSeq)
          .filter((s): s is number => s !== undefined)
      );
      const existingClientIds = new Set(
        conv.messages.map((m) => m.clientMsgId).filter(Boolean)
      );
      const newMsgs = msgs.filter((m) => {
        if (m.topicSeq !== undefined && existingSeqs.has(m.topicSeq)) {
          return false;
        }
        if (existingClientIds.has(m.clientMsgId)) {
          return false;
        }
        return true;
      });
      conv.messages.unshift(...newMsgs);
      conv.messages.sort((a, b) => {
        const sa = a.topicSeq || 0;
        const sb = b.topicSeq || 0;
        return sa - sb;
      });
      if (msgs.length < 20) {
        conv.hasMore = false;
      }
      if (newMsgs.length > 0) {
        const last = newMsgs[newMsgs.length - 1];
        if (last.topicSeq !== undefined && last.topicSeq > conv.lastSeq) {
          conv.lastSeq = last.topicSeq;
        }
      }
      return { conversations: map };
    }),
  markTopicRead: (topic, upToSeq) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      conv.unreadCount = 0;
      conv.lastReadSeq = Math.max(conv.lastReadSeq, upToSeq);
      for (const m of conv.messages) {
        if (
          m.senderId !== conv.peerId &&
          m.topicSeq !== undefined &&
          m.topicSeq <= upToSeq
        ) {
          m.status = "read";
        }
      }
      return { conversations: map };
    }),
  updateConnectionState: (state) => set({ connectionState: state }),
  setGroup: (group) =>
    set((state) => {
      const map = new Map(state.groups);
      map.set(group.topic, group);
      return { groups: map };
    }),
  setGroups: (groups) =>
    set(() => {
      const map = new Map<string, Group>();
      for (const g of groups) {
        map.set(g.topic, g);
      }
      return { groups: map };
    }),
  reset: () => set(initialState),
}));
