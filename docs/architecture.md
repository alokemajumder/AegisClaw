# AegisClaw Architecture

## Overview

AegisClaw is a microservices-based platform built in Go with a Next.js frontend. All services communicate through a combination of gRPC (synchronous), NATS JetStream (asynchronous), and a shared PostgreSQL database.

## System Architecture

```
                    ┌──────────────┐
                    │   Frontend   │
                    │  (Next.js)   │
                    │   :3000      │
                    └──────┬───────┘
                           │ HTTP
                    ┌──────▼───────┐
                    │  API Gateway │
                    │   (Chi)      │
                    │   :8080      │
                    └──────┬───────┘
                           │ gRPC
          ┌────────────────┼────────────────┐
          │                │                │
    ┌─────▼─────┐   ┌─────▼─────┐   ┌─────▼─────┐
    │Orchestrator│   │ Connector │   │  Evidence  │
    │  :9090     │   │  Service  │   │  Service   │
    │            │   │  :9093    │   │  :9092     │
    └─────┬─────┘   └─────┬─────┘   └─────┬─────┘
          │               │                │
    ┌─────▼─────┐   ┌─────▼─────┐   ┌─────▼─────┐
    │  Runner   │   │  Ollama   │   │  MinIO     │
    │  :9091    │   │  Bridge   │   │  (S3)      │
    │           │   │  :9095    │   │            │
    └───────────┘   └─────┬─────┘   └────────────┘
                          │
                    ┌─────▼─────┐
                    │  Ollama   │
                    │  :11434   │
                    └───────────┘

    ┌───────────┐   ┌───────────┐   ┌───────────┐
    │ Reporting │   │ Scheduler │   │ PostgreSQL │
    │  :9094    │   │  :9096    │   │  :5432     │
    └───────────┘   └───────────┘   └───────────┘

    ┌─────────────────────────────────────────────┐
    │            NATS JetStream :4222              │
    │  Streams: RUNS, AGENTS, EVIDENCE,           │
    │           CONNECTORS, APPROVALS             │
    └─────────────────────────────────────────────┘
```

## Services

| Service | gRPC Port | Health Port | Health Endpoint |
|---------|-----------|-------------|-----------------|
| API Gateway | — | 8080 | `:8080/healthz` |
| Orchestrator | 9090 | 10090 | `:10090/healthz` |
| Runner | 9091 | 10091 | `:10091/healthz` |
| Evidence Service | 9092 | 10092 | `:10092/healthz` |
| Connector Service | 9093 | 10093 | `:10093/healthz` |
| Reporting Service | 9094 | 10094 | `:10094/healthz` |
| Ollama Bridge | 9095 | 10095 | `:10095/healthz` |
| Scheduler | 9096 | 10096 | `:10096/healthz` |

### API Gateway (`:8080`)
- **Technology**: Go + Chi router
- **Role**: REST API (59 endpoints), authentication (JWT), RBAC (4 roles), rate limiting, CORS
- **Communicates with**: All internal services via gRPC
- **Health**: `:8080/healthz`

### Orchestrator (`:9090`)
- **Technology**: Go + gRPC + NATS
- **Role**: Manages agent lifecycle, run execution, policy enforcement
- **Communicates with**: NATS (pub/sub for agent tasks), all services via gRPC
- **Health**: `:10090/healthz`

### Runner (`:9091`)
- **Technology**: Go + gRPC
- **Role**: Execution of validation steps with cleanup verification (gVisor sandboxing planned for Phase 2, currently runs in-process)
- **Communicates with**: Orchestrator (task receipt), Evidence Service (artifact storage)
- **Health**: `:10091/healthz`

### Evidence Service (`:9092`)
- **Technology**: Go + gRPC + MinIO
- **Role**: Evidence vault CRUD, receipt storage, artifact management
- **Storage**: MinIO (S3-compatible) for blob storage
- **Health**: `:10092/healthz`

### Connector Service (`:9093`)
- **Technology**: Go + gRPC + Connector SDK
- **Role**: Connector lifecycle management, execution proxy, health monitoring
- **Communicates with**: External platforms (SIEM, EDR, ITSM, etc.)
- **Health**: `:10093/healthz`

