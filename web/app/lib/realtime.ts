import { api } from "@/lib/proto/im";
import { dispatchApi, refreshAuth, forceReLogin } from "@/lib/api";
import { generateClientMsgId } from "@/lib/uuid";

const { Packet, Command } = api.im.v1;

export type RealtimeEventType =
  | "connected"
  | "authed"
  | "disconnected"
  | "reconnecting"
  | "message"
  | "sendReceipt"
  | "deliveryReceipt"
  | "readReceipt"
  | "recall"
  | "error";

export interface RealtimeMessage {
  msgId: number;
  topic: string;
  senderId: number;
  msgType: number;
  content: string;
  timestamp: number;
  topicSeq: number;
  clientMsgId?: string;
}

export interface SendReceiptPayload {
  clientMsgId: string;
  msgId: number;
  topic: string;
  topicSeq: number;
  timestamp: number;
}

export interface DeliveryReceiptPayload {
  topic: string;
  topicSeq: number;
  msgId: number;
}

export interface ReadReceiptPayload {
  topic: string;
  upToSeq: number;
  readerId: number;
}

export interface RecallPayload { topic: string; topicSeq: number; msgId: number; senderId: number; recalledAt: number; }

type PacketType = InstanceType<typeof Packet>;

const HEARTBEAT_INTERVAL_MS = 30000;
const HEARTBEAT_TIMEOUT_MS = 10000;
const MAX_RECONNECT_DELAY_MS = 30000;

export class RealtimeClient extends EventTarget {
  private ws: WebSocket | null = null;
  private token: string | null = null;
  private userId: number | null = null;
  private seq = 0;
  private pending = new Map<
    number,
    {
      resolve: (pkt: PacketType) => void;
      reject: (err: Error) => void;
      timer: ReturnType<typeof setTimeout>;
    }
  >();
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private heartbeatTimeoutTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private closed = false;
  private didTokenRefresh = false;
  private authResolver: (() => void) | null = null;
  private authRejecter: ((err: Error) => void) | null = null;

  constructor() {
    super();
  }

