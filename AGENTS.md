# AGENTS.md — nonoka-im 开发指南

面向 AI 编码助手的项目说明书。开始任何任务前先读完本文件。

## 项目概述

nonoka-im 是一个 Go 实现的即时通讯（IM）系统，基于 Kratos v2 微服务骨架。核心架构：**Gateway（WebSocket 长连接 + gRPC/HTTP API）与 MsgWorker（消息持久化 / 排序 / 推送）通过 Kafka 解耦**；另有独立的 agent-worker 提供 AI 对话能力。前端是 Next.js Web 应用。

## 架构与模块

### 服务入口（cmd/）

- `cmd/nonoka-im` — 网关主服务：gRPC + HTTP server、WebSocket server、Redis 心跳节点注册，wire 依赖注入。
- `cmd/msgworker` — Kafka 消费者：消费 `im-messages` → 生成 topic_seq / msg_id → MongoDB 落库 → 推回在线客户端。
- `cmd/agent-worker` — AI agent worker：消费 Kafka `agent-events`，用 CloudWeGo Eino（通过 Bifrost 的 OpenAI 兼容 API）执行 agent，经 OpenFGA 鉴权后通过内部 HTTP 把回复发回 IM。使用 `-conf configs/agent-worker.yaml` + Kratos `internal/conf`，环境变量只做覆盖与 secret 注入。消费与执行解耦：consumer 只把触发事件落成 queued run（`agent_runs`），模型执行由 run worker 池（`AGENT_RUN_WORKERS`，默认 8）经 queued-run 重放拉起；stale running run 由 sweeper 回收（`AGENT_STALE_RUN_TIMEOUT`/`AGENT_MAX_ATTEMPTS`）；探针端点由 `internal/server.ProbeServer` 提供 `/healthz`、`/readyz`、`/metrics`。写类 MCP 工具会中断为卡片审批（`MSG_TYPE_AGENT_CARD`），经 `/v1/agent-approvals/{id}/approve|reject` 后从 PostgreSQL checkpoint 恢复。Agent 只引用扁平 `model_catalog` 的 `model_id`，不保存 `base_url`/`api_key`；Bifrost 是唯一 LLM gateway，per-Agent virtual key 在首次发布时创建、模型变更时更新 allowlist、删除 Agent 时回收（失败进 `gateway_key_cleanup`）。Nonoka 负责 user/agent 每日 runs/cost/tokens、Agent 并发与 max tool turns；Bifrost 负责 provider 路由、per-Agent 月度硬预算与全局 customer 预算。平台管理员身份存 OpenFGA `platform:global#admin`，启动时由 `AGENT_ADMIN_USER_IDS` 幂等写入。MCP/Skill 支持用户自助登记与上传；共享/公开资源需要管理员审核，共享 Agent 必须使用 per-user MCP connection，私有 MCP/Skill 不允许绑定到已共享 Agent。Agent 管理 UI 位于 `/agents`（目录/向导）与 `/agents/{id}?tab=overview|build|tools|runs`；分享收在 header 的 Share 弹窗，版本历史收在 header 的 History 弹窗，平台管理/配额/审计/DLQ 在 `/admin`。Agent 回答通过 `agent_run_citations` 记录检索来源，聊天 UI 渲染 source chips；Run detail 展示 trace link、cost 与 sources。OTLP trace 默认在 demo 中发往 Jaeger（`NEXT_PUBLIC_TRACE_URL_TEMPLATE`）。草稿测试入口已移除，Agent 只能发布后在真实 IM 会话中运行。Access 角色为 viewer/editor/owner，OpenFGA 使用 `editor`/`shared_group_editor`，owner 独占删除与分享变更。Streaming 使用 `port.StreamEvent` → `realtime.RunEvent` → Gateway `CMD_AGENT_RUN_EVENT`；Redis 保存每个 run 最近 500 条事件（15m TTL）供 `GET /v1/agent-runs/{id}/events?afterSeq=` 重连回放，非 delta 事件落 `agent_run_steps`。Gateway 内部 `PushRunEventToUser` 使用 `INTERNAL_PUSH_TOKEN`。敏感动作写入 append-only `audit_events`（before/after 白名单脱敏，不记录 prompt/正文/密钥）。
- `cmd/search-index` — 一次性/运维工具：从 MongoDB 重建可选 Meilisearch 索引。
- `cmd/authz-migrate` — 一次性工具：向 OpenFGA 写授权模型 / 历史数据回填。

