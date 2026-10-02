# Nonoka IM Agent Harness 分阶段实施计划

> 状态：阶段 0-2 实施基线（2026-10-01）。本文件说明实施顺序、模块边界和验收方式；总体设计见 [Agent 架构](agent-architecture.md)。阶段 0-2 的核心路径已落地，阶段 3-6 仍按下文计划推进。每阶段完成后记录实际测试结果，再进入下一阶段。

## 1. 已有基础与目标

当前链路是 `MsgWorker -> MongoDB outbox -> agent-events -> cmd/agent-worker -> Harness -> Eino ADK -> IM HTTP send`。已有单个预置 Agent、`AgentRun/Session/Turn`、事件幂等、JWT、topic ACL、三个 IM 工具和 OTel。`Harness.Handle` 还同时承担鉴权、上下文裁剪、运行状态和回复提交；Agent 身份来自环境变量，`Runtime` 只有 `Run(Input) -> Text`。

本计划交付多 Agent 管理、会话上下文和 Eino 摘要、OpenFGA 分享/资源鉴权、MCP 和 Skill、卡片审批与恢复、可靠性和可观测性。这里的 **Context** 是对一次模型调用输入的构建；**Session** 是 Agent 与 actor 在 topic 中的会话索引；**Checkpoint** 是一次中断执行的恢复状态。不建立跨会话长期用户 Memory，也不把摘要当成用户事实库。

### 设计不变量

1. Eino 负责模型循环、middleware、工具执行事件、摘要和 Interrupt/Resume；独立权限模块以 OpenFGA 为唯一授权引擎；Nonoka 其余模块负责身份输入、业务状态和 IM 交互。
2. `owner_user_id`、`actor_user_id`、`bot_user_id` 各有不同用途。工具读取的权限主体是本次 actor，Agent owner 的资源权限不能被继承。
3. Agent 分享只授予调用资格。Agent、IM 和工具调用方提交经过验证的 actor、Agent、工具、当前 topic 与目标资源给权限模块；所有授权关系及允许/拒绝决策集中在 OpenFGA，执行前重新检查。审批状态由审批模块保存，批准本身不增加权限。
4. IM 回复继续走 HTTP/Gateway、Kafka、MsgWorker 和 MongoDB；Agent 执行不占用 IM consumer。每个外部副作用有稳定幂等键。
5. Eino checkpoint、业务 approval、可见聊天消息和会话摘要分别存储。模型输出、Skill 文档、MCP 结果都视为不可信内容。

## 2. 技术选型与接口

| 能力 | 选型与职责 |
| --- | --- |
| Agent Runtime | 项目锁定的 Eino `v0.8.4` ADK `ChatModelAgent`/`Runner`；外层保持项目自定义 `Runtime` 接口。 |
| Context 摘要 | Eino `adk/middlewares/summarization`；Nonoka `context` 包负责历史选取、预算、摘要持久化与失败回退。 |
| Skill | Eino `adk/middlewares/skill` 的 `NewMiddleware` 和受控 `Backend`，首期仅版本化、经发布的 `SKILL.md`，默认 inline 加载。 |
| MCP | 官方 `eino-ext/components/tool/mcp` 适配器；通过受控 MCP registry 建连、发现、筛选工具，再接入 Eino ToolsNode。需新增兼容当前 Eino 的独立模块依赖，先做版本验证。 |
| 授权 | 独立 `internal/authz` 模块 + OpenFGA（PostgreSQL 后端）；OpenFGA 是权限关系和决策的唯一来源，模块提供检查、关系变更、模型版本及对账接口。业务模块不直接访问 OpenFGA 或维护平行 ACL。 |
| Checkpoint/Approval | PostgreSQL 持久化 Eino `CheckPointStore` 字节及独立审批业务状态；Redis 只做可选缓存/限流。 |
| IM 审批 | 新 Agent 卡片消息类型 + Web 卡片按钮 + 已认证 action API；按钮提交的参数不直接执行工具。 |
| 观测 | 现有 OTel 扩展到 Run、Context、Tool、MCP、审批；日志/trace 默认不含正文、工具参数和凭据。 |

### 包边界

