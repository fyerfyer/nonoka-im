import { create } from "zustand";
import type { RunCitation, RunEvent } from "@/lib/api/agents";
import type { ChatMessage, ToolCallState } from "@/types";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";

export interface LiveRun {
  runId: number;
  topic: string;
  agentId: number;
  botUserId: number;
  actorUserId: number;
  seq: number;
  text: string;
  tools: ToolCallState[];
  approvalId?: number;
  approvalStatus?: string;
  status: string;
  errorCode?: string;
  errorClass?: string;
  errorMessage?: string;
  retryAfterAt?: string;
  retryable?: boolean;
  replyMsgId?: number;
  clientMessageID?: string;
  inputTokens?: number;
  outputTokens?: number;
  citations?: RunCitation[];
}

interface RunStreamStore {
  runs: Map<number, LiveRun>;
  approvalStatus: Map<number, string>;
  typingTopics: Map<string, number>;
  applyEvent: (event: RunEvent) => void;
  onFinalMessage: (message: ChatMessage) => void;
  clearTestRun: (runId: number) => void;
}

function placeholder(run: LiveRun, timestampMs?: number): ChatMessage {
  return {
    clientMsgId: `run:${run.runId}`,
    topic: run.topic,
    senderId: run.botUserId,
    content: "",
    msgType: 1,
    timestamp: timestampMs ? timestampMs / 1000 : Date.now() / 1000,
    status: "sending",
    runId: run.runId,
    streaming: true,
    streamStatus: "starting",
    toolCalls: [],
  };
}

function upsertRunTool(tools: ToolCallState[], event: RunEvent): ToolCallState[] {
  const next = [...tools];
  const index = next.findIndex(
    (tool) =>
      (event.toolCallId && tool.toolCallId === event.toolCallId) ||
      (!event.toolCallId && tool.tool === event.tool)
  );
  const value: ToolCallState = {
    tool: event.tool || "tool",
    toolCallId: event.toolCallId,
    summary: event.summary,
    status: event.type === "tool_finished" ? event.status || "ok" : "running",
  };
  if (index >= 0) next[index] = { ...next[index], ...value };
  else next.push(value);
  return next;
}