### 核心包（internal/）

- `gateway/` — WebSocket 连接/会话管理、上行消息 Kafka producer、节点注册表。
- `service/` — Kratos transport 层：auth、message、group、user、conversation、dispatch、push、file，以及按资源拆分的 agent/run/approval/mcp/skill handler。**只做 DTO ↔ domain/app 转换，不直接依赖 `data` 或存储实现。**
- `biz/` — 认证等共享 usecase 与消费侧 repository 接口；IM 业务用例在 `internal/im/app`。
- `data/` — 数据访问：PostgreSQL repos（gorm）、Redis、MongoDB 装配；按 `im` / `agent` / `platform` 子域组织。repository 接口由消费侧定义，data 只做实现和 domain↔row 映射。
- `im/` — IM bounded context：`domain`（纯实体）、`app`（Message/Query/Push/Outbox/Conversation/Group/File usecase + ports）、`storage/mongo`（Mongo repository/索引/附件）、`search`（Mongo fallback / Meilisearch 客户端、索引 outbox relay、reindex）、`consumer`（Kafka adapter + Kratos worker）、`push`（realtime 适配器 + 重试队列）、`seq`、`group`。`internal/msgworker` 已删除。消息搜索按 topic 授权隔离；默认 `search.backend=mongo`，配置 Meilisearch 后可切换；未启用搜索时 API 返回 `SEARCH_UNAVAILABLE`，Agent 不暴露 `search_messages`。自定义后端接口与接入方式见 `docs/search-backend.md`。
- `agent/` — Agent 平台，已按 ports & adapters 组织：`domain`（纯实体/值对象）、`port`（Runtime 端口 + Run/Resume DTO + `ModelEndpoint` + 错误分类）、`app`（Harness/审批/finalize）、`eino`（Eino 适配器）、`tools/catalog`（工具元数据/风险/默认策略/CapabilityPreset/输入 schema/脱敏摘要）、`tools/builtin`（内置工具实现）、`mcp`（受控 MCP 连接、发现、SSRF Guard、per-user 凭据、健康检查）、`skills`（Skill middleware Backend）、`skillspec`（SKILL.md 解析/校验）、`store`（Eino CheckPointStore 的 PostgreSQL 适配）、`worker/executor/sweeper`（job runtime）、`event`（Kafka event/outbox 类型）、`credential`（JWT/api_key 加密）、`metrics`。内置工具 9 个：search_messages、get_conversation_context、send_message、read_file、get_topic_info、get_my_info、list_my_groups、get_group_members，以及隐藏系统工具 get_current_time。
- `llm/` — 平台 LLM 接缝：Bifrost 管理 API 客户端、扁平 `model_catalog` 同步/策展、运行时模型端点解析，以及 per-Agent virtual key 与 catalog key 的生命周期和回收队列。Provider key、路由和预算统一在 Bifrost；Nonoka 不维护第二套 LLM 控制面。
- `audit/` — append-only 审计 usecase：actor/action/resource/before-after（白名单脱敏）/request-id/trace-id/ip；PostgreSQL adapter 在 `data/platform`，保留期清理由 agent-worker 执行。
- `realtime/` — 共享 Gateway 路由 / Pusher / RunStream 契约；连接与推送适配器。
- `messaging/` — IM 集成契约：根包只放 DTO/接口；`messaging/im`、`messaging/mongo`、`messaging/httpclient`、`messaging/kafka` 放适配器。agent 不传递依赖 IM storage。
- `kafka/` — 共享 Kafka consumer/publisher、trace、DLQ 约定。
- `authz/` — OpenFGA 封装（`model.json` 为授权模型）。
- `server/` — Kratos server 装配、限流中间件；`conf/` — proto 定义的配置 schema；`metrics/` — Prometheus；`topic/` — topic 命名（`p2p_` / `grp_` / `sys_`）。