```text
internal/agent/app/       事件编排、Agent 解析、回复、审批编排
internal/agent/context/   历史选取、token 预算、摘要快照
internal/agent/domain/    Agent/Session/Run/Approval 的业务类型与状态机
internal/agent/runtime/   与 Eino 无关的 Run/Resume 输入和结果
internal/agent/eino/      Runner、middleware、checkpoint 和事件转换
internal/agent/tools/     本地工具与统一 ToolGuard；调用 authz，不写授权规则
internal/agent/mcp/       受控 server registry、连接、工具映射与结果约束
internal/agent/skills/    已发布 Skill 的绑定和 Backend
internal/agent/store/     PostgreSQL repositories 与 checkpoint 适配器
internal/agent/consumer/  Agent 事件调度、重试与 DLQ
internal/authz/           独立权限模块：模型、Check/Write API、关系变更与对账
```

不要求一次性创建空包。`consumer -> app -> runtime` 是调用方向；`app` 不引用 Eino/MCP 的具体类型。会话、Run、Approval 的业务 repository 接口定义在消费侧，存储实现放在 `store`。

`internal/authz` 是唯一允许调用 OpenFGA SDK 的包，向 HTTP/WS、文件、Agent 管理、Harness 和 ToolGuard 提供同一个接口。它接收可信主体与资源标识，在内部完成多项 OpenFGA Check 的组合或使用 OpenFGA model 的交集关系；业务模块只处理返回的允许/拒绝。现有 `CanAccessTopic`/`CanReadFile` 实现作为迁移适配器，完成切换后不保留 PostgreSQL ACL 后门。调用点必须存在于服务端资源边界；独立模块不等于可以省掉调用点。

JWT 验证负责回答“是谁”；JSON/schema、消息长度和超时校验负责输入合法性；审批状态机负责“这次操作是否得到人工确认”。这些不是 OpenFGA 授权关系，也不能用一次 OpenFGA `allowed=true` 代替。

### 数据与 API 契约

- `agents`：`id`、`bot_user_id`、`owner_user_id`、名称、说明、状态、当前发布版本；`agent_versions` 保存不可变指令、模型引用、Context 预算和工具/Skill/MCP 绑定快照。密钥只用部署 Secret/环境变量引用，不进表。
- `agent_sessions`：`agent_id + actor_user_id + topic` 唯一；`agent_turns` 关联 `session_id/run_id/trigger_msg_id/reply_msg_id`；`agent_context_snapshots` 保存摘要正文、覆盖到的 turn 边界、版本和 token 统计。摘要只在本 session 内复用。
- `agent_runs`：唯一键 `(event_id, agent_id)`；持有 `agent_version`、`session_id`、状态、尝试次数、租约、trace ID 和最终消息 ID。Run 状态至少有 `queued/running/waiting_approval/completed/failed/cancelled`。
- `agent_checkpoints`：`run_id`、checkpoint ID、Eino 版本、字节、过期时间；`agent_approvals`：`run_id`、tool call ID、工具与参数摘要/哈希、审批人、状态、过期时间、消费时间。参数原文仅放受控执行状态，不能放普通卡片或日志。
- `agent_tools`、`agent_skill_bindings`、`agent_mcp_bindings` 记录工具/Skill/MCP 的发布内容与运行配置；谁可调用、管理或执行，由 OpenFGA tuples 决定。不建可独立判权的 `agent_access` ACL 表；管理页通过权限模块读写分享关系。
- 新增 `/v1/agents` 管理/列表/详情/发布/启停/绑定接口，`/v1/agents/{id}/runs`、`/v1/agent-runs/{id}` 查询接口，以及 `/v1/agent-approvals/{id}/{approve|reject}` 动作接口。所有请求通过现有 JWT；服务端从 claims 取用户身份。
- `Runtime` 增加 `Run`、`Resume` 两个操作，结果为 `completed/interrupted/failed` 的结构化事件，包含最终文本或 checkpoint/interrupt 摘要；不得再用“空回复”代表审批暂停。

