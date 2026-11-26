import { create } from "zustand";
import {
  Conversation,
  ChatMessage,
  ConnectionState,
  Group,
  GroupMember,
} from "@/types";

export interface BotStreamPayload {
  streamId: string;
  seq: number;
  type: "started" | "delta" | "final" | "aborted";
  topic: string;
  botId: number;
  botUserId: number;
  delta?: string;
  clientMsgId?: string;
  replyMsgId?: number;
  timestampMs?: number;
}

interface ChatStore {
  conversations: Map<string, Conversation>;
  activeTopic: string | null;
  connectionState: ConnectionState;
  groups: Map<string, Group>;
  // Group member tables keyed by topic, used to resolve sender display names.
  memberNames: Map<string, Map<number, string>>;
  // Topics where the current user was @mentioned (badge until read).
  mentionedTopics: Set<string>;
  setConversations: (list: Conversation[]) => void;
  upsertConversation: (conv: Partial<Conversation> & { topic: string }) => void;
  setActiveTopic: (topic: string | null) => void;
  appendMessage: (topic: string, msg: ChatMessage) => void;
  updateMessageStatus: (
    topic: string,
    clientMsgId: string,
    patch: Partial<ChatMessage>
  ) => void;
  upsertStreamingMessage: (topic: string, msg: ChatMessage) => void;
  updateStreamMessage: (
    topic: string,
    runId: number,
    patch: Partial<ChatMessage>
  ) => void;
  appendStreamDelta: (topic: string, runId: number, delta: string) => void;
  upsertBotStreamMessage: (topic: string, msg: ChatMessage) => void;
  appendBotStreamDelta: (
    topic: string,
    streamId: string,
    seq: number,
    delta: string
  ) => void;
  updateBotStreamMessage: (
    topic: string,
    streamId: string,
    patch: Partial<ChatMessage>
  ) => void;
  removeBotStreamMessage: (topic: string, streamId: string) => void;
  applyBotStreamEvent: (event: BotStreamPayload) => void;
  removeMessage: (topic: string, clientMsgId: string) => void;
  hasMessageID: (topic: string, msgId: number) => boolean;
  setMessageCitations: (topic: string, msgId: number, citations: ChatMessage["citations"]) => void;
  hasClientMessageID: (topic: string, clientMessageId: string) => boolean;
  recallMessage: (topic: string, topicSeq: number, msgId?: number) => void;
  setMessagesLoading: (topic: string, flag: boolean) => void;
  prependMessages: (topic: string, msgs: ChatMessage[], hasMore: boolean) => void;
  markTopicRead: (topic: string, upToSeq: number) => void;
  updateConnectionState: (state: ConnectionState) => void;
  setGroup: (group: Group) => void;
  setGroups: (groups: Group[]) => void;
  setMemberNames: (topic: string, members: GroupMember[]) => void;
  markTopicMentioned: (topic: string) => void;
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
  if (msg.msgType === 2) return "[Image]";
  if (msg.msgType === 3) return "[File]";
  if (msg.msgType === 5) return "[Agent approval]";
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
  mentionedTopics: new Set<string>(),
};