### 其他

- `pkg/sdk/` — Go SDK（三层：HTTP Service → RealtimeClient WebSocket → ConversationManager），有自己的 README 和 DESIGN.md。
- `web/app/` — 官方前端：Next.js 16 + React 19 + TypeScript + Tailwind v4 + shadcn/ui + zustand，pnpm 管理。**注意 Next.js 16 与训练数据中的旧版本有破坏性差异，改前端前必读 `web/app/AGENTS.md`。**
- `examples/chatapp/` — SDK 示例聊天应用。
- `api/im/v1/*.proto` — API 定义，同时生成 gRPC 与 HTTP 接口；`third_party/` 放第三方 proto。
- `bin/` — 构建产物目录，**不要手改**。
- `docs/` — 设计与审查文档（implementation_plan.md、PERFORMANCE_TUNING.md、phase0-* 等，多为历史记录；架构与阶段以 `REFACTOR.md` 和本文件“架构规范”为准）。

## 架构规范（必须遵守）

### 项目形态

nonoka-im 是 **模块化单体 + 异步 job worker**，不追求网络微服务化。Kratos 是工具箱，不是拓扑要求；不要把“像 Kratos”理解为“每个进程都要有 HTTP/gRPC service/server 和 registry”。

| 应用 | Kratos 类型 | 说明 |
| --- | --- | --- |
| `cmd/nonoka-im` | `interface` / `service` | HTTP + gRPC + WebSocket；使用 `kratos.New`、wire、`internal/conf`、`internal/server`、`internal/service` |
| `cmd/msgworker` | `job` | Kafka `im-messages` consumer；transport 是 Kafka，不是 HTTP/gRPC |
| `cmd/agent-worker` | `job` | Kafka `agent-events` consumer + run worker + sweeper |
| `cmd/nonoka-migrate` / `cmd/authz-migrate` | `task` | 一次性迁移/回填工具 |

### 分层与依赖方向

```text
client -> DTO -> service -> DO -> biz/app -> DO -> data -> PO -> storage
```

- `internal/service`：只做 transport 适配与 DTO ↔ domain/app 转换；**不得 import `internal/data/*`、`internal/msgworker`、`internal/im/storage`**。
- `internal/biz` / `internal/agent/app` / `internal/im/app`：usecase、领域对象、repository 接口、业务错误；不依赖 transport 或 storage 实现。
- `internal/agent/domain`：纯实体/值对象；**不得 import gorm、mongo、eino、net/http**。
- `internal/agent/port`：应用拥有的端口（Runtime 接口 + Run/Resume DTO + 错误分类）；不得 import `eino`/`data`。
- `internal/data/*`：repository 实现、row model、mapper、存储错误翻译；不得 import `service`/`api`。
- `consumer` / `gateway` / `eino` / `mcp` / `skills`：适配器；调用 app/biz，不写业务分支。
- `cmd/*`：composition root；只做配置、DI、lifecycle，不写业务逻辑。

硬性规则：

- `agent/app` 不 import `agent/eino`、`data/*`、`msgworker`、`filestore`。
- `service` 不 import `data/*`、`msgworker`；需要数据时通过 biz/app 接口。
- `data/im`、`data/agent`、`data/platform` 之间不要互相 import 具体 Store；跨子域通过 domain/port。
- 新增 job 代码：`internal/<domain>/app` + `internal/<domain>/consumer`；不要再把逻辑堆进 `cmd/*/main.go`。

### 新功能落点