群聊 Session 默认按 actor 隔离，避免一个成员的私有上下文被别的成员继承；群公共消息通过当前 topic 的授权工具读取。Agent 配置采用显式发布：编辑草稿不影响正在运行的版本，恢复运行使用原版本快照。首期一个 Agent 对应一个 bot 用户；多个 Agent 不共用 bot 身份。

## 3. 阶段实施

### 阶段 0：测试底座和安全基线

> 实施状态：已完成核心路径。OpenFGA、文件鉴权、loopback 测试栈和 DeepSeek live 入口已接入；完整集成套件仍需在 CI 中按 Kafka 场景分组运行。

**模块**：`test/agentlive` 测试入口、`internal/authz`、现有 HTTP/WS 消息与文件服务、测试环境配置。

1. 把 `docker-compose.test.yml` 扩展为 Agent 联调用的基础设施：现有 PostgreSQL、Redis、MongoDB replica set、Kafka 保留；新增 OpenFGA，使用测试 PostgreSQL 中独立数据库或 schema，并提供健康检查。不要将 OpenFGA 测试数据写入生产库。阶段 0 即提交版本化基础权限模型，包含 user、group、topic、file 以及阶段 2 所需的 agent owner/viewer/can_manage/can_use 关系；将现有群成员、topic 和文件 owner 关系迁入 OpenFGA，HTTP/WS/文件入口统一切到权限模块。首次创建 P2P 会话时由权限模块为规范化 topic 幂等建立双方关系，发送入口不再通过解析 topic 字符串直接放行。
2. 固定测试端口、数据库名、Kafka topic 和清理规则。现有 `test/integration/setup_test.go` 会截断测试表并清空 Redis，因此 live 测试必须使用独立测试项目/数据库；禁止对非测试配置执行清理。
3. 复核并补测 HTTP/WS send、pull、recall 的 topic ACL；文件访问改成基于文件资源和可见关系的检查接口，旧 owner-only 文件在迁移时保持 owner 可读。不能靠不可猜测 file ID 授权。同步修正 `TestFile_UploadDownloadRoundTrip`：当前服务要求 JWT，但该旧测试仍期望无认证下载成功，应改为携带 owner token，并单测无 token 返回 401。
4. 提供一个只读取 `.env` 中 `DEEPSEEK_API_KEY` 的 live test 配置加载器，日志只输出 key 是否存在；模型采用 `deepseek-chat`，Base URL 默认为 `https://api.deepseek.com/v1`。无 key 时 live suite 显式 skip，不能悄悄变成 mock。

**验收**：现有集成测试通过；OpenFGA 与旧群成员数据完成对账，越权 topic/file 请求被拒绝，OpenFGA 不可用时 fail closed；测试栈健康；一次真实 DeepSeek 最小调用返回非空响应。这个模型预检验证 key 和网络，不代表 Agent 功能已通过。

### 阶段 1：Run/Session/Context 与 Eino Runtime

> 实施状态：已完成核心路径。Session advisory lock、token budget、Eino 摘要 callback/snapshot、错误与 usage 记录及跨 Runtime 测试已接入。

**模块**：`domain`、`app`、`context`、`runtime`、`eino`、`store`。

1. 从 `Harness.Handle` 抽出 AgentResolver、RunCoordinator、ContextBuilder、ReplyPublisher，保留现有单 Agent 行为作为迁移基线。Session 负责会话索引，Run 负责单事件状态，Turn 只记录稳定对话结果。
2. 用模型输入预算替换当前字符预算。ContextBuilder 组装发布版本的系统指令、可用摘要、最近轮次、当前输入；保留输出和工具结果预算。长历史触发 Eino summarization middleware，摘要通过 callback/snapshot 存储并在下一次 Run 加载。因为 Runner 每次由业务创建，middleware 的内存状态不会自动跨 Run 持久化。
3. Runtime 映射 Eino `AgentEvent` 到项目结果；保留模型和工具执行回调；错误分类、超时和模型用量进入 Run。摘要失败时按固定规则仅保留最近轮次，不丢当前输入。
4. 建立每 session 串行执行或等价的数据库锁，避免两个群触发在同一 actor/session 内读到相同旧历史后乱序写入。