export const useChatStore = create<ChatStore>((set, get) => ({
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
          peerIsBot: conv.peerIsBot ?? existing?.peerIsBot,
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
      const existsIdx = conv.messages.findIndex(
        (m) =>
          m.clientMsgId === msg.clientMsgId ||
          (msg.topicSeq !== undefined &&
            m.topicSeq !== undefined &&
            // Proto-JSON pull yields string seqs while realtime push yields
            // numbers; compare numerically so duplicates are caught.
            Number(m.topicSeq) === Number(msg.topicSeq))
      );
      let messages = conv.messages;
      if (existsIdx >= 0) {
        const existing = conv.messages[existsIdx];
        // A real message replaces its local bot-stream placeholder.
        if ((existing.streaming || existing.botStreamId) && msg.msgId) {
          messages = [...conv.messages];
          messages[existsIdx] = {
            ...existing,
            ...msg,
            streaming: false,
            streamStatus: "completed",
          };
        }
      } else {
        messages = insertBySeq(conv.messages, msg);
      }
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
  upsertStreamingMessage: (topic, msg) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = ensureConversation(map, topic);
      if (conv.messages.some((m) => m.clientMsgId === msg.clientMsgId)) {
        return state;
      }
      map.set(topic, {
        ...conv,
        messages: [...conv.messages, msg],
        lastMsgPreview: msg.content || "Agent is responding…",
        lastMsgAt: msg.timestamp,
      });
      return { conversations: map };
    }),
  upsertBotStreamMessage: (topic, msg) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = ensureConversation(map, topic);
      if (conv.messages.some((m) => m.botStreamId === msg.botStreamId)) {
        return state;
      }
      map.set(topic, {
        ...conv,
        messages: [...conv.messages, msg],
        lastMsgPreview: msg.content || "Bot is responding…",
        lastMsgAt: msg.timestamp,
      });
      return { conversations: map };
    }),
  appendBotStreamDelta: (topic, streamId, seq, delta) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      const idx = conv.messages.findIndex((m) => m.botStreamId === streamId);
      if (idx < 0) return state;
      const current = conv.messages[idx];
      if ((current.botStreamSeq ?? 0) >= seq) return state;
      const messages = [...conv.messages];
      const next = {
        ...current,
        content: current.content + delta,
        botStreamSeq: seq,
        streaming: true,
        streamStatus: "streaming" as const,
      };
      messages[idx] = next;
      map.set(topic, {
        ...conv,
        messages,
        lastMsgPreview: next.content.slice(-120) || "Bot is responding…",
        lastMsgAt: next.timestamp,
      });
      return { conversations: map };
    }),
  updateBotStreamMessage: (topic, streamId, patch) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      const idx = conv.messages.findIndex((m) => m.botStreamId === streamId);
      if (idx < 0) return state;
      const messages = [...conv.messages];
      messages[idx] = { ...messages[idx], ...patch };
      map.set(topic, { ...conv, messages });
      return { conversations: map };
    }),
  removeBotStreamMessage: (topic, streamId) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      const messages = conv.messages.filter((m) => m.botStreamId !== streamId);
      if (messages.length === conv.messages.length) return state;
      map.set(topic, { ...conv, messages });
      return { conversations: map };
    }),
  applyBotStreamEvent: (event) => {
    if (!event.streamId || !event.topic) return;
    const chat = get();
    if (event.type === "aborted") {
      chat.removeBotStreamMessage(event.topic, event.streamId);
      return;
    }
    const timestamp = event.timestampMs ? event.timestampMs / 1000 : Date.now() / 1000;
    if (event.type === "started") {
      chat.upsertBotStreamMessage(event.topic, {
        clientMsgId: event.clientMsgId || `botstream:${event.streamId}`,
        topic: event.topic,
        senderId: event.botUserId,
        content: "",
        msgType: 1,
        timestamp,
        status: "sending",
        streaming: true,
        streamStatus: "starting",
        botStreamId: event.streamId,
        botStreamSeq: 0,
      });
      return;
    }
    if (event.type === "delta") {
      const conv = chat.conversations.get(event.topic);
      const exists = conv?.messages.some((m) => m.botStreamId === event.streamId);
      if (!exists) {
        chat.upsertBotStreamMessage(event.topic, {
          clientMsgId: event.clientMsgId || `botstream:${event.streamId}`,
          topic: event.topic,
          senderId: event.botUserId,
          content: "",
          msgType: 1,
          timestamp,
          status: "sending",
          streaming: true,
          streamStatus: "starting",
          botStreamId: event.streamId,
          botStreamSeq: 0,
        });
      }
      if (event.delta) {
        chat.appendBotStreamDelta(
          event.topic,
          event.streamId,
          event.seq,
          event.delta
        );
      }
      return;
    }
    if (event.type === "final") {
      const conv = chat.conversations.get(event.topic);
      const existing = conv?.messages.find(
        (m) => m.botStreamId === event.streamId
      );
      if (existing?.msgId !== undefined) {
        // The durable message already replaced the placeholder.
        return;
      }
      if (!existing) {
        chat.upsertBotStreamMessage(event.topic, {
          clientMsgId: event.clientMsgId || `botstream:${event.streamId}`,
          topic: event.topic,
          senderId: event.botUserId,
          content: event.delta || "",
          msgType: 1,
          timestamp,
          status: "sending",
          streaming: false,
          streamStatus: "completed",
          botStreamId: event.streamId,
          botStreamSeq: event.seq,
        });
        return;
      }
      chat.updateBotStreamMessage(event.topic, event.streamId, {
        content: event.delta || existing.content,
        streaming: false,
        streamStatus: "completed",
        botStreamSeq: Math.max(existing.botStreamSeq ?? 0, event.seq),
      });
      // If the durable message already arrived, drop the placeholder.
      const durable = conv?.messages.find(
        (m) =>
          m !== existing &&
          m.msgId !== undefined &&
          event.clientMsgId !== undefined &&
          m.clientMsgId === event.clientMsgId
      );
      if (durable) {
        chat.removeBotStreamMessage(event.topic, event.streamId);
      }
    }
  },
  updateStreamMessage: (topic, runId, patch) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      const idx = conv.messages.findIndex((m) => m.runId === runId);
      if (idx < 0) return state;
      const messages = [...conv.messages];
      messages[idx] = { ...messages[idx], ...patch };
      map.set(topic, { ...conv, messages });
      return { conversations: map };
    }),
  appendStreamDelta: (topic, runId, delta) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      const idx = conv.messages.findIndex((m) => m.runId === runId);
      if (idx < 0) return state;
      const messages = [...conv.messages];
      const next = { ...messages[idx], content: messages[idx].content + delta };
      messages[idx] = next;
      map.set(topic, {
        ...conv,
        messages,
        lastMsgPreview: next.content.slice(-120) || "Agent is responding…",
        lastMsgAt: next.timestamp,
      });
      return { conversations: map };
    }),
  removeMessage: (topic, clientMsgId) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      const messages = conv.messages.filter((m) => m.clientMsgId !== clientMsgId);
      if (messages.length === conv.messages.length) return state;
      map.set(topic, { ...conv, messages });
      return { conversations: map };
    }),
  setMessageCitations: (topic, msgId, citations) =>
    set((state) => {
      const map = new Map(state.conversations);
      const conv = map.get(topic);
      if (!conv) return state;
      const idx = conv.messages.findIndex((m) => Number(m.msgId) === msgId);
      if (idx < 0) return state;
      const messages = [...conv.messages];
      messages[idx] = { ...messages[idx], citations };
      map.set(topic, { ...conv, messages });
      return { conversations: map };
    }),
  hasMessageID: (topic, msgId) => {
    const conv = get().conversations.get(topic);
    return !!conv?.messages.some((m) => Number(m.msgId) === msgId);
  },
  hasClientMessageID: (topic, clientMessageId) => {
    if (!clientMessageId) return false;
    const conv = get().conversations.get(topic);
    return !!conv?.messages.some((m) => m.clientMsgId === clientMessageId);
  },
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
          Number(m.senderId) !== Number(conv.peerId) &&
          m.topicSeq !== undefined &&
          Number(m.topicSeq) <= upToSeq
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
      const mentioned = new Set(state.mentionedTopics);
      mentioned.delete(topic);
      return { conversations: map, mentionedTopics: mentioned };
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
  markTopicMentioned: (topic) =>
    set((state) => {
      if (state.mentionedTopics.has(topic)) return state;
      const mentioned = new Set(state.mentionedTopics);
      mentioned.add(topic);
      return { mentionedTopics: mentioned };
    }),
  reset: () => set(initialState),
}));

// Debug/testing handle: inspect or drive the store from devtools. Stripped
// from production builds.
if (process.env.NODE_ENV !== "production" && typeof window !== "undefined") {
  (window as unknown as Record<string, unknown>).__chatStore = useChatStore;
}
