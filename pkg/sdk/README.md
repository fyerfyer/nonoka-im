# Nonoka IM Go SDK

Nonoka IM 的 Go SDK，封装了 WebSocket 连接管理、认证、心跳、重连、消息收发等底层细节，让开发者可以更方便地构建 IM 应用。

## 特性

- **连接管理** — WebSocket 自动连接与认证
- **自动重连** — 连接断开后自动重连，可配置重试次数和间隔
- **自动心跳** — 定时发送心跳维持连接
- **请求-响应关联** — 自动管理 `seq`，支持同步调用风格（`SendMessage` / `PullMessages`）
- **消息推送** — 通过回调函数接收服务器推送的消息
- **自动 ACK** — 收到推送消息后自动发送 ACK（可关闭）
- **消息去重** — 同一 `topic_seq` 的重复推送消息会自动去重
- **并发安全** — 支持并发发送消息

## 安装

```bash
go get nonoka-im/pkg/sdk
```

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    v1 "nonoka-im/api/im/v1"
    "nonoka-im/pkg/sdk"
)

func main() {
    // 1. 创建客户端
    client := sdk.NewClient(sdk.Options{
        GatewayURL:        "ws://localhost:8000/ws",
        Token:             "your-jwt-token",       // 通过 Login API 获取
        DeviceID:          "my-device",
        HeartbeatInterval: 30 * time.Second,
        RequestTimeout:    10 * time.Second,
        AutoReconnect:     true,
        AutoAck:           true,

        // 收到消息时的回调
        OnMessage: func(msg *sdk.Message) {
            fmt.Printf("收到消息: topic=%s sender=%d content=%s\n",
                msg.Topic, msg.SenderID, string(msg.Content))
        },

        // 连接成功时的回调
        OnConnect: func() {
            fmt.Println("连接并认证成功!")
        },

        // 断开连接时的回调
        OnDisconnect: func(reason error) {
            fmt.Printf("连接断开: %v\n", reason)
        },
    })

    // 2. 连接到网关
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := client.Connect(ctx); err != nil {
        log.Fatalf("连接失败: %v", err)
    }

    // 3. 发送消息
    sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer sendCancel()

    result, err := client.SendMessage(sendCtx, "p2p_1_2", v1.MsgType_MSG_TYPE_TEXT, []byte("你好!"))
    if err != nil {
        log.Fatalf("发送失败: %v", err)
    }
    fmt.Printf("消息已发送: client_msg_id=%s timestamp=%d\n", result.ClientMsgID, result.Timestamp)

    // 4. 拉取离线消息
    pullCtx, pullCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer pullCancel()

    pullResult, err := client.PullMessages(pullCtx, "p2p_1_2", 0, 50)
    if err != nil {
        log.Fatalf("拉取失败: %v", err)
    }
    fmt.Printf("拉取到 %d 条消息, hasMore=%v\n", len(pullResult.Messages), pullResult.HasMore)

    // 5. 关闭客户端
    client.Close()
}
```

## 配置选项

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `GatewayURL` | string | `ws://localhost:8000/ws` | WebSocket 网关地址 |
| `Token` | string | `""` | JWT 认证 Token |
| `DeviceID` | string | `"sdk-default"` | 设备标识 |
| `HeartbeatInterval` | time.Duration | `30s` | 心跳间隔 |
| `RequestTimeout` | time.Duration | `10s` | 请求超时 |
| `ReconnectInterval` | time.Duration | `5s` | 重连间隔 |
| `AutoReconnect` | bool | `true` | 是否自动重连 |
| `MaxReconnectAttempts` | int | `0` | 最大重连次数（0 表示无限） |
| `AutoAck` | bool | `true` | 收到推送后自动发送 ACK |
| `OnMessage` | MessageHandler | nil | 收到消息时的回调 |
| `OnConnect` | ConnectHandler | nil | 连接成功时的回调 |
| `OnDisconnect` | DisconnectHandler | nil | 断开连接时的回调 |

## API 参考

### Client 方法

#### Connect(ctx context.Context) error
建立 WebSocket 连接并认证。如果配置了 `AutoReconnect`，断开后会自动重连。

#### SendMessage(ctx context.Context, topic string, msgType v1.MsgType, content []byte) (*SendResult, error)
发送消息到指定 topic。返回服务器 ACK，包含 `ClientMsgID`、`Timestamp` 等。

#### SendMessageWithMentions(ctx context.Context, topic string, msgType v1.MsgType, content []byte, mentionedUserIDs []int64) (*SendResult, error)
发送带 @mentions 的消息。

#### PullMessages(ctx context.Context, topic string, lastSeq uint64, limit int32) (*PullResult, error)
拉取离线消息。`lastSeq` 为 0 表示从头拉取。返回消息列表和 `NextSeq`（用于下次拉取）。

#### Acknowledge(msgID int64, topic string, topicSeq uint64) error
手动发送 ACK（仅在 `AutoAck=false` 时需要手动调用）。

#### UserID() int64
获取当前认证用户的 ID。

#### IsConnected() bool
是否已连接（不一定已认证）。

#### IsAuthed() bool
是否已认证。

#### Close() error
关闭客户端，停止所有后台 goroutine。

### 类型

```go
type Message struct {
    MsgID     int64
    Topic     string
    SenderID  int64
    MsgType   v1.MsgType
    Content   []byte
    Timestamp int64
    TopicSeq  uint64
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

### 错误处理

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
    fmt.Printf("服务器错误: code=%d message=%s retryable=%v\n",
        se.Code, se.Message, se.Retryable)
}
```

## 注意事项

1. **Token 获取** — SDK 不包含 HTTP Login API，需要通过 `api/im/v1/auth.pb.go` 中的 gRPC/HTTP 接口获取 JWT Token，然后传给 SDK。
2. **并发安全** — `SendMessage`、`PullMessages`、`Acknowledge` 都是并发安全的，可以在多个 goroutine 中同时调用。
3. **消息去重** — SDK 基于 `topic + topic_seq` 进行去重，重复推送的消息不会重复触发 `OnMessage` 回调。
4. **上下文超时** — `Connect`、`SendMessage`、`PullMessages` 都接受 `context.Context`，可以通过 `context.WithTimeout` 控制超时。
