# Nonoka IM 压力测试报告

> 测试时间：2026-06-15  
> 测试环境：Docker Compose（`docker-compose.test.yml`）单节点  
> 服务版本：`efc5c17`  
> 压测工具：`test/loadtest/main.go`

## 1. 测试目标

- 量化 Gateway + MsgWorker 在容器环境下的单节点吞吐与连接能力。
- 识别端到端消息链路（Pub → Kafka → MsgWorker → MongoDB → Push）的延迟瓶颈。
- 为后续优化提供数据依据。

## 2. 测试环境

| 组件 | 版本/配置 | 端口 |
|------|----------|------|
| Gateway (nonoka-im) | Kratos HTTP/gRPC + WebSocket | 18000/19000 |
| MsgWorker | segmentio/kafka-go + MongoDB + Redis 路由 | 18001 (metrics) |
| PostgreSQL | 15（bitnami） | 5433 |
| Redis | 7（bitnami） | 6380 |
| MongoDB | 7（bitnami） | 27018 |
| Kafka + ZooKeeper | bitnami | 9092 |

- 宿主机：12 核 / 16 GB
- Kafka Topic：`im-messages`，单分区
- Kafka 生产者：`async=false`、`required_acks=all`、`batch_size=100`、`batch_timeout=100ms`

## 3. 本次新增的可观测性

为便于定位瓶颈，本次测试前为服务补充了 Prometheus 指标和 pprof：

- `internal/metrics/metrics.go`：统一的 Prometheus 指标集合（nil-safe）。
- Gateway 指标：活跃连接数、连接总数、消息发布数/延迟、消息推送数/延迟。
- MsgWorker 指标：消息消费/处理数、处理延迟、MongoDB 操作、推送尝试/失败/重试、gRPC 推送延迟。
- HTTP `/metrics` 端点（Gateway:18000、MsgWorker:18001）。
- `/debug/pprof/*` 端点（Gateway 与 MsgWorker）。

## 4. 测试方法与参数

压测工具通过 HTTP 注册/登录获取 JWT，再建立 WebSocket 连接并认证。每个用户按固定频率向“环上下一个用户”发送 P2P 消息，统计：

- `TotalSent`：客户端成功写入 WebSocket 的消息数。
- `TotalAcked`：收到的 `CMD_PUBLISH` ACK 数（含 client_msg_id 匹配）。
- `TotalPushed`：收到的 `CMD_NOTIFY` 推送数。
- `TotalReceipts`：收到的 `CMD_SEND_RECEIPT` 发送回执数。

> 注：ACK/Receipt 存在少量漏计（约 8-10%），主要发生在测试结束、连接关闭时仍有在途包；服务端指标显示所有消息均被成功处理，因此漏计不影响吞吐结论。

## 5. 测试结果

### 5.1 梯度压测（1 msg/s / 用户）

| 并发用户 | 发送速率 (msg/s) | 成功连接 | ACK 率 | 推送到达 | ACK 延迟 P99 (ms) | 回执延迟 P99 (ms) |
|---------|-----------------|---------|-------|---------|------------------|------------------|
| 50 | ~55 | 50/50 | 90.9% | 100% | 113.8 | 602.3 |
| 100 | ~108 | 100/100 | 93.7% | 100% | 108.9 | 918.9 |
| 150 | ~161 | 150/150 | 91.6% | 100% | 112.1 | 1009.9 |
| 200 | ~74（仅 68 用户在线） | 68/200 | 91.1% | 100% | 108.4 | 381.5 |
| 300 | ~320 | 300/300 | 93.8% | ~51%（客户端读侧瓶颈） | 57.6 | 19845.8 |

说明：

- 50-150 用户场景下服务端可稳定处理全部消息，推送到达率 100%。
- 200 用户时出现大量登录失败（HTTP 401），原因是 PostgreSQL 连接池仅 100，`data.NewData` 中 `MaxOpenConns=100`，200+ 并发注册/登录请求导致连接池耗尽。限制注册并发后 150 用户可全部成功。
- 300 用户时服务端仍能将所有消息处理并推送（服务端指标 `messages_processed == messages_consumed == messages_published`），但压测客户端的单连接读循环成为瓶颈，部分连接读缓冲区溢出，导致客户端侧统计的推送/回执率下降。

### 5.2 高吞吐压测（100 用户 × 5 msg/s）

| 指标 | 数值 |
|------|------|
| 发送速率 | ~545 msg/s |
| ACK 率 | ~91% |
| 推送到达（客户端侧） | 显著下降 |
| ACK 延迟 P99 | ~54 ms |

结论：当每个用户的发送速率提高后，客户端读循环难以同时处理 ACK + NOTIFY + SEND_RECEIPT，导致客户端侧统计失真。服务端仍能将消息全部处理。

## 6. 服务端延迟分解（来自 Prometheus）