**验收**：真实 DeepSeek 私聊两轮可引用前文；人为构造长历史后确实触发摘要，下一轮跨 Worker 重启仍能读取摘要；不同 actor/topic 的 Context 不串用。摘要测试另用确定性的 fake model 验证边界与失败回退。

### 阶段 2：完整多 Agent 管理

> 实施状态：已完成核心路径。Agent 草稿/发布/启停、版本冻结、动态 Worker 分发、OpenFGA 分享、Run 查询和管理页面已接入。

**模块**：Agent 配置/版本 repository、管理 API、AgentResolver、Worker 动态装配、Web Agent 管理视图。

1. 实现创建、编辑草稿、发布版本、启停、工具/Skill/MCP 绑定和 private/user/group 分享；管理、列表可见与调用资格通过阶段 0 已建立的权限模块/OpenFGA 关系判断，不增加本地 ACL。
2. 触发分发按私聊 bot ID 或群 `mentioned_user_ids` 找到候选 Agent，先检查 bot 身份和 Agent 状态，再按 `(event_id, agent_id)` 认领 Run。一次群消息可以触发多个独立 Agent Run。未知 bot 或未发布 Agent 不运行。
3. 在配置发布时冻结系统指令、模型、预算和允许的工具集合；已启动/暂停 Run 始终恢复原版本。管理 UI 提供状态、分享和运行记录，不暴露模型密钥。

**验收**：用真实 DeepSeek 创建两个不同指令的 Agent，分别私聊和群 @，回复来自正确 bot 且遵循各自指令；编辑未发布草稿不影响当前 Run；禁用 Agent 不再触发。API 权限和版本竞态用确定性集成测试覆盖。

### 阶段 3：扩展 OpenFGA 到工具和跨资源访问

**模块**：`authz` 工具/资源关系模型与检查 API、文件可见关系、ToolGuard 授权入口。

1. 在阶段 0 的 OpenFGA model 上增加 `agent_tool#can_execute` 和 `file#viewer/can_read` 等关系；Agent group 分享在阶段 2 已通过 `group#member` 建模。private/user/group 是分享界面的配置方式，最终授权数据是 OpenFGA 关系。
2. 所有成员、Agent 分享、工具授权及文件可见关系变更均通过 `authz` 模块的写入 API。原 PostgreSQL 群成员/Agent 配置和 MongoDB 文件元数据继续服务列表、投递和展示，不参与允许/拒绝判断。为跨库写入提供待同步状态、重试和对账：新增授权最后生效，撤权先在 OpenFGA 生效；不因投影延迟放宽权限。
3. 权限模块提供 `CanManageAgent`、`CanUseAgent`、`CanAccessTopic`、`CanReadFile`、`CanExecuteTool`。`CanExecuteTool` 接收 actor、Agent 版本、工具、当前 topic 和目标资源；“已绑定工具”“actor 可访问资源”“操作限定当前 topic”等授权条件全部在权限模块内由 OpenFGA 检查/组合。ToolGuard 只做参数校验、调用该接口、限额和执行控制，不保留自己的 ACL 或 allowlist 判权。
4. 文件增加 owner、共享 topic/group 和显式 viewer 关系。下载、Agent read_file、消息中的附件访问使用相同 `CanReadFile` 路径。OpenFGA 查询超时/失败一律拒绝；恢复和副作用工具使用高一致性读取并重新检查。

**验收**：真实 DeepSeek 让共享 Agent 搜索本群消息成功、搜索其他群/owner 私聊消息失败；群成员撤销后再次调用失败；授权审批前后变化均能阻断工具。OpenFGA 服务不可用、tuple 延迟、重复投影用集成测试验证。

### 阶段 4：MCP 与 Skill

**模块**：`mcp` registry/connector、`skills` registry/backend、ToolGuard、Agent 管理绑定 UI/API、Eino middleware 装配。

