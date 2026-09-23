# Nonoka IM SDK 重新设计方案

> 本文档描述 SDK 的重新设计思路、分层架构、API 设计以及需要后端配合的改动。不包含具体代码实现。
>
> 核心目标：让开发者只需要关心 "和谁聊、聊什么"，而不需要关心 WebSocket、心跳、重连、seq 管理等底层细节。

---

## 一、设计原则

1. **IM 逻辑内聚**：SDK 只封装 IM 相关能力（消息收发、会话管理、状态同步），不包含注册登录、用户资料、好友群组等非 IM 逻辑。
2. **分层可独立使用**：HTTP Service 层、Realtime 层、Application 层可以独立使用。
3. **开发者零负担**：自动处理心跳、重连、seq 管理、消息去重、断线补偿。
4. **渐进式实现**：先实现纯 SDK 封装（不改后端），再补后端配合的功能。

---

## 二、三层架构

```
┌─────────────────────────────────────────┐
│ Layer 3: Application Layer              │
│ 面向 IM 应用开发者                       │
│ Conversation, UnreadManager, Message     │
├─────────────────────────────────────────┤
│ Layer 2: Realtime Layer                 │
│ 面向需要直接操作长连接的开发者            │
│ WebSocket, heartbeat, reconnect, push    │
├─────────────────────────────────────────┤
│ Layer 1: Service Layer                  │
│ 面向服务端 / Bot / 管理后台开发者         │
│ HTTP/gRPC: Auth, Message, Dispatch, ...  │
└─────────────────────────────────────────┘
```

### 各层使用者

| 使用者 | 使用哪一层 | 场景 |
|--------|-----------|------|
| App 前端开发者 | Layer 3 (Conversation) | "打开会话，发消息，收消息" |
| 实时通信开发者 | Layer 2 (Realtime) | 需要精细控制 WebSocket、自定义协议 |
| 服务端 / Bot | Layer 1 (Service) | 不需要 WebSocket，只调 HTTP API |

---

## 三、包结构设计

采用扁平结构，未来后端新增接口时直接新增文件：

```
pkg/sdk/
├── client.go              # 主 Client：组合 Service + Realtime + Application
├── options.go             # 配置选项
├── errors.go              # SDK 错误定义
├── types.go               # 共享类型（Message, SendResult, PullResult 等）
│
├── service.go             # HTTP 客户端基座（统一 baseURL、超时、重试）
├── service_auth.go        # AuthService（登录注册，可选使用）
├── service_message.go     # MessageService（HTTP 发消息）
├── service_dispatch.go    # DispatchService（自动获取 Gateway URL）
│
├── realtime.go            # WebSocket 实时客户端（连接、心跳、重连）
├── realtime_pending.go    # seq 请求-响应关联
├── realtime_handler.go    # 推送消息分发、状态流转
│
├── conversation.go        # 高层：会话对象
├── conversation_manager.go # 会话管理器（管理多个会话的生命周期）
└── README.md              # 使用文档
```

未来扩展（后端新增接口后）：
- `service_user.go` / `service_friend.go` / `service_group.go`
- `contact.go` / `group.go`

---

## 四、API 设计

### 4.1 主 Client（推荐入口）

```go
type Client struct {
    opts Options

    // Layer 1: HTTP services（可独立使用）
    Auth     *AuthService
    Message  *MessageService
    Dispatch *DispatchService

    // Layer 2: Realtime
    Realtime *RealtimeClient

    // Layer 3: Application
    Conversations *ConversationManager
}
```

#### 创建与连接

```go
client := sdk.NewClient(sdk.Options{
    BaseURL:           "http://localhost:8000",
    GatewayURL:        "",              // 可选，空则自动通过 Dispatch 获取
    DeviceID:          "my-device",
    HeartbeatInterval: 30 * time.Second,
    AutoReconnect:     true,
    AutoAck:           true,
    MaxReconnectAttempts: 0,
})

// 登录（Layer 1 HTTP）
loginResp, err := client.Auth.Login(ctx, "username", "password")

// 连接实时网关（Layer 2，自动调度 Gateway）
err = client.Connect(ctx, loginResp.Token, loginResp.UserID)
```

#### 消息收发

