# AGENTS.md — nonoka-im 开发指南

面向 AI 编码助手的项目说明书。开始任何任务前先读完本文件。

## 项目概述

nonoka-im 是一个 Go 实现的即时通讯（IM）系统，基于 Kratos v2 微服务骨架。核心架构：**Gateway（WebSocket 长连接 + gRPC/HTTP API）与 MsgWorker（消息持久化 / 排序 / 推送）通过 Kafka 解耦**；另有独立的 agent-worker 提供 AI 对话能力。前端是 Next.js Web 应用。

## 架构与模块

### 服务入口（cmd/）

- `cmd/nonoka-im` — 网关主服务：gRPC + HTTP server、WebSocket server、Redis 心跳节点注册，wire 依赖注入。
- `cmd/msgworker` — Kafka 消费者：消费 `im-messages` → 生成 topic_seq / msg_id → MongoDB 落库 → 推回在线客户端。
- `cmd/agent-worker` — AI agent worker：消费 Kafka `agent-events`，用 CloudWeGo Eino（OpenAI 兼容 API，默认 DeepSeek）执行 agent，OpenFGA 鉴权后经 HTTP 把回复发回 IM。**纯环境变量驱动**（`POSTGRES_DSN`、`KAFKA_BROKERS`、`OPENAI_*`、`DEEPSEEK_API_KEY` 等）。
- `cmd/authz-migrate` — 一次性工具：向 OpenFGA 写授权模型 / 历史数据回填。

### 核心包（internal/）

- `gateway/` — WebSocket 连接/会话管理、上行消息 Kafka producer、节点注册表。
- `service/` — Kratos service 层：auth、message、group、user、conversation、dispatch、push、file、agent_http。
- `biz/` — 业务逻辑层（较薄）。
- `data/` — 数据访问：PostgreSQL repos（gorm）、Redis、MongoDB 装配。
- `msgworker/` — consumer、MongoDB storage、gRPC pusher、网关路由、seq 生成、重试队列。
- `agent/` — agent 运行时：`app`（harness、MessageTools）、`eino`（LLM）、`event`（Kafka + outbox）、`credential`（JWT）、`tools`。
- `authz/` — OpenFGA 封装（`model.json` 为授权模型）。
- `server/` — Kratos server 装配、限流中间件；`conf/` — proto 定义的配置 schema；`metrics/` — Prometheus；`topic/` — topic 命名（`p2p_` / `grp_` / `sys_`）。

### 其他

- `pkg/sdk/` — Go SDK（三层：HTTP Service → RealtimeClient WebSocket → ConversationManager），有自己的 README 和 DESIGN.md。
- `web/app/` — 官方前端：Next.js 16 + React 19 + TypeScript + Tailwind v4 + shadcn/ui + zustand，pnpm 管理。**注意 Next.js 16 与训练数据中的旧版本有破坏性差异，改前端前必读 `web/app/AGENTS.md`。**
- `examples/chatapp/` — SDK 示例聊天应用。
- `api/im/v1/*.proto` — API 定义，同时生成 gRPC 与 HTTP 接口；`third_party/` 放第三方 proto。
- `bin/` — 构建产物目录，**不要手改**。
- `docs/` — 设计与审查文档（implementation_plan.md、PERFORMANCE_TUNING.md、agent-architecture.md 等，多为历史记录）。

## 工具链与常用命令

Go 1.25，Kratos v2.9.2，wire（DI），gorm + PostgreSQL，go-redis v9，mongo-driver v2，segmentio/kafka-go，gorilla/websocket，OpenFGA。前端 pnpm 10 + Node 20。

- `make init` — 安装 protoc 插件 / kratos / wire。
- `make api` / `make config` — 改 proto 后重新生成（`api/**` / `internal/**`）；前端 proto 用 `pnpm proto:gen`（在 `web/app/`）。
- `make generate` — `go generate ./...`（含 wire）+ `go mod tidy`。**改了 wire 注入后必须跑。**
- `make build` — 构建全部到 `bin/`。
- 无 lint target，CI 只跑 `go vet ./...`；格式用 gofmt。前端：`pnpm lint` / `pnpm format`。

## 配置文件（configs/）