1. MCP 首期由平台管理员登记可信 server，Agent owner 只能绑定已登记 server 的允许工具。优先用官方 `eino-ext/components/tool/mcp` 转成 Eino 工具；选定与 Eino `v0.8.4` 兼容的模块版本后锁定依赖。远程 MCP 采用受控连接和凭据引用；首期不接受用户任意 URL 或 stdio 命令。
2. MCP registry 保存 server ID、连接方式、工具名/版本、schema 摘要、风险类别和健康状态；连接池有超时、重连、结果大小限制。工具发现结果只是候选清单；实际暴露与执行均由权限模块检查已发布 Agent 版本的工具授权，新工具不会自动开放。MCP 工具经同一个 ToolGuard、OpenFGA 和审批状态机。
3. Skill 使用 Eino `adk/middlewares/skill.NewMiddleware`。平台发布经审核的版本化 `SKILL.md`，Agent 绑定具体版本；Backend 只列出/读取本 Agent 可见版本。默认 inline 模式；fork 模式等实际需要时再设计子 Agent 权限继承。
4. Skill 文本不能改变系统权限或自动添加工具。Skill 和 MCP 可以组合：Skill 指导模型如何使用某个已绑定 MCP 工具，但工具执行仍由服务端 Guard 决定。MCP Prompt/Resource 首期只在显式绑定和资源 ACL 完成后开放。

**验收**：测试栈启动一个本地只读 MCP fixture server；真实 DeepSeek 被引导调用已绑定工具并引用结果，未绑定工具即使被 server 发现也不可见。绑定一个测试 Skill 后，模型能按需加载并使用已授权工具；解绑或改版本后新 Run 不再看到旧 Skill。恶意 Skill 指令、越权 MCP 工具和超大结果用确定性测试验证。

### 阶段 5：卡片审批与恢复

**模块**：Eino CheckPointStore、ApprovalCoordinator、受控副作用工具、protobuf/消息链路、Web 卡片、审批 API。

1. 在 `MsgType` 增加 Agent 卡片类型，定义版本化 JSON payload；继续通过普通 IM 消息链路持久化、推送、离线补拉。卡片只有脱敏工具名、参数摘要、目标、审批状态和过期时间，不含 checkpoint 字节或凭据。
2. 选定需要审批的工具通过 Eino `StatefulInterrupt` 暂停，Runner 用 `WithCheckPointID` 写入 PostgreSQL CheckPointStore。Harness 解析 interrupt 后创建业务 approval，再发送卡片；Run 转为 `waiting_approval`，Kafka 消费完成，不占用 Worker。
3. Approve/Reject API 使用 JWT，并调用权限模块检查审批资格；审批模块负责原子状态转换，动作绑定 tool call ID、参数哈希及 Agent 版本。批准后通过权限模块重查工具和资源授权，再使用 Eino `ResumeWithParams`；拒绝时给工具明确拒绝结果。重复点击或重复投递不重复执行。
4. 首期审批人固定为发起 actor；Agent owner 不自动能批准别人的操作。没有卡片能力的客户端显示文本摘要及打开 Web 审批页的链接，不通过普通聊天文本命令执行审批。

**验收**：真实 DeepSeek 触发测试写入工具，审批前目标状态不变；点击卡片批准后恰好执行一次并收到最终回复；拒绝、超时、撤权、重复点击和 Worker 重启恢复均正确。用可观测幂等 fixture 验证副作用次数。

### 阶段 6：运行可靠性与运营

**模块**：Agent consumer、Run 租约/取消、重试/DLQ、限额、查询 API、OTel/指标、清理任务。

1. 每 session 顺序、不同 session 并发；设置 Agent/actor 并发、模型和工具超时、token/费用上限。区分可重试基础设施错误、不可重试权限/参数错误和等待审批状态。
2. 对 stale Run 用租约和 attempt ID 恢复；回复和工具副作用使用稳定幂等键。加 DLQ、取消、checkpoint/approval TTL、按保留期清理。
3. 增加 Run、Context、Eino model、ToolGuard、MCP、审批和回复提交 spans；日志/trace 不记录密钥、消息正文、附件或原始工具参数。提供运行状态与错误分类查询。

**验收**：真实 DeepSeek 连续运行私聊、群聊、MCP、Skill、审批四条路径；中途重启 Worker、重复投递 event、模拟模型超时后恢复，最终正式回复和副作用各不重复。指标和 trace 能关联同一 run，且无敏感正文。

## 4. 每阶段真实联调规范

