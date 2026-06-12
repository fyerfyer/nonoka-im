# Nonoka IM Go SDK

Nonoka IM 的 Go SDK，封装了 HTTP 业务接口、WebSocket 长连接、会话管理、消息状态机等能力，可单独使用 HTTP 层构建后端机器人，也可通过 WebSocket 构建实时 IM 客户端。

## 架构分层

SDK 采用三层设计：

- **Layer 1 - HTTP Service**: `Auth`、`Message`、`Dispatch`，对应 REST API，可脱离 WebSocket 独立使用。
- **Layer 2 - Realtime**: `RealtimeClient` 负责 WebSocket 连接、认证、心跳、重连、消息推送。
- **Layer 3 - Application**: `ConversationManager` 提供面向会话的高层 API，自动维护本地消息缓存、未读数、消息状态。

## 特性

- **HTTP + WebSocket 双通道**：优先走 WebSocket，未连接时自动降级到 HTTP。
- **自动重连**：连接断开后自动重连，可配置重试次数和间隔。
- **自动心跳**：定时发送心跳并检测服务端回执，连续 missed 两次强制重连。
- **自动 ACK**：收到推送消息后自动发送 ACK（可关闭）。
- **消息去重**：基于 `topic + topic_seq` 对推送消息去重。
- **发送去重与重试**：同一 `client_msg_id` 的重复发送会返回缓存结果；发送失败按指数退避重试。
- **消息状态机**：`Sending` → `Sent` → `Delivered` → `Read`，自动跟踪发送、送达、已读状态。
- **会话管理**：`Conversation` 自动合并历史消息与本地发送中消息，维护未读数。
- **离线消息拉取**：重连后自动为所有会话拉取离线消息。
- **Receipt 回调**：支持 Send Receipt、Delivery Receipt、Read Receipt。
- **并发安全**：`SendMessage`、`PullMessages`、会话操作均可在多个 goroutine 中并发调用。

## 安装

```bash
go get nonoka-im/pkg/sdk
```

## 快速开始

### 完整客户端（HTTP + WebSocket）

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "nonoka-im/pkg/sdk"
)

func main() {
    client := sdk.NewClient(sdk.Options{
        BaseURL:           "http://localhost:8000",
        GatewayURL:        "ws://localhost:8000/ws",
        DeviceID:          "my-device",
        HeartbeatInterval: 30 * time.Second,
        HeartbeatTimeout:  60 * time.Second,
        RequestTimeout:    10 * time.Second,
        AutoReconnect:     true,
        AutoAck:           true,

        OnConnect: func() {
            fmt.Println("连接并认证成功!")
        },
        OnDisconnect: func(reason error) {
            fmt.Printf("连接断开: %v\n", reason)
        },
        OnMessage: func(msg *sdk.Message) {
            fmt.Printf("收到消息: topic=%s sender=%d content=%s\n",
                msg.Topic, msg.SenderID, string(msg.Content))
        },
    })

    // 1. 通过 HTTP 注册/登录
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    _, _ = client.Auth.Register(ctx, "alice", "password123")
    loginResult, err := client.Auth.Login(ctx, "alice", "password123")
    if err != nil {
        log.Fatalf("登录失败: %v", err)
    }
    client.UpdateToken(loginResult.Token)
    fmt.Printf("登录成功: user_id=%d\n", loginResult.UserID)

    // 2. 连接 WebSocket
    if err := client.Connect(ctx); err != nil {
        log.Fatalf("连接失败: %v", err)
    }

    // 3. 获取会话并发送消息
    conv := client.Conversations.Get("p2p_1_2")
    result, err := conv.SendText(ctx, "你好!")
    if err != nil {
        log.Fatalf("发送失败: %v", err)
    }
    fmt.Printf("发送成功: msg_id=%d topic_seq=%d\n", result.MsgID, result.TopicSeq)

    // 4. 加载历史
    msgs, err := conv.LoadHistory(ctx, 20)
    if err != nil {
        log.Fatalf("加载历史失败: %v", err)
    }
    fmt.Printf("加载了 %d 条历史消息\n", len(msgs))

    // 5. 标记已读
    _ = conv.MarkRead(ctx)

    // 6. 关闭
    client.Close()
}
```

### 仅使用 HTTP 服务层（机器人/服务端）

```go
client := sdk.NewClient(sdk.Options{
    BaseURL: "http://localhost:8000",
})

