# Nonoka IM

Nonoka IM 是一个基于 [Kratos](https://github.com/go-kratos/kratos) 微服务框架构建的即时通讯（IM）系统。它采用 Gateway + MsgWorker 分离架构：Gateway 专注维护 WebSocket 长连接，MsgWorker 负责消息的持久化、排序与推送，两者通过 Kafka 解耦。项目同时提供官方 Web 客户端（Next.js），实现开箱即用的聊天体验。

## 核心特性

- **微服务骨架**：基于 Kratos，API 协议使用 Protobuf 定义，同时生成 gRPC 与 HTTP 接口。
- **纯 WebSocket 接入**：客户端通过 WebSocket 进行认证、心跳、发送、拉取、ACK、已读回执等操作，使用统一的 Protobuf `Packet` 帧格式。
- **Gateway 与 MsgWorker 解耦**：Gateway 将上行消息投递至 Kafka 后立即响应；MsgWorker 异步消费、生成序列号、落库并推送。
- **多 Topic 模型**：统一使用 Topic 抽象单聊（`p2p_<uid1>_<uid2>`）、群聊（`grp_<groupId>`）与系统通知（`sys_<uid>`）。
- **分布式 ID 与序列号**：
  - `msg_id` 使用 Snowflake 生成，全局唯一。
  - `topic_seq` 使用 Redis `INCR` + Lua 脚本保证每个 Topic 内单调递增，并备份到 MongoDB 兜底。
- **消息可靠性**：
  - 基于 `client_msg_id` + MongoDB 唯一索引实现幂等去重。
  - 推送失败时进入 Redis 延迟重试队列，按指数退避重试。
  - 自动向发送方发送 Send Receipt，向在线接收方发送 Delivery Receipt。
- 支持消息撤回：发送方按 `topic_seq` 撤回，在线客户端收到 Recall Notice，离线拉取保留撤回状态。
- 撤回默认限制为 2 分钟；群聊撤回会向在线群成员实时广播，服务端仍会再次校验窗口和发送者权限。
  - Kafka 默认 3 分区；消费者按分区并行处理，失败消息本地重试后写入 `<topic>-dlq` 死信队列。
- **灵活分派策略**：DispatchService 支持一致性哈希（默认）与最小连接数策略，Gateway 节点通过 Redis 心跳自动发现。
- **官方 Web 客户端**：基于 Next.js 16 + TypeScript + Tailwind CSS + shadcn/ui 构建，支持单聊、群聊、用户搜索、会话列表与实时消息推送（[web/app](web/app)）。
- **Go SDK 与示例应用**：提供开箱即用的 Go SDK（[pkg/sdk](pkg/sdk)）以及完整聊天示例（[examples/chatapp](examples/chatapp)）。
- **多存储选型**：
  - PostgreSQL：用户、认证数据。
  - Redis：会话、序列号、群组成员、网关发现、消息状态、推送重试队列。
  - MongoDB：消息体、Inbox、历史记录。

## 技术栈

| 层级 | 技术 |
|------|------|
| 服务框架 | Go 1.25 + Kratos v2 |
| 协议 | Protobuf / gRPC / HTTP / WebSocket |
| 消息队列 | Kafka |
| 关系型数据 | PostgreSQL (GORM) |
| 缓存/会话 | Redis |
| 消息存储 | MongoDB |
| 依赖注入 | Wire |
| 部署 | Docker / Docker Compose |
| 官方 Web 客户端 | Next.js 16 + TypeScript + Tailwind CSS + shadcn/ui |

## 架构概览

```text
┌─────────────────────────────────────────────────────────────────────┐
│                           Client (Go SDK / JS / App)                 │
└─────────────────────────────────┬───────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│  HTTP API                  │  WebSocket Gateway                      │
│  - POST /v1/auth/register  │  - 长连接管理                            │
│  - POST /v1/auth/login     │  - 心跳 / 认证                           │
│  - GET  /v1/dispatch/gateway│  - 上行消息 → Kafka                     │
│  - POST /v1/message/send   │  - 下行推送 / ACK / Receipt             │
│  - GET  /v1/message/pull   │                                         │
└──────────────┬───────────────────────────────┬───────────────────────┘
               │                               │
               ▼                               ▼
        ┌─────────────┐              ┌──────────────────┐
        │  PostgreSQL │              │  Redis           │
        │  (users)    │              │  (session/seq)   │
        └─────────────┘              └────────┬─────────┘
                                              │
                                              ▼
                                    ┌──────────────────┐
                                    │   Kafka          │
                                    │   im-messages    │
                                    └────────┬─────────┘
                                             │
                                             ▼
                              ┌──────────────────────────┐
                              │      MsgWorker           │
                              │  - 生成 msg_id/topic_seq │
                              │  - MongoDB 落库          │
                              │  - 在线用户推送          │
                              │  - 推送失败重试          │
                              └────────────┬─────────────┘
                                           │
                                           ▼
                              ┌──────────────────────────┐
                              │       MongoDB            │
                              │  (messages / inbox)      │
                              └──────────────────────────┘
```

## 项目结构

```text
.
├── api/im/v1                  # Protobuf API 定义（auth / message / dispatch / push / packet）
├── cmd
│   ├── nonoka-im              # 主服务入口：HTTP/gRPC API + WebSocket Gateway
│   └── msgworker              # 消息工作者入口：消费 Kafka、落库、推送
├── internal
│   ├── biz                    # 业务用例层（auth / group / conversation 等）
│   ├── conf                   # 配置 Protobuf 与解析
│   ├── data                   # 数据访问层（PostgreSQL / Redis / MongoDB）
│   ├── gateway                # WebSocket Gateway 实现
│   ├── msgworker              # Kafka 消费、序列号、存储、推送、重试
│   ├── server                 # HTTP / gRPC 服务器注册
│   └── service                # Kratos Service 实现（auth / message / dispatch / push / group / conversation）
├── pkg/sdk                    # Go SDK
├── examples/chatapp           # 完整聊天示例
├── web/app                    # 官方 Web 客户端（Next.js App Router）
│   ├── app                    # 页面路由（登录、注册、聊天）
│   ├── components             # React 组件（聊天、认证、UI）
│   ├── hooks                  # 业务 Hooks（实时连接、会话、列表）
│   ├── lib                    # API 封装、WebSocket 客户端、Protobuf 生成代码
│   ├── stores                 # Zustand 全局状态（auth、chat）
│   └── types                  # TypeScript 类型
├── configs                    # 配置文件
├── test/integration           # 集成测试
├── docker-compose.yml         # 完整产品部署（含 Web 前端代理）
└── docker-compose.test.yml    # 测试依赖（PostgreSQL + Redis + Kafka + MongoDB）
```

## 快速开始

### 1. 启动基础设施

```bash
# 启动 PostgreSQL、Redis、MongoDB、ZooKeeper、Kafka
docker compose -f docker-compose.yml up -d postgres redis mongodb zookeeper kafka
```

> 若需要一键启动全部后端服务、中间件依赖与 Web 前端，可使用 `make demo`（停止并清理用 `make demo-down`），前端访问 <http://localhost:3000>。首次构建需要可访问 Go 模块代理（Dockerfile 默认使用 goproxy.cn，可用 `--build-arg GOPROXY=...` 覆盖）与 npm registry。

### 2. 初始化 Kafka Topic

```bash
docker exec nonoka_kafka kafka-topics \
  --bootstrap-server 127.0.0.1:9092 \
  --create --topic im-messages \
  --partitions 1 --replication-factor 1
```

### 3. 构建后端

```bash
make build
```

### 4. 启动主服务

```bash
./bin/nonoka-im -conf ./configs/config.yaml
```

- HTTP API: [http://localhost:8000](http://localhost:8000)
- gRPC: `localhost:9000`
- WebSocket: `ws://localhost:8000/ws`

### 5. 启动 MsgWorker

```bash
./bin/msgworker -conf ./configs/config.yaml
```

> MsgWorker 依赖 `GATEWAY_GRPC_ADDR` 环境变量或配置中的 `server.grpc.addr` 来连接 Gateway 进行推送。

### 6. 启动 Web 前端（本地开发）

`make demo` / `make demo-scale` 已包含 Web 前端容器；以下步骤仅用于本地开发调试：

```bash
cd web/app
pnpm install
pnpm dev
```

前端默认运行在 [http://localhost:3000](http://localhost:3000)。如果 3000 端口被占用，Next.js 会自动提示切换到其他端口。

打开浏览器注册/登录后即可开始单聊或群聊。前端开发详情请参考 [web/app/README.md](web/app/README.md)。

### 7. 运行 SDK 示例

```bash
cd examples/chatapp
go run ./cmd/client
```

更多 SDK 用法请参考 [pkg/sdk/README.md](pkg/sdk/README.md)。

### 示例演示：多 Worker 消费组扩容

`msgworker` 是独立进程，多个实例共享 `consumer_group`，Kafka 会按 partition 自动分配负载。项目提供了三实例演示入口：

```bash
make demo-scale
docker compose -f docker-compose.scale.yml ps
docker compose -f docker-compose.scale.yml logs -f msgworker
```

该演示同时启动两个 gateway（nginx 负载均衡在 `http://localhost:18000`）与 Web 前端（`http://localhost:3000`），可直接注册两个账号互发消息验证跨网关节点投递。

演示环境仍使用单 Kafka broker + 3 分区以降低资源占用；生产环境应扩展 broker 数量并将副本因子提高到 3。停止任一 worker 后，Kafka 会触发再均衡，其分区会被其他 worker 接管。

## 已知限制

- 撤回目前按发送方 + `topic_seq` 授权；P2P 和群聊在线成员会收到通知，离线客户端在拉取历史消息时看到撤回状态。
- Kafka Compose 示例使用单 broker、3 分区、单副本，生产环境应提高 broker/副本数并配置监控、认证与保留策略。
- 集成测试依赖 PostgreSQL、Redis、MongoDB、Kafka；CI 的基础 Go 检查默认不启动外部依赖，完整集成测试请运行 `make test`。

## WebSocket 协议

客户端与 Gateway 之间通过二进制 WebSocket 帧传输统一的 `Packet` Protobuf 消息。

```protobuf
message Packet {
  Command cmd = 1;      // HEARTBEAT / AUTH / PUBLISH / ACK / PULL / NOTIFY / ...
  uint64 seq = 2;       // 请求-响应关联序号
  oneof payload {       // 根据 cmd 选择具体消息体
    AuthRequest auth_req = 10;
    AuthResponse auth_resp = 11;
    SendMessageRequest send_req = 20;
    SendMessageReply send_reply = 21;
    PullRequest pull_req = 30;
    PullReply pull_reply = 31;
    AckRequest ack_req = 40;
    MessagePush notify = 50;
    ReadReceipt read_receipt = 60;
    DeliveryReceipt delivery_receipt = 70;
    SendReceipt send_receipt = 80;
    ErrorResponse error = 99;
  }
}
```

完整协议定义见 [api/im/v1/packet.proto](api/im/v1/packet.proto)。

## HTTP API

由 Protobuf 自动生成，主要接口包括：

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/v1/auth/register` | 用户注册 |
| POST | `/v1/auth/login` | 用户登录，返回 JWT Token |
| GET  | `/v1/dispatch/gateway` | 获取推荐的 WebSocket 网关地址 |
| POST | `/v1/message/send` | HTTP 方式发送消息 |
| GET  | `/v1/message/pull` | 拉取离线/历史消息 |

完整 OpenAPI 定义见 [openapi.yaml](openapi.yaml)。

## Web 客户端

项目提供官方 Web 客户端，基于 Next.js 16 App Router 构建：

| 功能 | 说明 |
|------|------|
| 认证 | 注册、登录、JWT Token 持久化 |
| 单聊 | 实时收发文本消息、历史消息拉取、消息状态（sending/sent/delivered/read） |
| 群聊 | 创建群聊、搜索用户、群成员消息广播 |
| 会话列表 | 显示最近会话、最后消息预览、未读数、群名称 |
| 实时通信 | WebSocket 二进制 Protobuf 帧，支持心跳、断线重连、ACK、已读回执 |

前端目录：[web/app](web/app)。

## Go SDK 快速示例

```go
import (
    "context"
    "fmt"
    "time"

    v1 "nonoka-im/api/im/v1"
    "nonoka-im/pkg/sdk"
)

client := sdk.NewClient(sdk.Options{
    GatewayURL:        "ws://localhost:8000/ws",
    Token:             "<jwt-token>",
    DeviceID:          "device-1",
    HeartbeatInterval: 30 * time.Second,
    AutoReconnect:     true,
    AutoAck:           true,
    OnMessage: func(msg *sdk.Message) {
        fmt.Printf("收到消息: %s\n", string(msg.Content))
    },
})

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}

result, err := client.SendMessage(ctx, "p2p_1_2", v1.MsgType_MSG_TYPE_TEXT, []byte("你好!"))
if err != nil {
    log.Fatal(err)
}
fmt.Printf("发送成功: msg_id=%d topic_seq=%d\n", result.MsgID, result.TopicSeq)
```

更多选项与 API 参考见 [pkg/sdk/README.md](pkg/sdk/README.md)。

## 测试

项目使用集成测试覆盖核心链路。测试依赖通过 Docker Compose 启动。

```bash
# 启动测试依赖
make test-deps-up

# 运行全部测试
make test

# 单独运行分组测试
make test-auth
make test-gateway
make test-gateway-concurrent
make test-kafka

# 停止测试依赖
make test-deps-down
```

测试覆盖场景包括：

- 用户注册 / 登录 / JWT 鉴权
- WebSocket 连接、认证、心跳
- 单聊 / 群聊消息发送与推送
- 离线消息拉取与排序
- 并发连接与广播
- SDK 自动重连、消息去重、回执
- Kafka 端到端消息投递

## 配置说明

默认配置位于 [configs/config.yaml](configs/config.yaml)，关键字段说明：

```yaml
server:
  http:
    addr: 0.0.0.0:8000
  grpc:
    addr: 0.0.0.0:9000
data:
  database:
    driver: postgres
    source: host=127.0.0.1 user=postgres password=root dbname=nonoka_im port=5432 sslmode=disable
  redis:
    addr: 127.0.0.1:6379
  mongodb:
    uri: mongodb://127.0.0.1:27017
    database: nonoka_im
  kafka:
    brokers:
      - 127.0.0.1:9092
    topic: im-messages
    consumer_group: msgworker-group
dispatch:
  strategy: "consistent_hash"   # consistent_hash / least_connections
  heartbeat_interval: 10s
  node_ttl: 30s
gateway:
  heartbeat_interval: 30s
  heartbeat_timeout: 90s
  read_timeout: 60s
auth:
  jwt_secret: "change-me-in-production"
  token_ttl: 168h
```

## 部署

### Docker 镜像

```bash
docker build -t nonoka-im .
docker run --rm -p 8000:8000 -p 9000:9000 -v $(pwd)/configs:/data/conf nonoka-im
```

### 多 Gateway 节点

部署多个 Gateway 节点时，需要：

1. 每个节点的 `node_id` 唯一（默认使用 hostname）。
2. 配置 `dispatch.gateways` 静态列表，或依赖 Redis 心跳自动发现。
3. 前端调用 `/v1/dispatch/gateway` 获取应连接的 Gateway 地址。
4. MsgWorker 配置 `GATEWAY_GRPC_ADDR` 指向可用的 Gateway gRPC 地址。

## 开发

### 生成代码

修改 `api/**/*.proto` 或 `internal/**/*.proto` 后：

```bash
make api       # 生成 API 的 pb.go / http / grpc / openapi
make config    # 生成内部配置 pb.go
make generate  # 执行 go generate + go mod tidy
make all       # 执行以上全部
```

### 生成 Wire 依赖注入

```bash
cd cmd/nonoka-im
go run github.com/google/wire/cmd/wire@latest
```

或安装后执行：

```bash
go install github.com/google/wire/cmd/wire@latest
cd cmd/nonoka-im && wire
```

## 相关文档

- [SDK 使用说明](pkg/sdk/README.md)

## 许可证

[MIT](LICENSE)