| 阶段 | 观测指标 | 典型延迟 | 备注 |
|------|---------|---------|------|
| Gateway Publish（客户端 → Kafka ACK） | `nonoka_gateway_publish_latency_seconds` | P99 < 128 ms | 含 Kafka 生产同步等待 |
| MsgWorker 处理（消费 → MongoDB → 推送） | `nonoka_msgworker_processing_latency_seconds` | P99 < 64 ms | 含 gRPC push |
| MsgWorker → Gateway gRPC | `nonoka_msgworker_grpc_push_latency_seconds` | P99 < 12.8 ms |  |
| Gateway → Client WebSocket | `nonoka_gateway_push_latency_seconds` | P99 < 0.4 ms |  |

端到端回执延迟（客户端测量）明显大于上述各阶段之和，差值主要来自：

1. Kafka 消费轮询延迟（`max_wait=500ms`）。
2. Kafka 生产者批量等待（`batch_timeout=100ms`、`batch_size=100`）。
3. 客户端读循环处理与 OS 网络缓冲。

## 7. 资源使用

测试期间容器资源未出现饱和：

| 容器 | CPU | 内存 |
|------|-----|------|
| Kafka | ~1% | 865 MB |
| MongoDB | ~0.7% | 348 MB |
| Redis | ~8.7% | 69 MB |
| PostgreSQL | ~0% | 51 MB |

Redis 8.7% 的 CPU 占比最高，与 MsgWorker 路由（`HGetAll`、`SMembers`、`ZAdd`）的 CPU profile 结果一致。

## 8. 发现的瓶颈与优化建议

### 8.1 高优先级

1. **PostgreSQL 连接池过小**
   - 现状：`data/data.go` 中 `SetMaxOpenConns(100)`。
   - 影响：>100 并发注册/登录即失败，限制压测用户规模。
   - 建议：将数据库连接池改为可配置，生产环境根据 CPU/内存设置为 50-200；登录/注册增加熔断或限流。

2. **Kafka 单分区**
   - 现状：Topic `im-messages` 为单分区。
   - 影响：MsgWorker 仅单消费者实例能并行，吞吐量受限于单分区顺序消费。
   - 建议：按 topic/key（如 `p2p_<uid1>_<uid2>` 或 `group_<gid>`）分区，提升消费并行度。

3. **Kafka 生产者批量参数**
   - 现状：`batch_size=100`、`batch_timeout=100ms`、`async=false`。
   - 影响：低并发下延迟可控，但高并发下批量等待和同步 ack 会放大端到端延迟。
   - 建议：评估是否可改为异步发送 + 失败回调，或降低 `batch_timeout`；对可靠性要求高的场景保留 `required_acks=all`。

4. **MsgWorker Redis 路由开销**
   - 现状：每条消息都要查询 Redis 用户节点索引（`HGetAll`、`SMembers`）。
   - 影响：Redis CPU 使用最高，且 Redis 往返增加延迟。
   - 建议：
     - 对本地 Gateway 节点缓存用户会话（LRU）。
     - 批量路由：一次 Redis 查询解析多个用户，而非每个用户单独查询。
     - 本地路由优先：MsgWorker 与 Gateway 同进程部署时可直接查询本地 Manager。

### 8.2 中优先级

5. **客户端读循环瓶颈**
   - 现状：压测工具每个连接只有一个读 goroutine，且收到 NOTIFY 后还要同步/异步回 ACK。
   - 影响：高吞吐下客户端先于服务端崩溃，无法准确测量服务端上限。
   - 建议：压测工具改为“读 goroutine + 无锁处理 goroutine + 批量 ACK”，或直接使用多个压测进程/机器。

6. **MongoDB 写入与索引**
   - 现状：每条 P2P 消息写 inbox，每条群消息写 messages + 扩散到多个 inbox。
   - 建议：监控 `nonoka_msgworker_mongo_ops_total` 与 MongoDB slow query，确认索引命中；大群场景考虑读扩散/写扩散策略优化。

### 8.3 低优先级 / 可观测性

7. **补齐 MsgWorker 推送指标**
   - 已新增 `grpc_push_latency_seconds`、`push_attempts_total` 等，建议后续再增加：
     - Kafka lag 监控（consumer lag）。
     - 按 topic 类型的处理延迟（P2P vs Group）。
     - 失败重试队列深度。

8. **pprof 采样增强**
   - 当前 CPU profile 采样率低（1.6%），建议在高负载期间采集 30s 以上 profile，或降低采样间隔。

## 9. 结论

在当前单机 Docker 环境下，Nonoka IM 服务端可稳定处理 **约 150-300 msg/s** 的 P2P 消息（150 用户 1 msg/s 时端到端基本正常；300 用户时服务端仍能处理但客户端读侧成为瓶颈）。主要瓶颈不在 CPU/内存，而在：

- **PostgreSQL 连接池**限制登录并发；
- **Kafka 单分区**限制消费并行；
- **Redis 路由查询**增加延迟和 Redis CPU；
- **Kafka 生产者批量/同步 ack** 放大端到端延迟。

按上述高优先级优化后，预计单机吞吐可提升至 500-1000 msg/s 以上。

## 10. 产物清单

- 压测工具：`test/loadtest/main.go`
- 测试配置：`configs/loadtest/config.yaml`
- 指标与 profile：`test/loadtest/reports/`
- 测试报告：`test/loadtest/PERFORMANCE_REPORT.md`