每阶段都要有 **确定性测试** 和 **真实模型试用** 两层。数据库状态机、ACL、签名和幂等由单元/集成测试断言；真实模型测试只验证对应模块的用户可见链路，不依赖某句生成文本的精确匹配。每个阶段的模块在该阶段至少由一条 DeepSeek 驱动的端到端场景覆盖；纯存储/授权模块通过这条场景中的实际调用路径和独立确定性测试验证。

统一步骤：

```bash
docker compose -f docker-compose.test.yml up -d --wait
go test ./internal/agent/... ./internal/authz/... ./test/integration/... -count=1
NONOKA_AGENT_LIVE=1 go test ./test/agentlive/... -count=1 -timeout=10m
```

`test/agentlive` 是阶段 0 新增的测试入口，已接入 DeepSeek 两轮对话和跨 Runtime 摘要场景。它从仓库 `.env` 读取 `DEEPSEEK_API_KEY`，测试用例只记录模型名、轮次和通过状态，不记录 prompt 原文或 key。`docker-compose.test.yml` 只负责基础设施，不会自动启动这些 Go 进程；每阶段在 live 测试入口中按需启动。CI 默认运行确定性测试；真实模型 suite 在明确提供 key 的受控环境运行并单独标识费用。

允许覆盖 `DEEPSEEK_BASE_URL` / `DEEPSEEK_MODEL`，默认分别为 `https://api.deepseek.com/v1` / `deepseek-chat`。当前 Agent Worker 会读取 `DEEPSEEK_API_KEY`，因此不需要把 key 改名或提交到配置。MCP fixture 必须是本地可重复的只读服务；另设一条受控写入 fixture 走审批。Skill fixture 只使用测试文档。Agent live suite 需要设置每轮模型请求超时和最大输出 token，失败时保留脱敏的 Run/Trace 元数据供排查。

环境退出时可用 `docker compose -f docker-compose.test.yml down` 停服务；不要用 `down -v` 清理共享测试 volume，除非确定不需要保留其中数据。阶段验收表中注明实际运行命令、服务版本、Eino/eino-ext 版本、通过/失败案例、DeepSeek 模型及日期。

## 5. 依赖与发布顺序

1. 阶段 0、1 先让现有单 Agent 继续正常回复。数据库新增字段使用兼容迁移，旧 Run/Turn 能被读取。
2. 阶段 2 发布多 Agent 配置后，再将 Worker 从环境变量固定 Agent 切到动态解析；预置 Agent 作为迁移数据保留。
3. 阶段 0 先把现有 IM 授权及 Agent 基础关系切到 OpenFGA，阶段 2 多 Agent 管理和分享直接使用独立权限模块；阶段 3 扩展工具/跨资源关系。迁移校验期间可比较旧检查与 OpenFGA 决策，但正式放行只以权限模块的 OpenFGA 结果为准。
4. 阶段 4 MCP/Skill 由管理员登记和灰度绑定，默认所有现有 Agent 均未启用。
5. 阶段 5 卡片协议先支持客户端展示和只读历史，再开放副作用工具及审批动作。
6. 阶段 6 完成故障注入和费用/并发监控后再扩大可用范围。

本期不做长期用户 Memory、任意 MCP server 注册、公共 Skill 市场、多 Agent 自主协作、全量消息向量索引或 MinIO 迁移。

## 参考资料

- [Eino ADK Memory and Session](https://www.cloudwego.io/docs/eino/quick_start/chapter_03_memory_and_session/)：业务层管理会话历史。
- [Eino Interrupt/Resume](https://www.cloudwego.io/docs/eino/quick_start/chapter_07_interrupt_resume/)：Tool 中断和恢复。
- [Eino Skill Middleware](https://www.cloudwego.io/docs/eino/core_modules/eino_adk/eino_adk_chatmodelagentmiddleware/middleware_skill/)：Skill Backend 和 Handlers 装配。
- [Eino MCP Tool Adapter](https://github.com/cloudwego/eino-ext/tree/main/components/tool/mcp)：官方 MCP 工具适配。
- [OpenFGA modeling](https://openfga.dev/docs/modeling)：关系授权模型。
- [Codex Tool Orchestrator](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/orchestrator.rs)：工具策略和执行链路的参考。
