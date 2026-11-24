# SDK v1 design

## Goals

- One client entry point with resource modules.
- HTTP for durable writes/reads; WebSocket for live events.
- Stable public types; generated protobuf types stay internal.
- Explicit token sources, typed errors, cursor pagination and idempotent sends.
- A run stream that survives reconnects and falls back to HTTP replay.

## Layers

```text
Client
  Auth/Users/Conversations/Messages/Groups/Files
  Agents/LLM/MCP/Skills/Runs
  Realtime (typed event fan-out)
      └── RunStream (replay + live + polling fallback)
```

`Client.New` creates a Kratos HTTP client for the existing IM proto endpoints
and a small JSON transport for the hand-written Agent platform endpoints. The
public API never exposes `api/im/v1` types.

## Send path

`Messages.Send` prefers the authenticated WebSocket when connected, so the
server's send receipt (msg_id/topic_seq) completes the `SendResult`. If the
WebSocket is unavailable it falls back to `POST /v1/message/send`, which only
acknowledges acceptance after Kafka. A short result cache keyed by
`ClientMessageID` makes explicit retries idempotent.

## Realtime fan-out

The WebSocket read loop is the only reader. It decodes packets and publishes
typed events to subscriber channels. Slow subscribers drop events; consumers
that need durability use the run replay endpoint.

`RealtimeConfig` uses `DisableAutoReconnect` / `DisableAutoAck` instead of
zero-value booleans, so an explicit false is representable.

## RunStream

1. `GET /v1/agent-runs/{id}/events?afterSeq=0`.
2. Subscribe to realtime events filtered by run ID.
3. Deduplicate by `(runID, seq)`.
4. Poll the replay endpoint every 2s while the WebSocket is unavailable.
5. Close on completed/failed/cancelled; `Err()` carries a typed failure.

Deltas are transient and never persisted in MongoDB; the final answer remains a
normal IM message referenced by `completed.replyMsgId`.

## Non-goals

- Exposing internal protobuf or storage types.
- Owning an application message cache (use `ConversationClient` as a stateless
  topic helper and keep your own store).
- Multi-language SDKs in this phase.
