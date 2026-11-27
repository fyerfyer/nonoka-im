# Nonoka IM Go SDK v1

A Go SDK for IM and the Nonoka Agent platform. It exposes resource modules for
HTTP APIs plus a typed WebSocket client and resumable agent run streams. The
SDK never exposes generated protobuf types.

## Quick start

```go
ctx := context.Background()

tokens := sdk.NewPasswordTokenSource(
    "http://localhost:8000", "alice", "password123", "my-device",
)

rt := sdk.DefaultRealtimeConfig()
rt.GatewayURLs = []string{"ws://localhost:8000/ws"}
rt.DeviceID = "my-device"

client, err := sdk.New(sdk.Config{
    BaseURL:     "http://localhost:8000",
    TokenSource: tokens,
    Realtime:    rt,
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}

conv := client.Conversations.Get(sdk.P2PTopic(1, 2))
conv.SetOnMessage(func(msg *sdk.Message) { fmt.Println(string(msg.Content)) })
result, err := conv.SendText(ctx, "hello!")
```

Server-side SDK clients must set `RealtimeConfig.Origin` when the gateway
configures `allowed_origins`; browser clients provide it automatically.

Messages with attachments use `MediaContent` (not a bare URL):

```go
result, err := client.Messages.SendMedia(ctx, topic, sdk.MessageTypeImage, sdk.MediaContent{
    FileID: uploaded.FileID, URL: uploaded.URL, Name: uploaded.Name,
    Mime: uploaded.Mime, Size: uploaded.Size,
})
```

`Messages.Send` uses the live WebSocket when connected, so the returned
`SendResult` contains the server-assigned `MessageID` and `TopicSeq`. Without a
WebSocket it falls back to the HTTP API, which returns only the accepted client
message ID.

## Message search

Search is scoped to one conversation and uses whatever backend the server has
configured (MongoDB fallback, Meilisearch, or a custom server adapter). The SDK
only sees the stable search contract:

```go
result, err := client.Messages.Search(ctx, sdk.SearchRequest{
    Topic:   "p2p_1_2",
    Query:   "incident",
    HasFile: false,
    Limit:   20,
})
if errors.Is(err, sdk.ErrSearchUnavailable) {
    // The deployment has no search backend.
}
for _, hit := range result.Hits {
    fmt.Println(hit.MessageID, hit.TopicSeq, hit.Snippet)
}
```

`Query` may be empty only when `HasFile` is true (browse attachments).

## Resources

| Field           | Capabilities                                                                    |
| --------------- | ------------------------------------------------------------------------------- |
| `Auth`          | register, login, refresh, logout                                                |
| `Users`         | search, presence                                                                |
| `Conversations` | list, mark read, lightweight topic clients                                      |
| `Messages`      | idempotent send, pull history/offline, topic-scoped keyword search              |
| `Groups`        | create, list, members                                                           |
| `Files`         | upload, download, share                                                         |
| `Agents`        | CRUD, draft, publish, restore, versions, shares, runs, tool catalog |
| `Models`        | read-only Bifrost catalog, administration sync/curation                         |
| `Quotas`        | admin user/agent quota CRUD and top usage                                       |
| `Audit`         | admin append-only audit event query                                             |
| `MCP`           | register, discover, test, review, connections                                   |
| `Skills`        | create/version/review                                                           |
| `Runs`          | get, events replay, cancel, retry, approve/reject, stream                       |
| `Realtime`      | typed WebSocket events, reconnect, auto-ack                                     |

## Agent configuration and models

```go
models, canManage, err := client.Models.List(ctx)
presets, tools, err := client.Agents.Tools(ctx)

agent, err := client.Agents.Create(ctx, sdk.AgentInput{
    Name: "Research Assistant", Instruction: "Be concise.",
    ModelID: models[0].ID,
    CapabilityPresets: []string{"im.read"}, ToolPolicies: map[string]string{},
})
versions, err := client.Agents.Versions(ctx, agent.ID)
```

Agents select one curated model ID; provider credentials, routing and
virtual-key lifetimes stay in Bifrost. Use `Models.Sync` / `Models.Update`
behind a platform-admin token.

## Typed errors

```go
_, err := client.Agents.Get(ctx, 42)
var apiErr *sdk.APIError
if errors.As(err, &apiErr) {
    fmt.Println(apiErr.StatusCode, apiErr.Code, apiErr.Retryable, apiErr.RequestID)
}
if sdk.IsNotFound(err) { ... }
```