  get isConnected() {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  async connect(token: string): Promise<void> {
    if (this.ws && this.isConnected) {
      return;
    }
    this.token = token;
    this.closed = false;
    this.didTokenRefresh = false;
    return this.doConnect();
  }

  private async doConnect(): Promise<void> {
    if (this.closed) return;

    try {
      this.userId = this.extractUserId(this.token!);
      const { gatewayUrl } = await dispatchApi.getGateway(this.userId);
      const url = gatewayUrl || this.inferGatewayUrl();

      return new Promise<void>((resolve, reject) => {
        const ws = new WebSocket(url);
        ws.binaryType = "arraybuffer";
        this.ws = ws;

        const onOpen = () => {
          this.dispatch("connected");
          this.sendAuth()
            .then(() => {
              this.reconnectAttempt = 0;
              this.startHeartbeat();
              this.dispatch("authed");
              resolve();
            })
            .catch((err) => {
              // Expired access token: swap it via the refresh token once,
              // then reconnect. If the refresh token is also dead, the
              // session is over — stop reconnecting and force re-login.
              if (!this.didTokenRefresh && isTokenError(err)) {
                this.didTokenRefresh = true;
                cleanup();
                try {
                  ws.close();
                } catch {
                  // ignore
                }
                this.ws = null;
                refreshAuth()
                  .then((newToken) => {
                    this.token = newToken;
                    resolve(this.doConnect());
                  })
                  .catch((refreshErr) => {
                    this.closed = true;
                    forceReLogin();
                    reject(refreshErr);
                  });
                return;
              }
              reject(err);
            });
        };

        const onMessage = (ev: MessageEvent) => {
          this.handleMessage(ev.data as ArrayBuffer);
        };

        const onClose = () => {
          cleanup();
          this.handleDisconnect();
        };

        const onError = (ev: Event) => {
          cleanup();
          reject(new Error("websocket error"));
        };

        const cleanup = () => {
          ws.removeEventListener("open", onOpen);
          ws.removeEventListener("message", onMessage);
          ws.removeEventListener("close", onClose);
          ws.removeEventListener("error", onError);
        };

        ws.addEventListener("open", onOpen);
        ws.addEventListener("message", onMessage);
        ws.addEventListener("close", onClose);
        ws.addEventListener("error", onError);
      });
    } catch (err) {
      if (!this.closed) {
        this.scheduleReconnect();
      }
      throw err;
    }
  }

  disconnect() {
    this.closed = true;
    this.clearTimers();
    this.rejectPending(new Error("disconnected"));
    if (this.ws) {
      try {
        this.ws.close();
      } catch {
        // ignore
      }
      this.ws = null;
    }
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  sendText(
    topic: string,
    text: string,
    clientMsgId = generateClientMsgId()
  ): { clientMsgId: string; ok: boolean } {
    const encoded = new TextEncoder().encode(text);
    const ok = this.send(Packet.create({
      cmd: Command.CMD_PUBLISH,
      seq: this.nextSeq(),
      sendReq: {
        topic,
        msgType: 1, // MSG_TYPE_TEXT
        content: encoded,
        clientMsgId,
      },
    }));
    return { clientMsgId, ok };
  }

  pullMessages(topic: string, lastSeq: number, limit = 20) {
    this.send(
      Packet.create({
        cmd: Command.CMD_PULL,
        seq: this.nextSeq(),
        pullReq: { topic, lastSeq, limit },
      })
    );
  }

  sendReadReceipt(topic: string, upToSeq: number) {
    this.send(
      Packet.create({
        cmd: Command.CMD_READ_RECEIPT,
        seq: this.nextSeq(),
        readReceipt: { topic, upToSeq, readerId: this.userId || 0 },
      })
    );
  }

  recallMessage(topic: string, topicSeq: number, msgId = 0) {
    this.send(Packet.create({ cmd: Command.CMD_RECALL, seq: this.nextSeq(), recallReq: { topic, topicSeq, msgId } }));
  }

  sendAck(msgId: number, topic: string, topicSeq: number) {
    this.send(
      Packet.create({
        cmd: Command.CMD_ACK,
        seq: this.nextSeq(),
        ackReq: { msgId, topic, topicSeq },
      })
    );
  }

  private sendAuth(): Promise<void> {
    return new Promise((resolve, reject) => {
      this.authResolver = resolve;
      this.authRejecter = reject;
      const seq = this.nextSeq();
      this.pending.set(seq, {
        resolve: (pkt: PacketType) => {
          this.pending.delete(seq);
          if (pkt.authResp?.success) {
            resolve();
          } else {
            reject(new Error("auth failed"));
          }
        },
        reject: (err: Error) => {
          this.pending.delete(seq);
          reject(err);
        },
        timer: setTimeout(() => {
          this.pending.delete(seq);
          reject(new Error("auth timeout"));
        }, 10000),
      });
      this.send(
        Packet.create({
          cmd: Command.CMD_AUTH,
          seq,
          authReq: { token: this.token!, deviceId: getDeviceId() },
        })
      );
    });
  }

  private send(packet: PacketType): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return false;
    }
    const buf = Packet.encode(packet).finish();
    this.ws.send(buf);
    return true;
  }

  private handleMessage(data: ArrayBuffer) {
    try {
      const pkt = Packet.decode(new Uint8Array(data)) as PacketType;

      if (Number(pkt.cmd) === Command.CMD_HEARTBEAT) {
        this.handleHeartbeatAck();
        return;
      }

      // Auth response special handling
      if (Number(pkt.cmd) === Command.CMD_AUTH && pkt.authResp) {
        if (this.authResolver) {
          if (pkt.authResp.success) {
            this.authResolver();
          } else {
            this.authRejecter?.(new Error("auth failed"));
          }
          this.authResolver = null;
          this.authRejecter = null;
        }
        return;
      }

      // Pending request handling
      if (this.pending.has(Number(pkt.seq))) {
        const p = this.pending.get(Number(pkt.seq))!;
        clearTimeout(p.timer);
        this.pending.delete(Number(pkt.seq));
        if (pkt.error) {
          p.reject(new Error(pkt.error.message || `error ${pkt.error.code}`));
        } else {
          p.resolve(pkt);
        }
        return;
      }

      // Server pushes
      if (Number(pkt.cmd) === Command.CMD_NOTIFY && pkt.notify) {
        const n = pkt.notify;
        const content =
          n.content instanceof Uint8Array
            ? new TextDecoder().decode(n.content)
            : typeof n.content === "string"
            ? n.content
            : "";
        this.dispatch("message", {
          msgId: Number(n.msgId),
          topic: n.topic,
          senderId: Number(n.senderId),
          msgType: n.msgType,
          content,
          timestamp: Number(n.timestamp),
          topicSeq: Number(n.topicSeq),
          clientMsgId: n.clientMsgId,
        } as RealtimeMessage);
        // Auto ack delivery
        this.sendAck(Number(n.msgId), n.topic || "", Number(n.topicSeq));
        return;
      }

      if (Number(pkt.cmd) === Command.CMD_SEND_RECEIPT && pkt.sendReceipt) {
        const r = pkt.sendReceipt;
        this.dispatch("sendReceipt", {
          clientMsgId: r.clientMsgId,
          msgId: Number(r.msgId),
          topic: r.topic,
          topicSeq: Number(r.topicSeq),
          timestamp: Number(r.timestamp),
        } as SendReceiptPayload);
        return;
      }

      if (Number(pkt.cmd) === Command.CMD_DELIVERY_RECEIPT && pkt.deliveryReceipt) {
        const r = pkt.deliveryReceipt;
        this.dispatch("deliveryReceipt", {
          topic: r.topic,
          topicSeq: Number(r.topicSeq),
          msgId: Number(r.msgId),
        } as DeliveryReceiptPayload);
        return;
      }

      if (Number(pkt.cmd) === Command.CMD_READ_RECEIPT && pkt.readReceipt) {
        const r = pkt.readReceipt;
        this.dispatch("readReceipt", {
          topic: r.topic,
          upToSeq: Number(r.upToSeq),
          readerId: Number(r.readerId),
        } as ReadReceiptPayload);
        return;
      }

      if (Number(pkt.cmd) === Command.CMD_RECALL && pkt.recallNotice) {
        const r = pkt.recallNotice;
        this.dispatch("recall", { topic: r.topic, topicSeq: Number(r.topicSeq), msgId: Number(r.msgId), senderId: Number(r.senderId), recalledAt: Number(r.recalledAt) } as RecallPayload);
        return;
      }

      if (Number(pkt.cmd) === Command.CMD_PUBLISH && pkt.sendReply) {
        // Publish accepted by server (no msgId yet)
        return;
      }
    } catch (err) {
      this.dispatch("error", err);
    }
  }

