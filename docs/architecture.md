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

### API Gateway (`:8080`)
- **Technology**: Go + Chi router
- **Role**: REST API, authentication (JWT + SSO), RBAC, rate limiting, CORS
- **Communicates with**: All internal services via gRPC

### Orchestrator (`:9090`)
- **Technology**: Go + gRPC + NATS
- **Role**: OpenClaw engine — manages agent lifecycle, run execution, policy enforcement
- **Communicates with**: NATS (pub/sub for agent tasks), all services via gRPC

### Runner (`:9091`)
- **Technology**: Go + gRPC + gVisor
- **Role**: Sandboxed execution of validation steps with cleanup verification
- **Communicates with**: Orchestrator (task receipt), Evidence Service (artifact storage)

### Evidence Service (`:9092`)
- **Technology**: Go + gRPC + MinIO
- **Role**: Evidence vault CRUD, receipt storage, artifact management
- **Storage**: MinIO (S3-compatible) for blob storage

### Connector Service (`:9093`)
- **Technology**: Go + gRPC + Connector SDK
- **Role**: Connector lifecycle management, execution proxy, health monitoring
- **Communicates with**: External platforms (SIEM, EDR, ITSM, etc.)

### Reporting Service (`:9094`)
- **Technology**: Go + gRPC
- **Role**: Report generation in PDF, Markdown, and JSON formats

### Ollama Bridge (`:9095`)
- **Technology**: Go + gRPC
- **Role**: LLM proxy with prompt governance, evidence anchoring, model allowlisting
- **Communicates with**: Ollama (`:11434`)

### Scheduler (`:9096`)
- **Technology**: Go + gRPC + cron
- **Role**: Engagement scheduling, blackout enforcement, run triggering

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

## Observability

- **Tracing**: OpenTelemetry SDK → Jaeger
- **Metrics**: Prometheus (via OTEL)
- **Logging**: Structured JSON via Go `slog`
- **Dashboards**: Grafana