ctx := context.Background()
login, err := client.Auth.Login(ctx, "bot", "password")
if err != nil {
    log.Fatal(err)
}
client.UpdateToken(login.Token)

result, err := client.Message.SendMessage(ctx, &sdk.SendMessageRequest{
    Topic:   "grp_100",
    MsgType: sdk.MsgTypeText,
    Content: []byte("大家好"),
})
```

## 配置选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `BaseURL` | string | `""` | HTTP API 基础地址，如 `http://localhost:8000` |
| `GatewayURL` | string | `""` | WebSocket 网关地址，为空时自动通过 DispatchService 解析 |
| `Token` | string | `""` | JWT 认证 Token |
| `DeviceID` | string | `"sdk-default"` | 设备标识 |
| `HeartbeatInterval` | time.Duration | `30s` | 心跳发送间隔 |
| `HeartbeatTimeout` | time.Duration | `60s` | 心跳回执超时 |
| `RequestTimeout` | time.Duration | `10s` | 请求响应超时 |
| `ReconnectInterval` | time.Duration | `5s` | 重连间隔 |
| `AutoReconnect` | bool | `true` | 是否自动重连 |
| `MaxReconnectAttempts` | int | `0` | 最大重连次数（0 表示无限） |
| `AutoAck` | bool | `true` | 收到推送后自动发送 ACK |
| `OnMessage` | MessageHandler | nil | 收到消息时的全局回调 |
| `OnConnect` | ConnectHandler | nil | 连接并认证成功时的回调 |
| `OnDisconnect` | DisconnectHandler | nil | 断开连接时的回调 |
| `OnSendReceipt` | SendReceiptHandler | nil | 收到发送回执时的回调 |
| `OnDeliveryReceipt` | DeliveryReceiptHandler | nil | 收到送达回执时的回调 |
| `OnReadReceipt` | ReadReceiptHandler | nil | 收到已读回执时的回调 |

## Client API

### 连接与状态

#### `Connect(ctx context.Context) error`
建立 WebSocket 连接并认证。如果 `GatewayURL` 为空且 Token 有效，SDK 会自动调用 `DispatchService` 解析网关地址。

#### `Close() error`
关闭客户端，释放所有资源，可安全重复调用。

#### `UpdateToken(token string)`
更新 JWT Token。如果已连接，会触发重新连接以使用新 Token。

#### `UserID() int64`
返回当前认证用户的 ID。

#### `IsConnected() bool`
是否已连接 WebSocket（不一定已认证）。

#### `IsAuthed() bool`
是否已通过 WebSocket 认证。

### 消息发送

#### `SendMessage(ctx context.Context, topic string, msgType v1.MsgType, content []byte) (*SendResult, error)`
发送消息。优先使用 WebSocket，未连接时自动降级到 HTTP。自动生成 `client_msg_id`，支持发送去重和失败重试。

#### `SendText(ctx context.Context, topic string, text string) (*SendResult, error)`
发送文本消息。

#### `SendImage(ctx context.Context, topic string, imageURL string) (*SendResult, error)`
发送图片消息（content 为图片 URL）。

#### `SendFile(ctx context.Context, topic string, fileURL string) (*SendResult, error)`
发送文件消息（content 为文件 URL）。

#### `SendMessageWithMentions(ctx context.Context, topic string, msgType v1.MsgType, content []byte, mentionedUserIDs []int64) (*SendResult, error)`
发送带 @ 提醒的群聊消息。

### 消息拉取

#### `PullMessages(ctx context.Context, topic string, lastSeq uint64, limit int32) (*PullResult, error)`
拉取离线/历史消息。`lastSeq` 为 0 表示从头拉取。

### 全局回调

#### `OnMessage(handler MessageHandler)`
注册全局消息处理器，会在每个会话的 `OnMessage` 之前被调用。

## Conversation API

`Conversation` 是会话级高层抽象，自动维护本地消息缓存、未读数和消息状态。

### 获取会话