## Token sources

- `sdk.StaticToken("...")`
- `sdk.TokenSourceFunc(func(ctx) (string, error))`
- `sdk.NewPasswordTokenSource(baseURL, username, password, deviceID)` — logs in
  on first use and refreshes before expiry.
- Implement `TokenSource`/`RefreshableTokenSource` for custom storage.

## Idempotent sends

Pass `ClientMessageID` explicitly when retrying outside the SDK. When omitted,
the SDK generates one and reuses it across retries. Successful results are
briefly cached by client message ID.

```go
result, err := client.Messages.Send(ctx, sdk.SendRequest{
    Topic: "p2p_1_2", Type: sdk.MessageTypeText,
    Content: []byte("hello"),
    ClientMessageID: "checkout-42",
})
```

## Agent runs

```go
// Runs are created by sending a mention/DM to a published agent.
run, err := client.Runs.GetByReply(ctx, replyMsgID, clientMsgID)
if err != nil { ... }

stream := client.Runs.Stream(ctx, run.ID)
defer stream.Close()
for event := range stream.Events() {
    switch event.Type {
    case sdk.RunEventDelta:
        fmt.Print(event.Delta)
    case sdk.RunEventToolStarted:
        fmt.Println("\n[tool]", event.Tool, event.Summary)
    case sdk.RunEventApprovalRequired:
        // Show the approval card; call Approve/Reject when the user decides.
    case sdk.RunEventCompleted:
        fmt.Println("\nreply message:", event.ReplyMsgID)
    }
}
if err := stream.Err(); err != nil { ... }
```

`RunStream` first replays `GET /v1/agent-runs/{id}/events?afterSeq=...`, then
follows WebSocket events and polls the replay endpoint after reconnects. Deltas
are transient; the final answer still arrives as a normal IM message.

## Realtime events

```go
for event := range client.Realtime.Events() {
    if event.Type == sdk.RealtimeMessage {
        fmt.Println(event.Message.Topic, string(event.Message.Content))
    }
}
```

One dispatcher reads the WebSocket and fans typed events out to all
subscriptions, preserving per-topic order.

## External bots

`pkg/sdk` is the human/Agent client. External bots use the separate
[`pkg/botsdk`](../botsdk/README.md) package and the
[External Bot Protocol](../../docs/bot-protocol.md).

## Breaking changes from the old SDK

- `sdk.Options`/`NewClient` were replaced by `Config`/`New`.
- Public types no longer use `api/im/v1`.
- `AutoReconnect`/`AutoAck` are explicit `Disable*` fields in `RealtimeConfig`.
- `SendMessage` uses a stable client message ID and only retries retryable errors.
- Old `ConversationManager` state was removed; `ConversationClient` is a
  stateless topic helper. Callbacks are installed with `SetOnMessage` /
  `SetOnReadReceipt` so they can be changed safely after the client starts.
- Image/file messages use the JSON `MediaContent` contract. Use
  `Messages.SendMedia` or pass the metadata returned by `Files.Upload`.
- Server-side WebSocket clients behind an `allowed_origins` gateway must set
  `RealtimeConfig.Origin`.

## Transport and load-test mode

All HTTP resources, including generated proto endpoints, share
`pkg/sdk/internal/transport` for token injection, one-shot 401 refresh,
bounded GET/DELETE/PUT retries and typed `APIError` decoding. The WebSocket
state machine lives in `pkg/sdk/internal/realtime`.

`test/loadtest` can exercise the public SDK instead of a bare WebSocket:

```bash
# SDK HTTP + WebSocket + receipts (pipeline keeps 1 msg/s/user under load)
go run ./test/loadtest -mode sdk -sdk-pipeline -users 300 -duration 60s -msg-rate 1

# group fan-out
go run ./test/loadtest -mode sdk -sdk-pipeline -chat-mode group -users 100 -duration 60s

# reconnect storm (SDK mode only)
go run ./test/loadtest -mode sdk -sdk-pipeline -users 300 -duration 60s \
  -reconnect-every 5s -reconnect-users 10

# raw protocol baseline
go run ./test/loadtest -mode raw -users 300 -duration 60s -msg-rate 1
```

`-msg-size` covers large-message runs; `-mode sdk` and `-mode raw` use the same
ring topology so throughput, ACK, push and receipt metrics can be compared.
The loadtest disables SDK auto-ack so the raw and SDK modes send the same number
of protocol ACKs; production clients should leave auto-ack enabled.