```go
// 方式 1：通过 Client 直接发（优先 WebSocket，断开时 fallback HTTP）
result, err := client.SendMessage(ctx, "p2p_1_2", sdk.MsgTypeText, []byte("hello"))

// 方式 2：通过 Conversation 发（推荐）
conv := client.Conversations.Get("p2p_1_2")
result, err := conv.SendText(ctx, "hello")

// 接收消息
client.OnMessage(func(msg *sdk.Message) {
    // 自动去重、自动 ACK（如果 AutoAck=true）
    fmt.Printf("收到: %s\n", string(msg.Content))
})
```

#### 状态查询

```go
client.IsConnected()   // WebSocket 是否连接
client.IsAuthed()      // 是否已认证
client.UserID()        // 当前用户 ID
```

#### 关闭

```go
client.Close()  // 关闭连接，停止所有后台 goroutine
```

---

### 4.2 Service Layer（HTTP API 封装）

可完全不依赖 WebSocket，供 Bot / 服务端使用。

```go
// AuthService
type AuthService struct{}
func (s *AuthService) Register(ctx, username, password string) (*RegisterResult, error)
func (s *AuthService) Login(ctx, username, password string) (*LoginResult, error)
func (s *AuthService) RefreshToken(ctx, refreshToken string) (*LoginResult, error) // 预留

// MessageService
type MessageService struct{}
func (s *MessageService) SendMessage(ctx, req *SendMessageRequest) (*SendMessageReply, error)

// DispatchService
type DispatchService struct{}
func (s *DispatchService) GetGateway(ctx context.Context, userID int64) (string, error)
```

**设计要点**：
- HTTP 客户端统一处理 baseURL、请求超时、重试、错误解析
- 返回的结构化错误包含 code / message / retryable

---

### 4.3 Realtime Layer（WebSocket 协议封装）

可独立使用，不依赖 Application 层。

```go
rt := sdk.NewRealtimeClient(sdk.RealtimeOptions{
    GatewayURL:    "ws://localhost:8000/ws",
    Token:         token,
    DeviceID:      "device-1",
    AutoAck:       true,
    AutoReconnect: true,
    OnMessage:     func(msg *sdk.Message) { ... },
    OnDisconnect:  func(reason error) { ... },
    OnConnect:     func() { ... },
})

err := rt.Connect(ctx)
result, err := rt.SendMessage(ctx, topic, msgType, content)
pull, err := rt.PullMessages(ctx, topic, lastSeq, limit)
err = rt.Acknowledge(msgID, topic, topicSeq)
```

**设计要点**：
- 自动管理 seq（单调递增）
- 请求-响应关联（pending map + channel）
- 心跳定时发送
- 断线自动重连（指数退避）
- 推送消息去重（topic + topic_seq）

---

### 4.4 Application Layer：Conversation（核心易用性提升）

```go
type Conversation struct {
    Topic       string
    Type        ConversationType    // P2P / Group
    Messages    []*Message          // 本地消息列表（内存缓存）
    LastSeq     uint64              // 已同步到的最大 seq
    UnreadCount int32               // 未读数

    OnMessage     func(msg *Message)
    OnReadReceipt func(upToSeq uint64)
}
```

#### 方法

```go
// 发送消息
func (c *Conversation) SendText(ctx context.Context, text string) (*SendResult, error)
func (c *Conversation) SendImage(ctx context.Context, imageURL string) (*SendResult, error)
func (c *Conversation) SendFile(ctx context.Context, fileURL string, name string) (*SendResult, error)

// 加载历史
func (c *Conversation) LoadHistory(ctx context.Context, limit int32) ([]*Message, error)
func (c *Conversation) LoadMore(ctx context.Context, limit int32) ([]*Message, error)

// 已读回执
func (c *Conversation) MarkRead(ctx context.Context) error

// 状态查询
func (c *Conversation) UnreadCount() int32
func (c *Conversation) LastMessage() *Message
```

#### 使用示例

```go
conv := client.Conversations.Get("p2p_1_2")

// 加载历史
msgs, _ := conv.LoadHistory(ctx, 20)

// 发消息
_, _ = conv.SendText(ctx, "你好")

// 实时收消息
conv.OnMessage = func(msg *sdk.Message) {
    renderMessage(msg)
    conv.MarkRead(ctx)  // 自动发送已读回执
}

// 对方已读
conv.OnReadReceipt = func(upToSeq uint64) {
    markMessagesAsRead(upToSeq)
}
```