export const useRunStreamStore = create<RunStreamStore>((set, get) => ({
  runs: new Map(),
  approvalStatus: new Map(),
  typingTopics: new Map(),

  applyEvent: (event) => {
    const currentUser = useAuthStore.getState().user?.userId || 0;
    const isActor = event.actorUserId === currentUser;
    const previous = get().runs.get(event.runId);
    if (previous && event.seq > 0 && event.seq <= previous.seq) return;
    const live: LiveRun = previous
      ? { ...previous, seq: Math.max(previous.seq, event.seq) }
      : {
          runId: event.runId,
          topic: event.topic,
          agentId: event.agentId,
          botUserId: event.botUserId,
          actorUserId: event.actorUserId,
          seq: event.seq,
          text: "",
          tools: [],
          status: event.status || "running",
        };
    if (event.type === "delta") live.text += event.delta || "";
    if (event.type === "tool_started" || event.type === "tool_finished") {
      live.tools = upsertRunTool(live.tools, event);
    }
    if (event.type === "started") live.status = "running";
    if (event.type === "heartbeat") live.status = "running";
    if (event.type === "approval_required") {
      live.approvalId = event.approvalId;
      live.approvalStatus = "pending";
      live.status = "waiting_approval";
    }
    if (event.type === "approval_resolved") {
      live.approvalId = event.approvalId || live.approvalId;
      live.approvalStatus = event.status;
      live.status = "running";
    }
    if (event.type === "completed") {
      live.status = "completed";
      live.replyMsgId = event.replyMsgId;
      live.clientMessageID = event.clientMessageId;
      live.inputTokens = event.inputTokens;
      live.outputTokens = event.outputTokens;
      live.citations = event.citations;
    }
    if (event.type === "failed") {
      live.status = "failed";
      live.errorCode = event.errorCode;
      live.errorClass = event.errorClass;
      live.errorMessage = event.errorMessage;
      live.retryAfterAt = event.retryAfterAt;
      live.retryable = event.retryable;
    }
    if (event.type === "cancelled") live.status = "cancelled";

    const runs = new Map(get().runs);
    runs.set(event.runId, live);
    const approvalStatus = new Map(get().approvalStatus);
    if (event.approvalId && event.type === "approval_required") {
      approvalStatus.set(event.approvalId, "pending");
    }
    if (event.approvalId && event.type === "approval_resolved") {
      approvalStatus.set(event.approvalId, event.status || "approved");
    }

    const typingTopics = new Map(get().typingTopics);
    if (!isActor) {
      if (event.type === "started" || event.type === "heartbeat") {
        typingTopics.set(event.topic, event.runId);
      }
      if (event.type === "completed" || event.type === "failed" || event.type === "cancelled") {
        if (typingTopics.get(event.topic) === event.runId) typingTopics.delete(event.topic);
      }
    }
    set({ runs, approvalStatus, typingTopics });

    if (!isActor) return;
    const chat = useChatStore.getState();
    if (event.type === "started") {
      chat.upsertStreamingMessage(event.topic, placeholder(live, event.timestampMs));
      return;
    }
    if (event.type === "delta" && event.delta) {
      chat.appendStreamDelta(event.topic, event.runId, event.delta);
    }
    const patch: Partial<ChatMessage> = {};
    if (event.type === "tool_started" || event.type === "tool_finished") {
      patch.toolCalls = live.tools;
    }
    if (event.type === "approval_required") {
      patch.streamStatus = "waiting_approval";
      patch.approval = {
        approvalId: live.approvalId || 0,
        tool: event.tool || "tool",
        summary: event.summary || "",
        status: "pending",
      };
    }
    if (event.type === "approval_resolved" && live.approvalId) {
      patch.approval = {
        approvalId: live.approvalId,
        tool: event.tool || "tool",
        summary: event.summary || "",
        status: event.status || "approved",
      };
      patch.streamStatus = "streaming";
    }
    if (event.type === "completed") {
      patch.streaming = false;
      patch.streamStatus = "completed";
      patch.status = "delivered";
      patch.content = live.text;
      patch.citations = live.citations;
    }
    if (event.type === "failed") {
      patch.streaming = false;
      patch.streamStatus = "failed";
      patch.errorCode = event.errorCode;
      patch.errorClass = event.errorClass;
      patch.errorMessage = event.errorMessage;
      patch.retryAfterAt = event.retryAfterAt;
    }
    if (event.type === "cancelled") {
      patch.streaming = false;
      patch.streamStatus = "cancelled";
    }
    if (Object.keys(patch).length > 0) chat.updateStreamMessage(event.topic, event.runId, patch);
    if (
      event.type === "completed" &&
      ((event.replyMsgId && chat.hasMessageID(event.topic, Number(event.replyMsgId))) ||
        (event.clientMessageId && chat.hasClientMessageID(event.topic, event.clientMessageId)))
    ) {
      get().onFinalMessage({
        clientMsgId: "",
        msgId: event.replyMsgId,
        topic: event.topic,
        senderId: live.botUserId,
        content: live.text,
        timestamp: 0,
        status: "delivered",
      });
    }
  },

  onFinalMessage: (message) => {
    const chat = useChatStore.getState();
    for (const run of get().runs.values()) {
      if (
        (run.replyMsgId && Number(run.replyMsgId) === Number(message.msgId)) ||
        (run.clientMessageID && run.clientMessageID === message.clientMsgId)
      ) {
        if (run.citations && message.msgId) {
          chat.setMessageCitations(run.topic, Number(message.msgId), run.citations);
        }
        chat.removeMessage(run.topic, `run:${run.runId}`);
        return;
      }
    }
    // The final IM push can arrive before the completed event. Remember the
    // message so the completed handler can remove the placeholder.
    if (message.msgId && chat.hasMessageID(message.topic, Number(message.msgId))) {
      for (const run of get().runs.values()) {
        if (run.topic !== message.topic) continue;
        if (run.status === "completed" && run.replyMsgId === Number(message.msgId)) {
          chat.removeMessage(run.topic, `run:${run.runId}`);
        }
      }
    }
  },

  clearTestRun: (runId) => {
    const runs = new Map(get().runs);
    runs.delete(runId);
    set({ runs });
  },
}));