- **新 HTTP API**：`api/...proto` → `make api` → `internal/service` DTO 转换 → `internal/biz`/`app` usecase → `internal/data` repository。
- **新 Agent 工具**：`internal/agent/tools/catalog` 加 `Definition`（category/risk/default policy/side effect/summarize）→ `internal/agent/tools/builtin` 加 handler（或 `mcp`）→ 测试。不要在 `eino` 里写 `switch toolName`。新工具必须用 `utils.InferTool` + typed request structs；先查 Eino ADK/middleware，不要手写第二个 agent loop。审计见 `docs/agent-eino-audit.md`。
- **新 Agent 能力/策略**：优先放 `agent/domain`（纯规则）或 `agent/app`（编排）；`eino` 只做适配。
- **新 Kafka job**：`cmd/<job>` 只装配；消费/执行/重试/清理拆到 `internal/<domain>/{consumer,app,worker}`。
- **新存储**：row model + mapper 放 `internal/data/<domain>`；domain 保持无 tag；错误在 adapter 内翻译成 domain error。

### Job 应用的 Kratos 接入方式

Kafka job 不需要伪造 HTTP/gRPC service，但应统一 lifecycle/config/log/metrics：

```go
type WorkerServer struct{ /* consumer, app service, cleanup */ }

func (s *WorkerServer) Start(ctx context.Context) error { return s.consumer.Start(ctx) }
func (s *WorkerServer) Stop(ctx context.Context) error  { return s.consumer.Stop() }

app := kratos.New(
    kratos.Name("msgworker"),
    kratos.Server(workerServer), // transport.Server 接口
)
```

- 不要给 Kafka job 注册 registry；consumer group 本身就是发现/负载均衡。
- `msgworker` / `agent-worker` 复用 `internal/conf`、`internal/metrics`、Kratos log/lifecycle；环境变量只做覆盖与 secret 注入。
- HTTP/gRPC 的 auth/rate-limit/recovery/tracing 用 Kratos middleware；WebSocket 握手单独认证。

### 架构测试

修改边界相关代码时，优先补/更新 architecture tests（`go list -deps` 或 `golang.org/x/tools/go/packages`），至少覆盖：

- `agent/domain` 不依赖 gorm/mongo/eino；
- `agent/app` 不依赖 `eino`/`data`/`msgworker`；
- `service` 不依赖 `data`/`msgworker`；
- `data` 不依赖 `service`/`api`。

### 当前迁移状态

- Agent 子域已按 `domain/port/app/eino/tools/{catalog,builtin}` 分层；新增 Agent 代码遵循此结构。
- Phase 0.5 已完成：`internal/msgworker`、`internal/filestore` 已删除；IM 代码在 `internal/im/...`，messaging 接缝为接口 + 适配器，job 应用统一使用 `kratos.App` + probe server。
- Phase 1/2 已完成：Bifrost Model Service（控制面 + 推理面）、Tool Registry/CapabilityPreset/`auto|ask|deny`、MCP 自助（发现/认证/SSRF/健康/审核）、Skill 版本/校验/审核。SDK MCP/Skill 模块按决策推迟到 Phase 3。
- 架构约束由 `test/architecture/architecture_test.go` 自动检查（`make test-arch`）；详细说明见 `docs/architecture.md`。
- 详细阶段规划见 `REFACTOR.md`。

## 工具链与常用命令

Go 1.25，Kratos v2.9.2，wire（DI），gorm + PostgreSQL，go-redis v9，mongo-driver v2，segmentio/kafka-go，gorilla/websocket，OpenFGA。前端 pnpm 10 + Node 20。

### 分支命名