```go
conv := client.Conversations.Get("p2p_1_2")
```

### 发送消息

```go
result, err := conv.SendText(ctx, "你好!")
result, err := conv.SendImage(ctx, "https://example.com/img.png")
result, err := conv.SendFile(ctx, "https://example.com/file.pdf", "file.pdf")
```

### 历史消息

```go
// 加载最近的 N 条消息，合并本地发送中消息
msgs, err := conv.LoadHistory(ctx, 20)

// 加载更早的消息
older, err := conv.LoadMore(ctx, 20)
```

### 已读与未读

```go
_ = conv.MarkRead(ctx)
count := conv.GetUnreadCount()
last := conv.LastMessage()
```

### 会话回调

```go
conv.OnMessage = func(msg *sdk.Message) {
    fmt.Printf("新消息: %s\n", string(msg.Content))
}

conv.OnReadReceipt = func(upToSeq uint64) {
    fmt.Printf("对方已读到 seq %d\n", upToSeq)
}
```

## 消息状态

每条 `Message` 都带有一个 `Status` 字段：

| 状态 | 说明 |
|------|------|
| `MessageStatusSending` | 发送中 |
| `MessageStatusSent` | 服务器已确认（收到 Send Receipt） |
| `MessageStatusDelivered` | 对方设备已收到（收到 Delivery Receipt） |
| `MessageStatusRead` | 对方已读（收到 Read Receipt） |
| `MessageStatusFailed` | 发送失败 |

```go
msg.Status.String() // "sending" / "sent" / "delivered" / "read" / "failed"
```

## 类型

```go
type Message struct {
    MsgID       int64
    Topic       string
    SenderID    int64
    MsgType     v1.MsgType
    Content     []byte
    Timestamp   int64
    TopicSeq    uint64
    Status      MessageStatus
    ClientMsgID string
}

type SendResult struct {
    ClientMsgID string
    MsgID       int64
    Timestamp   int64
    TopicSeq    uint64
}

type PullResult struct {
    Messages []*Message
    HasMore  bool
    NextSeq  uint64
}
```

## HTTP Service 层

SDK 暴露三个 HTTP 服务层对象，可在不连接 WebSocket 的情况下使用：

### `client.Auth`

```go
Register(ctx, username, password) (*RegisterResult, error)
Login(ctx, username, password) (*LoginResult, error)
```

### `client.Message`

```go
SendMessage(ctx, *SendMessageRequest) (*SendResult, error)
PullMessages(ctx, *PullMessagesRequest) (*PullResult, error)
```

### `client.Dispatch`

```go
GetGateway(ctx, userID int64) (string, error)
```

## 错误处理

SDK 定义了以下错误：

| 错误 | 说明 |
|------|------|
| `ErrNotConnected` | 未连接 |
| `ErrAlreadyConnected` | 已经连接 |
| `ErrAuthFailed` | 认证失败 |
| `ErrRequestTimeout` | 请求超时 |
| `ErrConnectionClosed` | 连接已关闭 |
| `ErrServerError` | 服务器返回错误 |

服务器错误可以通过 `IsServerError` 获取详细信息：

```go
if se, ok := sdk.IsServerError(err); ok {
    fmt.Printf("服务器错误: code=%d message=%s retryable=%v retry_after=%v\n",
        se.Code, se.Message, se.Retryable, se.RetryAfter)
}
```

## 注意事项

1. **BaseURL 与 GatewayURL**：建议同时提供 `BaseURL` 和 `GatewayURL`。如果只提供 `BaseURL`，HTTP 层在 `NewClient` 时即可使用；如果同时提供且 Token 有效，`Connect` 会自动解析网关地址。
2. **Token 更新**：登录后调用 `client.UpdateToken(token)`，再调用 `Connect`。如果 Token 会过期，更新 Token 后会自动重连。
3. **并发安全**：`SendMessage`、`PullMessages`、会话的 `LoadHistory` / `LoadMore` / `MarkRead` 都是并发安全的。
4. **上下文超时**：所有网络操作都接受 `context.Context`，可通过 `context.WithTimeout` 控制超时。
5. **重连离线拉取**：开启 `AutoReconnect` 后，重连成功会自动为已有会话拉取离线消息。