  private handleDisconnect() {
    this.stopHeartbeat();
    this.rejectPending(new Error("disconnected"));
    this.ws = null;
    this.dispatch("disconnected");
    if (!this.closed) {
      this.scheduleReconnect();
    }
  }

  private scheduleReconnect() {
    if (this.closed || this.reconnectTimer) return;
    this.dispatch("reconnecting");
    const delay = Math.min(
      1000 * 2 ** this.reconnectAttempt,
      MAX_RECONNECT_DELAY_MS
    );
    this.reconnectAttempt++;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.doConnect().catch(() => {
        // handled by onClose/onError
      });
    }, delay);
  }

  private startHeartbeat() {
    this.stopHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
      this.send(
        Packet.create({
          cmd: Command.CMD_HEARTBEAT,
          seq: this.nextSeq(),
        })
      );
      this.heartbeatTimeoutTimer = setTimeout(() => {
        this.ws?.close();
      }, HEARTBEAT_TIMEOUT_MS);
    }, HEARTBEAT_INTERVAL_MS);
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
    if (this.heartbeatTimeoutTimer) {
      clearTimeout(this.heartbeatTimeoutTimer);
      this.heartbeatTimeoutTimer = null;
    }
  }

  private handleHeartbeatAck() {
    if (this.heartbeatTimeoutTimer) {
      clearTimeout(this.heartbeatTimeoutTimer);
      this.heartbeatTimeoutTimer = null;
    }
  }

  private clearTimers() {
    this.stopHeartbeat();
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private rejectPending(err: Error) {
    for (const p of this.pending.values()) {
      clearTimeout(p.timer);
      p.reject(err);
    }
    this.pending.clear();
    if (this.authRejecter) {
      this.authRejecter(err);
      this.authResolver = null;
      this.authRejecter = null;
    }
  }

  private nextSeq(): number {
    return ++this.seq;
  }

  private dispatch(type: RealtimeEventType, detail?: unknown) {
    this.dispatchEvent(new CustomEvent(type, { detail }));
  }

  private extractUserId(token: string): number {
    try {
      const payload = token.split(".")[1];
      const json = JSON.parse(atob(payload));
      return Number(json.user_id) || 0;
    } catch {
      return 0;
    }
  }

  private inferGatewayUrl(): string {
    const base =
      process.env.NEXT_PUBLIC_API_BASE_URL || "http://127.0.0.1:8000";
    return base.replace(/^http/, "ws") + "/ws";
  }
}

function getDeviceId(): string {
  const key = "nonoka_device_id";
  try {
    const existing = sessionStorage.getItem(key);
    if (existing) return existing;
    const suffix =
      typeof crypto.randomUUID === "function"
        ? crypto.randomUUID()
        : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const id = `web-${suffix}`;
    sessionStorage.setItem(key, id);
    return id;
  } catch {
    return `web-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  }
}

function isTokenError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err);
  return /invalid token|auth failed|unauthorized|401/i.test(msg);
}

export const realtimeClient = new RealtimeClient();
