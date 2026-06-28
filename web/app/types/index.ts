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

export type MessageStatus = "sending" | "sent" | "delivered" | "read" | "failed";

export interface ChatMessage {
  clientMsgId: string;
  msgId?: number;
  topic: string;
  senderId: number;
  content: string;
  timestamp: number;
  topicSeq?: number;
  status: MessageStatus;
}

export interface Conversation {
  topic: string;
  type: "p2p" | "group";
  peerId: number;
  peerUsername: string;
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