- 分支名不要带阶段标签（`p0` / `p1` / `phase1` 等），阶段属于 TODO 文档，不属于 git 历史。
- 使用 `feat/<topic>`、`fix/<topic>`、`refactor/<topic>`、`test/<topic>` 或 `chore/<topic>`，例如 `feat/trust-security`。

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
make test-deps-up    # 只起依赖（postgres:5433 redis:6380 mongo:27018 kafka:9092 openfga:8081 meilisearch:7700）
make test-auth / test-gateway / test-gateway-concurrent / test-kafka   # 分组跑
make test-release-gate # 发布前 claim/sweeper/outbox/shutdown/search 门禁
make search-reindex  # 从 MongoDB 重建可选 Meilisearch 索引
make test-deps-down  # 清理
```

集成测试代码在 `test/integration/`（含 auth / gateway / kafka / IM consumer / offline_sync / dispatch / sdk_* 等）。新增功能应在该目录补充对应的集成测试。

### AI / agent 相关测试

agent 相关功能使用真实 LLM 测试：API Key 从项目根目录 `.env` 的 `DEEPSEEK_API_KEY` 读取（OpenAI 兼容接口，默认模型 DeepSeek）。agent 实连测试在 `test/agentlive/`。真实公网 MCP（DeepWiki/Context7）测试用 `NONOKA_MCP_LIVE=1` 开启，`TestDeepSeekCallsExternalMCPTool` 同时需要 `NONOKA_AGENT_LIVE=1`。**不要把 key 写进代码或提交 `.env`。**

## 新功能开发流程（必须遵守）

1. **先理解现状再动手**：读现有代码与目录结构，了解已有实现（本仓库已建 CodeGraph 索引，优先用 CodeGraph 探索，见文末）。涉及不熟悉的库/组件/方案时，可 web search 同类开源项目与官方文档，参考别人的实现方式与组件用法。
2. **先审视、后实现**：动手前审视当前实现，若发现严重问题（架构缺陷、明显 bug、安全隐患等），先向用户提出并讨论，达成一致后再重构/优化。不要直接埋头写新代码或绕开问题。需要补测试时，在现有测试架构（`test/integration/` 等）基础上补充。

## 完成前后端功能后的验收流程（必须执行）

功能开发完成后，**不能只跑测试就结束**，必须做端到端验收：

1. `make demo` 启动完整栈（postgres / redis / mongo / kafka / openfga / nonoka-im / msgworker / agent-worker / web，前端在 http://localhost:3001，后端 HTTP 8000）。agent-worker 默认随栈启动；未配置 `DEEPSEEK_API_KEY` 时 Bifrost 没有可用 provider，Agent 无法运行（日志告警），不影响 IM 功能。
2. 确认各容器正常启动。
3. 用 **Playwright** 在浏览器里实测：注册 / 登录等基本流程 + 本次新实现的功能，确认真实可用。
4. 有问题先看日志排查：`docker compose -f docker-compose.yml logs <service>`（如 `nonoka-im`、`msgworker`、`web`、`agent-worker`），或按需看其他日志。
5. 验收完成后 `make demo-down` 清理。

多网关场景用 `make demo-scale`（双 gateway + nginx LB(18000) + 3 msgworker + agent-worker，前端仍在 3001）；运行前需先 `make demo-down`，两者端口冲突。所有运行配置统一放在 `configs/`（demo 用 `config.docker.yaml`，scale 用 `config.scale.yaml` + `nginx-scale.conf`）。

## 代码约定

- 提交前：`go vet ./...` 通过、gofmt 干净、相关集成测试通过。
- 改 `api/im/v1/*.proto` 后：`make api`；前端用到的话再跑 `web/app` 的 `pnpm proto:gen`。
- 改依赖注入后：`make generate` 重新生成 wire 代码。
- 目录边界：service 只做协议适配，业务进 `biz`/`app`，IM 存储在 `internal/im/storage/mongo`，平台存储进 `data`。完整规则见上文“架构规范”和 `docs/architecture.md`。
- 文档（docs/）多为历史设计稿，与代码冲突时以代码为准。

<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tools** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them. `codegraph_node` returns one symbol's source + callers, or reads a whole file with line numbers. If the tools are listed but deferred, load them by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` and `codegraph node <symbol-or-file>` print the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->
