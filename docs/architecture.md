# nonoka-im Architecture

> Status: Phase 0.5 baseline. This document describes the enforced package
> boundaries, application types and migration state. It is the source of truth
> when `REFACTOR.md` and code disagree.

## 1. Bounded contexts

| Context | Packages | Responsibility |
| --- | --- | --- |
| IM | `internal/im/{domain,app,storage/mongo,consumer,push,seq,group}` | message persistence, sequencing, delivery, receipts, conversations, groups, agent trigger outbox |
| Agent | `internal/agent/{domain,port,app,app/admin,eino,tools,mcp,skills,store,worker,executor,sweeper}` | agent configuration, run state machine, Eino runtime, tools, approvals |
| Platform | `internal/data/platform`, `internal/httpapi` | MCP/skill registry, idempotency, admin contracts |
| Gateway | `internal/gateway`, `internal/server`, `internal/service` | HTTP/gRPC/WebSocket transport adapters only |
| Shared adapters | `internal/kafka`, `internal/realtime`, `internal/messaging`, `internal/authz`, `internal/metrics` | Kafka, realtime push, IM integration ports, OpenFGA, metrics |

## 2. Application types

| Binary | Kratos type | Transport | Registry |
| --- | --- | --- | --- |
| `cmd/nonoka-im` | `interface` / `service` | HTTP + gRPC + WebSocket | yes |
| `cmd/msgworker` | `job` | Kafka consumer + probe server | no |
| `cmd/agent-worker` | `job` | Kafka consumer + run pool + probe servers | no |
| `cmd/nonoka-migrate` / `authz-migrate` | `task` | in-process one-shot | no |

Both job binaries are started with `kratos.New` and implement
`transport.Server`:

- `internal/im/consumer.Worker` — Kafka consumer, delivery pool, outbox relay.
- `internal/agent/worker.Worker` — Kafka consumer, queued-run replay, executor
  pool, sweeper.
- `internal/server.ProbeServer` — `/healthz`, `/readyz`, `/metrics`.

Kafka consumer groups provide partition assignment and load balancing. Job
applications do not register with a service registry.

## 3. Dependency direction

```text
cmd/*                  composition only
  -> internal/service  DTO <-> app usecases
  -> internal/gateway  WebSocket protocol/session/dispatch
  -> internal/im/app   IM usecases + ports
  -> internal/agent/app Agent usecases + ports
  -> internal/agent/app/admin  agent management usecases
  -> internal/*/domain pure entities

adapters:
  internal/im/storage/mongo -> internal/im/domain + app ports
  internal/im/consumer      -> internal/im/app + internal/kafka
  internal/im/push          -> internal/im/app + internal/realtime
  internal/data/*           -> domain/ports
  internal/agent/eino       -> internal/agent/port + tools
  internal/messaging/*      -> contracts + adapters
```

Forbidden dependencies are enforced by `test/architecture/architecture_test.go`:

- `service` -> `data/*`, `msgworker`, `filestore`
- `agent/app` -> `agent/eino`, `data/*`, `msgworker`, `filestore`
- `agent/domain`, `im/domain` -> gorm, mongo, eino, `net/http`, generated API
- `im/app` -> storage/consumer/push adapters, `data`, `msgworker`
- `gateway` -> `data/*`, `im/storage`
- `internal/messaging` root -> storage/data adapters
- `data/*` -> `service`, `api/im/v1`, `msgworker`

## 4. IM message flow

```text
WS/HTTP -> im/app.MessageService.Send -> Kafka im-messages
Kafka -> im/consumer.Worker -> im/app.MessageService.HandleIncoming
                              -> im/storage/mongo (message + agent outbox)
                              -> conversations (PostgreSQL)
                              -> im/app.PushService -> gateway gRPC
                              -> im/app.OutboxService -> Kafka agent-events
```

- Mongo collection names and document shapes are unchanged from the previous
  `internal/msgworker` implementation.
- `domain.Message` is the boundary type; protobuf mapping happens in
  `internal/im/consumer`, BSON mapping in `internal/im/storage/mongo`.
- `messaging.MessageEvent` is the cross-service agent trigger contract.
- `im/app` owns sequencing decisions, bot-trigger policy and conversation
  fan-out; storage adapters only persist.

## 5. Messaging seam

```text
internal/messaging/            DTOs + ports only
internal/messaging/im/         adapters over IM application services
internal/messaging/mongo/      attachment adapter over the IM file store
internal/messaging/httpclient/ internal HTTP reply sender
internal/messaging/kafka/      agent-event publisher
```

Agent tools depend on `messaging.Reader`, `messaging.FileReader` and
`messaging.OutboxAdmin`, never on the IM storage implementation.

## 6. Configuration and lifecycle

- Both jobs use Kratos `config` with a `-conf` flag and `internal/conf`.
- `configs/agent-worker.yaml` uses the same `Bootstrap` schema as the gateway.
- Environment variables only override YAML values and inject secrets.
- `/healthz` is liveness; `/readyz` checks the Kafka consumer's first read and
  PostgreSQL/MongoDB ping; `/metrics` exposes Prometheus.

## 7. Migration state

- Phase 0.5 completed and reviewed (2026-11-21): `internal/msgworker` removed,
  service/gateway converged on app usecases, agent management moved to
  `agent/app/admin`, messaging seam completed, job lifecycle/config unified,
  architecture tests added.
- Phase 1 will add the Model Service and Tool Registry. New API/storage must
  follow the rules above; do not reintroduce transport -> storage shortcuts.
