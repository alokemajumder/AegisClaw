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
- **Role**: Manages agent lifecycle, run execution (3-phase pipeline), policy enforcement, connector resolution, coverage snapshots
- **Key components**: `AgentRegistry` (12 agents), `RunEngine` (full pipeline), `KillSwitch`, `Orchestrator` (NATS consumer)
- **Dependencies**: DB pool, NATS, ConnectorService, CoverageRepo, all agent deps
- **Communicates with**: NATS (pub/sub for run triggers + kill switch), PostgreSQL (runs, steps, findings, coverage), MinIO (evidence + receipts)
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

## Agent Squads

AegisClaw uses 12 autonomous agents organized into 4 squads. All 12 agents are wired into the `RunEngine` and called in sequence during every validation run.

| Squad | Agent | Role |
|-------|-------|------|
| **Governance** | PolicyEnforcer | **Fail-closed**: validates every step against tier policy, target allowlist, and exclusions. Blocks Tier 3. Gates Tier 2+ for approval. Requires non-empty allowlist for Tier 1+. Blocks all actions when PolicyContext is missing. |
| **Governance** | ApprovalGate | Creates DB-backed approval records for Tier 2+ actions. Blocks execution until human decision. |
| **Governance** | ReceiptAgent | Generates HMAC-SHA256 signed, tamper-evident run receipts with full step data, scope snapshot, and evidence manifest. Stores in MinIO. Generates findings when receipts are unsigned or signing fails. |
| **Emulation** | Planner | Loads validation playbooks from YAML, filters by allowed tiers, generates ordered step list. |
| **Emulation** | Executor | Executes playbook steps in-process with real operations (SIEM queries, EDR health checks, EICAR marker files, allowlisted commands, detection verification, cleanup checks). gVisor sandboxing planned. |
| **Emulation** | EvidenceAgent | Captures execution artifacts as JSON and uploads to MinIO evidence vault. |
| **Validation** | TelemetryVerifier | Queries SIEM/EDR connectors for expected telemetry matching the executed technique. |
| **Validation** | DetectionEvaluator | Queries EDR for alerts, measures detection latency, generates detection gap findings. |
| **Validation** | ResponseAutomator | Creates ITSM tickets for findings (with input sanitization against injection), sends notifications via Teams/Slack connectors. |
| **Improvement** | CoverageMapper | Upserts ATT&CK coverage entries from validation results, computes coverage percentage. |
| **Improvement** | DriftAgent | Compares post-run coverage against pre-run snapshot, generates drift findings for regressions. |
| **Improvement** | RegressionAgent | Compares findings between the current run and previous runs using composite fingerprinting (title+severity+technique IDs), detects new regressions. |

### Agent Dependency Injection

Agents receive dependencies via `agentsdk.AgentDeps` (fields typed as `any` to avoid circular imports). Each agent type-asserts the fields it needs in its `Init()` method:

- `Logger` — `*slog.Logger`
- `DB` — `*pgxpool.Pool` (for agents that need direct DB access)
- `EvidenceStore` — `*evidence.Store` (EvidenceAgent, ReceiptAgent)
- `ConnectorSvc` — `*connector.Service` (validation agents)
- `PlaybookLoader` / `PlaybookExecutor` — `*playbook.Loader` / `*playbook.Executor` (Planner, Executor)
- `ReceiptHMACKey` — `[]byte` (ReceiptAgent HMAC-SHA256 signing key)
- `ConnectorInstanceRepo` — `*repository.ConnectorInstanceRepo` (connector resolution)

## Data Flow: Validation Run

The `RunEngine.ExecuteRun()` method implements the full 3-phase agent pipeline:

```
Phase 1 — Planning
  1. Scheduler/User triggers engagement
     └─► NATS: runs.trigger.<org_id>
  2. Orchestrator receives event
     └─► Creates Run record in PostgreSQL
  3. Planner Agent plans validation campaign
     └─► Loads playbooks, filters by tier, returns ordered step list
     └─► Orchestrator stores step count in PostgreSQL
  4. Pre-run coverage snapshot (for drift detection)
     └─► Queries CoverageRepo for current technique coverage state
  5. Connector resolution
     └─► Maps engagement ConnectorIDs to category buckets (siem, edr, itsm, notification)

Phase 2 — Per-step execution (for each planned step)
  a. PolicyEnforcer → validates against scope/tier/allowlist (fail-closed: missing context = blocked)
  b. ApprovalGate → blocks for human decision (Tier 2+ only, approval expiry re-verified at execution)
  c. Executor → runs the playbook step, captures results
  d. EvidenceAgent → uploads execution artifacts to MinIO
  e. TelemetryVerifier → queries SIEM/EDR for expected telemetry
  f. DetectionEvaluator → checks alert generation + latency
  g. Step results accumulated (findings, evidence, telemetry/detection flags)

Phase 3 — Post-run agents
  a. ResponseAutomator → creates ITSM tickets + sends notifications for findings
  b. CoverageMapper → upserts ATT&CK coverage matrix with validation results
  c. DriftAgent → compares post-run vs pre-run coverage, generates drift findings
  d. RegressionAgent → compares current vs previous run findings
  e. ReceiptAgent → generates HMAC-SHA256 signed receipt with full step data
  f. Run marked complete in PostgreSQL
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
- `token_blacklist` — Revoked JWT tokens (SHA256 hashed, DB-persisted for multi-instance safety)
- `login_attempts` — Failed login tracking for account lockout (DB-persisted, survives restarts)

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
