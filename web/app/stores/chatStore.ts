import { create } from "zustand";
import {
  Conversation,
  ChatMessage,
  ConnectionState,
  Group,
  GroupMember,
} from "@/types";

interface ChatStore {
  conversations: Map<string, Conversation>;
  activeTopic: string | null;
  connectionState: ConnectionState;
  groups: Map<string, Group>;
  // Group member tables keyed by topic, used to resolve sender display names.
  memberNames: Map<string, Map<number, string>>;
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
  prependMessages: (topic: string, msgs: ChatMessage[], hasMore: boolean) => void;
  markTopicRead: (topic: string, upToSeq: number) => void;
  updateConnectionState: (state: ConnectionState) => void;
  setGroup: (group: Group) => void;
  setGroups: (groups: Group[]) => void;
  setMemberNames: (topic: string, members: GroupMember[]) => void;
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

// previewForMessage renders the conversation-list preview for a message,
// mirroring the server-side placeholder for media messages.
function previewForMessage(msg: ChatMessage): string {
  if (msg.recalled) return "This message was recalled";
  if (msg.msgType === 2) return "[图片]";
  if (msg.msgType === 3) return "[文件]";
  return msg.content;
}

// insertBySeq appends a message, keeping ascending topic_seq order. Pushed
// messages can arrive late (after a pull already returned newer pages), so a
// plain push to the end would break ordering.
function insertBySeq(messages: ChatMessage[], msg: ChatMessage): ChatMessage[] {
  const seq = msg.topicSeq !== undefined ? Number(msg.topicSeq) : undefined;
  const last = messages[messages.length - 1];
  if (
    seq === undefined ||
    !last ||
    last.topicSeq === undefined ||
    seq >= Number(last.topicSeq)
  ) {
    return [...messages, msg];
  }
  const idx = messages.findIndex(
    (m) => m.topicSeq !== undefined && Number(m.topicSeq) > seq
  );
  const at = idx === -1 ? messages.length : idx;
  return [...messages.slice(0, at), msg, ...messages.slice(at)];
}

const initialState = {
  conversations: new Map<string, Conversation>(),
  activeTopic: null as string | null,
  connectionState: "disconnected" as ConnectionState,
  groups: new Map<string, Group>(),
  memberNames: new Map<string, Map<number, string>>(),
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
      // New object reference so zustand subscribers (Object.is) re-render.
      map.set(conv.topic, { ...existing, ...conv });
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
          (msg.topicSeq !== undefined &&
            m.topicSeq !== undefined &&
            // Proto-JSON pull yields string seqs while realtime push yields
            // numbers; compare numerically so duplicates are caught.
            Number(m.topicSeq) === Number(msg.topicSeq))
      );
      const messages = exists
        ? conv.messages
        : insertBySeq(conv.messages, msg);
      const msgSeq = msg.topicSeq !== undefined ? Number(msg.topicSeq) : undefined;
      map.set(topic, {
        ...conv,
        messages,
        lastMsgPreview: previewForMessage(msg),
        lastMsgAt: msg.timestamp,
        lastSeq:
          msgSeq !== undefined && msgSeq > conv.lastSeq ? msgSeq : conv.lastSeq,
      });
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
    const messages = conv.messages.map((m) => m.topicSeq !== undefined && (Number(m.topicSeq) === topicSeq || (!!msgId && Number(m.msgId) === msgId)) ? { ...m, content: "This message was recalled", status: "recalled" as const, recalled: true } : m);
    map.set(topic, { ...conv, messages, lastMsgPreview: "This message was recalled" }); return { conversations: map };
  }),
  setMessagesLoading: (topic, flag) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = ensureConversation(map, topic);
      map.set(topic, { ...conv, isLoading: flag });
      return { conversations: map };
    }),
  prependMessages: (topic, msgs, hasMore) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = ensureConversation(map, topic);
      const existingSeqs = new Set(
        conv.messages
          .map((m) => m.topicSeq)
          .filter((s): s is number => s !== undefined)
          .map(Number)
      );
      const existingClientIds = new Set(
        conv.messages.map((m) => m.clientMsgId).filter(Boolean)
      );
      const newMsgs = msgs.filter((m) => {
        if (m.topicSeq !== undefined && existingSeqs.has(Number(m.topicSeq))) {
          return false;
        }
        if (existingClientIds.has(m.clientMsgId)) {
          return false;
        }
        return true;
      });
      const merged = [...newMsgs, ...conv.messages].sort((a, b) => {
        const sa = Number(a.topicSeq) || 0;
        const sb = Number(b.topicSeq) || 0;
        return sa - sb;
      });
      const last = newMsgs.length > 0 ? newMsgs[newMsgs.length - 1] : undefined;
      map.set(topic, {
        ...conv,
        messages: merged,
        // hasMore comes from the server (limit+1 probing), not msg count.
        hasMore,
        lastSeq:
          last?.topicSeq !== undefined && Number(last.topicSeq) > conv.lastSeq
            ? Number(last.topicSeq)
            : conv.lastSeq,
      });
      return { conversations: map };
    }),
  markTopicRead: (topic, upToSeq) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      const messages = conv.messages.map((m) => {
        if (
          m.senderId !== conv.peerId &&
          m.topicSeq !== undefined &&
          m.topicSeq <= upToSeq
        ) {
          return { ...m, status: "read" as const };
        }
        return m;
      });
      map.set(topic, {
        ...conv,
        messages,
        unreadCount: 0,
        lastReadSeq: Math.max(conv.lastReadSeq, upToSeq),
      });
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
  setMemberNames: (topic, members) =>
    set((state) => {
      const map = new Map(state.memberNames);
      const names = new Map<number, string>();
      for (const m of members) {
        // Proto int64 fields arrive as strings in JSON; normalize.
        names.set(Number(m.userId), m.username);
      }
      map.set(topic, names);
      return { memberNames: map };
    }),
  reset: () => set(initialState),
}));

// Debug/testing handle: inspect or drive the store from devtools. Stripped
// from production builds.
if (process.env.NODE_ENV !== "production" && typeof window !== "undefined") {
  (window as unknown as Record<string, unknown>).__chatStore = useChatStore;
}