**设计要点**：
- Conversation 内部自动管理 `lastSeq`
- `LoadHistory` / `LoadMore` 自动调用 `PullMessages` 并更新 `lastSeq`
- 收到推送时自动追加到 `Messages` 列表
- 断线重连后自动拉取 `lastSeq + 1` 之后的离线消息

---

### 4.5 消息状态机

```go
type MessageStatus int

const (
    MessageStatusSending   MessageStatus = iota // 发送中（转圈）
    MessageStatusSent                            // 服务器已确认（收到 ACK）
    MessageStatusDelivered                       // 对方已收到（推送到达其设备）
    MessageStatusRead                            // 对方已读
    MessageStatusFailed                          // 发送失败
)
```

**状态流转**：

```
调用 SendMessage() ──→ Sending
                          │
                          ├─ 收到 ACK ──→ Sent
                          │
                          ├─ 收到 DeliveryReceipt ──→ Delivered
                          │
                          ├─ 收到 ReadReceipt ──→ Read
                          │
                          └─ 超时 / 错误 ──→ Failed
```

**状态存储位置**：
- `Message.Status` 字段
- SDK 内部维护一个 `map[clientMsgID]*Message` 记录发送中的消息

---

### 4.6 发送去重机制

SDK 内部自动维护 `client_msg_id`：

```go
// 发送时自动生成 client_msg_id
clientMsgID := uuid.New().String()

// 本地维护 "发送中" 缓存，避免同一内容重复发送
// 如果同一 client_msg_id 在 30 秒内重复发送，直接返回上次结果

// 如果网络超时，SDK 自动重试（带指数退避，最多 3 次）
```

**注意**：这是客户端侧去重，和服务端的 `client_msg_id` 唯一索引（MongoDB）是互补关系。

---

### 4.7 断线重连后的消息补偿

SDK 在重连成功后自动执行：

```go
// 1. 重新认证
// 2. 对每个活跃会话：
//    - 调用 PullMessages(lastSeq + 1, limit)
//    - 合并到本地消息列表
//    - 触发 OnMessage 回调
// 3. 对发送中的消息：
//    - 检查是否已确认（通过 Pull 确认）
//    - 未确认的自动重发
```

开发者不需要关心断线期间的消息丢失问题。

---

## 五、后端需要配合的改动

### 5.1 后端不需要改的（纯 SDK 封装）

| 功能 | 说明 |
|------|------|
| WebSocket 连接管理 | 已有 |
| 心跳 | 已有 |
| 消息发送 / ACK | 已有 |
| 离线消息拉取（Pull） | 已有 |
| 推送（Notify） | 已有 |
| 消息去重（服务端） | 已有（MongoDB 唯一索引） |
| Conversation 抽象 | SDK 层封装 |
| 发送去重（客户端） | SDK 层逻辑 |
| 断线自动拉取 | SDK 层逻辑 |
| 本地消息缓存 | SDK 层逻辑 |

### 5.2 后端需要新增的

#### 改动 1：`CMD_READ_RECEIPT` — 已读回执（中等工作量）

**需求**：用户看完消息后标记已读，服务器更新状态并通知发送方。

**需要新增**：
1. `packet.proto` 增加 `ReadReceipt` 消息类型
2. `handler.go` 增加 `handleReadReceipt` 处理函数
3. 更新 MongoDB：标记消息为已读
4. 推送给消息发送方：对方已读到 seq X

```protobuf
// packet.proto
message ReadReceipt {
    string topic = 1;
    uint64 up_to_seq = 2;   // 已读到这个 seq（包含）
    int64 reader_id = 3;    // 谁读的
}
```

**模式参考**：和现有的 `CMD_ACK` 处理流程几乎一样，可以直接复制扩展。

#### 改动 2：DeliveryReceipt — 消息送达通知（小工作量）

**需求**：消息推送到接收方设备后，给发送方一个 "已送达" 通知。

**方案选项**：
- **方案 A（推荐）**：复用现有的 `MessagePush`，给发送方推送一个特殊的系统消息
- **方案 B**：新增 `DeliveryReceipt` 消息类型

**实现位置**：`msgworker.go` 或 `pusher.go` 中，推送成功后给发送方发一个轻量通知。

#### 改动 3：未读数查询 API（小工作量，可选）

**需求**：获取每个 topic 的未读消息数。