- `config.yaml` — 本地开发默认（HTTP 8000 / gRPC 9002，直连本机基础设施）。
- `config.test.yaml` — 集成测试（18000/19000，连测试容器端口）。
- `config.test.gateway1/2.yaml` — 双网关测试。
- `config.docker.yaml` — demo 容器内挂载；`config.scale.yaml` + `nginx-scale.conf` — scale demo。

## 测试规范（重要）

### 必须使用真实基础设施做集成测试

**所有功能改动必须通过 `docker-compose.test.yml` 启动的真实基础设施（PostgreSQL、Redis、MongoDB、Kafka、OpenFGA）做集成测试，不允许只用 mock / 单元测试充数。**

```bash
make test            # 一键：起测试依赖 → 分组跑全部集成测试 → 自动清理
make test-deps-up    # 只起依赖（postgres:5433 redis:6380 mongo:27018 kafka:9092 openfga:8081）
make test-auth / test-gateway / test-gateway-concurrent / test-kafka   # 分组跑
make test-deps-down  # 清理
```

集成测试代码在 `test/integration/`（约 22 个文件，含 auth / gateway / kafka / msgworker / offline_sync / dispatch / sdk_* 等）。新增功能应在该目录补充对应的集成测试。

### AI / agent 相关测试

agent 相关功能使用真实 LLM 测试：API Key 从项目根目录 `.env` 的 `DEEPSEEK_API_KEY` 读取（OpenAI 兼容接口，默认模型 DeepSeek）。agent 实连测试在 `test/agentlive/`。**不要把 key 写进代码或提交 `.env`。**

## 新功能开发流程（必须遵守）

1. **先理解现状再动手**：读现有代码与目录结构，了解已有实现（本仓库已建 CodeGraph 索引，优先用 CodeGraph 探索，见文末）。涉及不熟悉的库/组件/方案时，可 web search 同类开源项目与官方文档，参考别人的实现方式与组件用法。
2. **先审视、后实现**：动手前审视当前实现，若发现严重问题（架构缺陷、明显 bug、安全隐患等），先向用户提出并讨论，达成一致后再重构/优化。不要直接埋头写新代码或绕开问题。需要补测试时，在现有测试架构（`test/integration/` 等）基础上补充。

## 完成前后端功能后的验收流程（必须执行）

功能开发完成后，**不能只跑测试就结束**，必须做端到端验收：

1. `make demo` 启动完整栈（postgres / redis / mongo / kafka / openfga / nonoka-im / msgworker / agent-worker / web，前端在 http://localhost:3000，后端 HTTP 8000）。agent-worker 默认随栈启动；未配置 `DEEPSEEK_API_KEY` 时它保持空闲（日志告警并等待），不影响 IM 功能。
2. 确认各容器正常启动。
3. 用 **Playwright** 在浏览器里实测：注册 / 登录等基本流程 + 本次新实现的功能，确认真实可用。
4. 有问题先看日志排查：`docker compose -f docker-compose.yml logs <service>`（如 `nonoka-im`、`msgworker`、`web`、`agent-worker`），或按需看其他日志。
5. 验收完成后 `make demo-down` 清理。

多网关场景用 `make demo-scale`（双 gateway + nginx LB(18000) + 3 msgworker + agent-worker，前端仍在 3000）；运行前需先 `make demo-down`，两者端口冲突。所有运行配置统一放在 `configs/`（demo 用 `config.docker.yaml`，scale 用 `config.scale.yaml` + `nginx-scale.conf`）。

## 代码约定

- 提交前：`go vet ./...` 通过、gofmt 干净、相关集成测试通过。
- 改 `api/im/v1/*.proto` 后：`make api`；前端用到的话再跑 `web/app` 的 `pnpm proto:gen`。
- 改依赖注入后：`make generate` 重新生成 wire 代码。
- 目录边界：service 层只做协议适配，业务进 biz，存储进 data；msgworker 侧逻辑放 `internal/msgworker/`。
- 文档（docs/）多为历史设计稿，与代码冲突时以代码为准。

<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tools** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them. `codegraph_node` returns one symbol's source + callers, or reads a whole file with line numbers. If the tools are listed but deferred, load them by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` and `codegraph node <symbol-or-file>` print the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->
