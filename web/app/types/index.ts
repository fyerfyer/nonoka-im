export interface User {
  userId: number;
  username: string;
}

export interface Group {
  groupId: string;
  topic: string;
  name: string;
  ownerId: number;
  createdAt: number;
}

export interface GroupMember {
  userId: number;
  username: string;
}

export type MessageStatus = "sending" | "sent" | "delivered" | "read" | "failed" | "recalled";

export interface ToolCallState {
  tool: string;
  toolCallId?: string;
  summary?: string;
  status?: string;
}

export interface ApprovalState {
  approvalId: number;
  tool: string;
  summary: string;
  status: string;
}

export interface ChatMessage {
  clientMsgId: string;
  msgId?: number;
  topic: string;
  senderId: number;
  content: string;
  msgType?: number;
  timestamp: number;
  topicSeq?: number;
  status: MessageStatus;
  recalled?: boolean;
  // Client-only streaming state. These fields never come from the server.
  runId?: number;
  streaming?: boolean;
  streamStatus?:
    | "starting"
    | "streaming"
    | "waiting_approval"
    | "completed"
    | "failed"
    | "cancelled";
  toolCalls?: ToolCallState[];
  approval?: ApprovalState;
  errorCode?: string;
  errorClass?: string;
  errorMessage?: string;
  retryAfterAt?: string;
}

export interface Conversation {
  topic: string;
  type: "p2p" | "group";
  peerId: number;
  peerUsername: string;
  // peerIsBot marks a p2p conversation whose peer is an agent bot user.
  // Only set when the conversation was opened locally (NewChatDialog /
  // Agents page); the server conversation list does not return it.
  peerIsBot?: boolean;
  name?: string;
  lastMsgPreview?: string;
  lastMsgAt?: number;
  lastSeq: number;
  lastReadSeq: number;
  unreadCount: number;
  messages: ChatMessage[];
  hasMore: boolean;
  isLoading: boolean;
}

export interface AuthState {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
}

export type ConnectionState =
  | "connecting"
  | "connected"
  | "authed"
  | "disconnected"
  | "reconnecting";