**方案**：
- 后端新增 HTTP API：`GET /v1/message/unread?topic=xxx`
- 或者 SDK 自己通过 `PullMessages` 返回的消息数推算未读数（不需要后端改）

**建议**：先让 SDK 自己推算，后端 API 可以后面补。

### 5.3 后端需要修的 Bug

#### Bug：msgworker 的 `IsDuplicateError` 判断位置

**位置**：`internal/msgworker/msgworker.go`

**问题**：`IsDuplicateError(err)` 的判断写在 `if err != nil { return }` 之后，导致 `err` 已经是 nil，永远不会触发 `isDuplicate = true`。

**影响**：重复消息时 `isDuplicate` 永远是 false，group 消息可能被重复 push。

**修复**：把 `IsDuplicateError` 判断挪到 `if err != nil` 分支内部，或者让 storage 直接返回 `(recipientIDs, isDuplicate, error)`。

---

## 六、实施路径

建议分两步走，降低风险：

### Phase 1：纯 SDK 封装（不动后端）

**目标**：开发者能基于 Conversation 聊天，断线自动补消息。

**工作项**：
1. 修 msgworker `IsDuplicateError` bug（1 分钟）
2. SDK 重新设计包结构
3. 实现 Service Layer（HTTP 封装）
4. 实现 Realtime Layer（保留现有逻辑，整理代码）
5. 实现 Conversation + ConversationManager
6. 实现消息状态机（到 `Sent` 为止）
7. 实现发送去重 + 自动重试
8. 实现断线后自动拉取离线消息
9. 写新的集成测试

**里程碑**：SDK 可以独立跑通完整的聊天流程，开发者体验大幅提升。

### Phase 2：后端配合（加已读回执）

**目标**：消息状态机完整（Sent → Delivered → Read）。

**工作项**：
1. 后端加 `CMD_READ_RECEIPT` + `handleReadReceipt`
2. 后端加 `DeliveryReceipt` 推送
3. SDK 对接：
   - `Conversation.MarkRead()` 发送 ReadReceipt
   - 收到 ReadReceipt 更新 Message.Status = Read
   - 收到 DeliveryReceipt 更新 Message.Status = Delivered
4. 补充集成测试

**里程碑**：完整的消息状态流转，已读回执可用。

---

## 七、设计决策记录

### 决策 1：SendMessage 走 WS 还是 HTTP？

**结论**：优先 WebSocket，断开时 fallback HTTP。

理由：
- 在线时用 WS：低延迟、有实时上下文
- 离线时用 HTTP：保证消息至少能发出去

### 决策 2：Gateway URL 自动调度

**结论**：`Client.Connect()` 时，如果 `GatewayURL` 为空，自动调用 `DispatchService.GetGateway()` 获取。

开发者不再需要硬编码 Gateway 地址。

### 决策 3：消息状态机是否必须？

**结论**：必须。没有状态机，开发者无法展示 "发送中..."、"对方已读" 等 UI 状态，IM 体验不完整。

### 决策 4：Conversation 是否必须？

**结论**：必须。Conversation 是 IM SDK 的核心抽象，让开发者从 "操作 topic 和 seq" 变成 "打开会话聊天"。

### 决策 5：Token 刷新是否纳入 SDK？

**结论**：不纳入。Token 获取/刷新属于用户认证逻辑，不属于 IM 逻辑。SDK 只接收 token，不管理 token 生命周期。

---

## 八、已知限制

以下功能**不在本次设计范围内**：

| 功能 | 原因 | 未来扩展方式 |
|------|------|-------------|
| 用户注册 / 登录 | 不属于 IM 逻辑 | 开发者直接调 HTTP API |
| 用户资料 | 不属于 IM 逻辑 | 后端加 `UserService` + SDK 加 `service_user.go` |
| 好友关系 | 不属于 IM 逻辑 | 后端加接口 + SDK 加 `service_friend.go`、`contact.go` |
| 群组管理 | 不属于 IM 逻辑 | 后端加接口 + SDK 加 `service_group.go`、`group.go` |
| 文件上传 | 需要独立的存储服务 | 后端加 `UploadService` + SDK 加发送方法 |
| 本地持久化缓存 | 先使用内存缓存，足够验证设计 | 后续可加本地存储接口 |
| 多设备同步 | 依赖后端推送机制扩展 | 后端支持设备间广播后 SDK 对接 |