### Reporting Service (`:9094`)
- **Technology**: Go + gRPC
- **Role**: Report generation in Markdown and JSON formats (PDF renderer planned)
- **Health**: `:10094/healthz`

### Ollama Bridge (`:9095`)
- **Technology**: Go + gRPC
- **Role**: LLM proxy with prompt governance, evidence anchoring, model allowlisting
- **Communicates with**: Ollama (`:11434`)
- **Health**: `:10095/healthz`

### Scheduler (`:9096`)
- **Technology**: Go + gRPC + cron
- **Role**: Engagement scheduling, blackout enforcement, run triggering
- **Health**: `:10096/healthz`

### Health Checks

All services expose HTTP health endpoints at `/healthz` (liveness) and `/readyz` (readiness). The API gateway serves its health checks on its primary HTTP port (`:8080`). All other services run a dedicated health HTTP server on a port offset of +1000 from their gRPC port (e.g., Orchestrator gRPC on `:9090`, health on `:10090`). The `/healthz` endpoint returns HTTP 200 with `{"status":"healthy","service":"<name>"}`. The `/readyz` endpoint checks database and dependency connectivity and returns HTTP 200 or HTTP 503. These endpoints are used by Docker health checks, Kubernetes probes, and the monitoring stack.

## Data Flow: Validation Run

```
1. Scheduler/User triggers engagement
   └─► NATS: runs.created

2. Orchestrator receives event
   └─► Creates Run record in PostgreSQL
   └─► Dispatches to Planner Agent via NATS

3. Planner Agent plans validation campaign
   └─► Returns ordered list of tasks
   └─► Orchestrator stores steps in PostgreSQL

4. For each step:
   a. Policy Enforcer validates against scope/tier/allowlist
   b. If Tier 2+: Approval Gate blocks for human decision
   c. Executor dispatches to Runner (sandboxed)
   d. Runner executes and reports results
   e. Evidence Agent stores artifacts in MinIO
   f. Telemetry Verifier queries SIEM/EDR via Connector Service
   g. Detection Evaluator checks alert generation
   h. Receipt Agent records step outcome

5. After all steps:
   a. Response Automator creates ITSM tickets
   b. Coverage Mapper updates ATT&CK matrix
   c. Receipt Agent generates signed run receipt
   d. Run marked complete in PostgreSQL
```

## NATS JetStream Streams

| Stream | Subjects | Retention | Max Age |
|--------|----------|-----------|---------|
| RUNS | `runs.>` | Limits | 30 days |
| AGENTS | `agents.>` | Work Queue | 24 hours |
| EVIDENCE | `evidence.>` | Limits | 90 days |
| CONNECTORS | `connectors.>` | Work Queue | 24 hours |
| APPROVALS | `approvals.>` | Limits | 7 days |

## Database Schema

See `internal/database/migrations/` for the complete SQL schema. Key tables:

- `organizations` — Tenant organizations
- `users` — Platform users with RBAC roles
- `assets` — Target inventory
- `connector_registry` — Available connector types
- `connector_instances` — Configured connector integrations
- `engagements` — Validation programs
- `runs` — Execution instances
- `run_steps` — Individual actions within runs
- `findings` — Security findings with lifecycle tracking
- `approvals` — Approval queue
- `audit_log` — Immutable audit trail
- `coverage_entries` — ATT&CK coverage matrix
- `policy_packs` — Configurable policy packs
- `reports` — Generated reports with type, format, and storage reference

## Kill Switch Flow

```
1. Admin triggers kill switch
   └─► API Gateway: POST /api/v1/admin/system/kill-switch

2. API Gateway:
   a. Sets in-memory kill switch state
   b. Updates all running runs to "killed" in PostgreSQL
   c. Publishes kill switch event to NATS (runs.killswitch)
   d. Writes audit log entry (persists across restarts)

3. Orchestrator:
   a. Receives NATS kill switch event
   b. Cancels all in-flight run contexts (goroutines)
   c. Before each step: checks killSwitch.IsEngaged()

4. On restart:
   a. API Gateway queries last kill switch event from audit_log
   b. Restores engaged/disengaged state
```

## Observability

- **Tracing**: OpenTelemetry SDK → Jaeger
- **Metrics**: Prometheus (via OTEL)
- **Logging**: Structured JSON via Go `slog`
- **Dashboards**: Grafana
