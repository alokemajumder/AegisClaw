<p align="center">
  <h1 align="center">AegisClaw</h1>
  <p align="center">
    <strong>Autonomous Security Validation Platform</strong>
  </p>
  <p align="center">
    Continuously validate your security controls with safe, ATT&CK-mapped emulations.<br/>
    Prove outcomes with audit-grade receipts. Keep all data and AI reasoning local.
  </p>
  <p align="center">
    <a href="#features">Features</a> &bull;
    <a href="#quick-start">Quick Start</a> &bull;
    <a href="#architecture">Architecture</a> &bull;
    <a href="#documentation">Documentation</a> &bull;
    <a href="#contributing">Contributing</a>
  </p>
</p>

---

## What is AegisClaw?

AegisClaw is an open-source, enterprise-grade platform that **continuously validates whether your security controls actually work**. Instead of periodic manual red/blue exercises, AegisClaw runs safe, bounded adversary emulations autonomously and verifies whether your controls **observe, detect, alert, and ticket** as expected.

The platform is **agents-first**: autonomous agent squads (Red + Blue + Purple loop) orchestrate all actions end-to-end. Humans stay in the loop as a **critical approval gate** for high-risk actions and policy changes — not as operators of every test.

### How It Works

```
                    Schedule / Trigger
                          |
                          v
    ┌─────────────────────────────────────────────┐
    │           AegisClaw Orchestrator              │
    │      (Policy enforcement + Run lifecycle)    │
    └───────────────┬───────────────┬──────────────┘
                    |               |
         ┌──────────┘               └──────────┐
         v                                     v
  ┌──────────────┐                    ┌──────────────┐
  │  Emulation   │   Safe bounded    │  Validation   │
  │  Squad (Red) │   emulations -->  │  Squad (Blue) │
  │  Plan+Execute│   telemetry -->   │  Verify+Eval  │
  └──────┬───────┘                    └──────┬───────┘
         │                                    │
         └──────────┐               ┌─────────┘
                    v               v
              ┌──────────────────────────┐
              │   Improvement Squad      │
              │   (Purple Loop)          │
              │   Coverage + Drift +     │
              │   Regression Analysis    │
              └──────────────────────────┘
                         │
                         v
              ┌──────────────────────────┐
              │   Evidence Vault         │
              │   Findings + Receipts    │
              │   Reports + Tickets      │
              └──────────────────────────┘
```

## Features

### Continuous Autonomous Validation
- **Tier 0 (Passive)**: Telemetry pipeline health, config posture checks — fully autonomous
- **Tier 1 (Benign emulation)**: Safe atomic-style tests with ATT&CK mapping and cleanup verification — fully autonomous
- **Tier 2 (Sensitive)**: Actions impacting auth/ops require explicit human approval
- **Tier 3 (Prohibited)**: DoS, exfil, destructive actions — blocked by default

### Agent Squads (12 agents, fully wired pipeline)

All 12 agents are wired into a 3-phase `RunEngine` pipeline that executes end-to-end on every validation run:

- **Governance Squad**: PolicyEnforcer (tier/allowlist enforcement), ApprovalGate (human approval for Tier 2+), ReceiptAgent (HMAC-SHA256 signed audit receipts)
- **Emulation Squad (Red)**: Planner (playbook loading + step ordering), Executor (safe step execution), EvidenceAgent (artifact capture to MinIO)
- **Validation Squad (Blue)**: TelemetryVerifier (SIEM/EDR telemetry queries), DetectionEvaluator (alert verification + latency measurement), ResponseAutomator (ITSM tickets + notifications)
- **Improvement Squad (Purple)**: CoverageMapper (ATT&CK coverage matrix updates), DriftAgent (pre/post-run coverage comparison), RegressionAgent (cross-run finding comparison)

### Enterprise-Grade Controls
- Audit-grade immutable run receipts (HMAC-SHA256 signed with full step data + scope snapshot)
- Target allowlists + exclusions (hard enforced — empty allowlist blocks all Tier 1+ actions)
- Time windows, blackout periods, rate limits, concurrency caps
- Circuit breakers and global kill switch
- RBAC with 4 roles (admin, operator, approver, viewer)
- Pre-run coverage snapshots for drift detection

### Local LLM Reasoning (Ollama)
- Exposure graph analysis and validation planning
- Evidence-anchored findings (anti-hallucination enforcement)
- Finding deduplication and root-cause clustering
- Remediation drafting with stack-specific guidance
- All reasoning stays within your network — no external data egress

### Settings-Driven Connector System
- Fully configurable via UI/API — add and manage connectors without code changes
- JSON Schema-driven config forms, per-instance rate limits and field mappings
- Hot-pluggable with health monitoring and circuit breaking

### Supported Connectors

| Category | Connector | Status |
|----------|-----------|--------|
| **SIEM** | Microsoft Sentinel | Available |
| **EDR/XDR** | Microsoft Defender for Endpoint | Available |
| **ITSM** | ServiceNow | Available |
| **Notifications** | Microsoft Teams (Webhook) | Available |
| **Notifications** | Slack (Webhook) | Available |
| **SIEM** | Splunk | Planned |
| **SIEM** | Elastic Security | Planned |
| **EDR/XDR** | CrowdStrike Falcon | Planned |
| **ITSM** | Jira Service Management | Planned |
| **Identity** | Microsoft Entra ID | Planned |
| **Identity** | Okta | Planned |

The connector registry pre-seeds 10 connector types (sentinel, defender, entraid, servicenow, teams, slack, splunk, elastic, crowdstrike, jira, okta). Additional connectors can be built using the [Connector SDK](docs/connector-development.md).

### CISO-Ready Reporting
- **Executive report**: Posture overview, top gaps, drift trends, SLA adherence
- **Technical report**: Evidence-linked findings, remediation steps, retest outcomes
- **Coverage report**: Blind spots and detection gaps
- **Delta report**: New vs fixed vs unchanged since last run
- Export: Markdown, JSON (PDF renderer planned)

## Quick Start

### Prerequisites
- [Go 1.25+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/)
- [Docker 24+](https://www.docker.com/get-started/) and Docker Compose v2
- At least 8 GB RAM (for the full stack)

### Option 1: Docker Compose (Recommended)

```bash
# Clone the repository
git clone https://github.com/alokemajumder/AegisClaw.git
cd AegisClaw

# Configure environment
cp .env.example .env
# Edit .env — at minimum set AEGISCLAW_AUTH_JWT_SECRET to a strong random value

# Start everything
docker compose -f deploy/docker-compose.yml up -d

# Run database migrations
make migrate

# Seed initial admin user (admin@aegisclaw.local / admin)
make seed

# Access the platform
# UI:        http://localhost:3000
# API:       http://localhost:8080
# NATS:      http://localhost:8222 (monitoring)
# MinIO:     http://localhost:9001 (console)
# Grafana:   http://localhost:3001
# Jaeger:    http://localhost:16686
```

### Option 2: Development Setup

```bash
# Clone the repository
git clone https://github.com/alokemajumder/AegisClaw.git
cd AegisClaw

# Configure environment
cp .env.example .env

# Start infrastructure only
make infra-up

# Install dependencies
go mod tidy
cd web && npm install && cd ..

# Run database migrations
make migrate

# Run services (in separate terminals)
make dev-api          # API Gateway on :8080
make dev-orchestrator # Orchestrator on :9090
make dev-web          # Frontend on :3000

# Run tests
make test
```

See the [Deployment Guide](docs/deployment.md) for production deployment, Kubernetes, and hardening instructions.

## Architecture

### Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend Services | Go 1.25+ |
| Frontend | Next.js 15, React 19, Tailwind CSS, shadcn/ui |
| Database | PostgreSQL 16 |
| Message Broker | NATS + JetStream |
| Blob Storage | MinIO (S3-compatible) |
| LLM | Ollama (local inference) |
| Runner Sandbox | In-process (gVisor planned for Phase 2) |
| Observability | OpenTelemetry, Prometheus, Grafana, Jaeger |

### Services

| Service | Port | Health Port | Description |
|---------|------|-------------|-------------|
| `api-gateway` | 8080 | 8080 | REST API (59 endpoints), JWT auth, RBAC |
| `orchestrator` | 9090 | 10090 | Agent lifecycle, run execution, policy enforcement |
| `runner` | 9091 | 10091 | Sandboxed validation step execution |
| `evidence-service` | 9092 | 10092 | Evidence vault (MinIO), receipt storage |
| `connector-service` | 9093 | 10093 | Connector lifecycle, execution, health checks |
| `reporting-service` | 9094 | 10094 | Report generation (Markdown/JSON) |
| `ollama-bridge` | 9095 | 10095 | LLM proxy with prompt governance |
| `scheduler` | 9096 | 10096 | Cron scheduling, blackout enforcement |
| `web` | 3000 | — | Control Plane UI |

All services expose `/healthz` for Docker health checks and Kubernetes probes.

### Project Structure

```
AegisClaw/
├── cmd/                    # Service entrypoints
├── internal/               # Shared internal packages
│   ├── config/             # Configuration management
│   ├── database/           # PostgreSQL + migrations
│   ├── models/             # Domain models
│   ├── auth/               # JWT, RBAC, SSO
│   ├── nats/               # NATS JetStream client
│   ├── policy/             # Tier policy engine
│   ├── receipt/            # Immutable receipt generation
│   ├── evidence/           # MinIO evidence storage
│   ├── circuitbreaker/     # Circuit breaker + kill switch
│   └── observability/      # OTEL tracing + metrics
├── pkg/                    # Public SDKs
│   ├── connectorsdk/       # Connector development SDK
│   └── agentsdk/           # Agent development SDK
├── connectors/             # Built-in connector implementations
├── agents/                 # Agent squad implementations
├── playbooks/              # Validation playbook definitions
├── web/                    # Next.js frontend
├── deploy/                 # Docker, Compose, Helm, scripts
├── docs/                   # Documentation
└── configs/                # Default configuration files
```

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/architecture.md) | System design, data flows, service interactions |
| [Security Model](docs/security-model.md) | Governance tiers, safety controls, threat model |
| [Deployment Guide](docs/deployment.md) | Docker Compose, production hardening, configuration |
| [Connector Development](docs/connector-development.md) | How to build custom connectors |
| [Playbook Authoring](docs/playbook-authoring.md) | How to create validation playbooks |

## Deployment Models

AegisClaw is designed as a **dedicated single-tenant deployment** per organization:

- **Docker Compose** — Development, PoC, and small-scale production environments
- **Kubernetes (Helm)** — Production scale, isolation, and HA (planned)
- **VM Appliance** — Hardened control plane + runner nodes (planned)

All data stays inside the customer boundary. Ollama runs locally. No external LLM dependency required. Air-gap capable.

For detailed deployment instructions, see the [Deployment Guide](docs/deployment.md).

## Governance & Safety

AegisClaw enforces strict safety controls that **cannot be bypassed**:

- **Hard allowlists**: Only targets explicitly listed can be validated
- **Tier enforcement**: Tier 3 (DoS/exfil/destructive) is always blocked
- **Human approval gates**: Tier 2+ actions require explicit human approval
- **Mandatory cleanup**: All Tier 1+ steps must verify cleanup before completion
- **Kill switch**: Global and per-run emergency stop with cleanup confirmation
- **Immutable audit trail**: Every action, approval, and policy change is logged

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Workflow

```bash
# Fork and clone
git clone https://github.com/YOUR_USERNAME/AegisClaw.git
cd AegisClaw

# Create a branch
git checkout -b feature/your-feature

# Make changes, then test
make lint
make test

# Submit a pull request
```

### Code Standards

- Go: Follow standard Go conventions, `gofmt`, `golangci-lint`
- Frontend: ESLint + Prettier
- Tests: Required for all new functionality
- Commits: Imperative mood, concise, focused on "why"

## Roadmap

### Phase 0 — Foundations (Complete)
- [x] Core platform architecture
- [x] Database schema and migrations (14 tables across 2 migrations)
- [x] Service skeletons with gRPC/REST (8 Go services + 1 CLI)
- [x] Agent SDK and Connector SDK
- [x] Docker Compose development stack (16 services total)
- [x] Frontend shell with navigation

### Phase 1 — MVP (Complete)
- [x] End-to-end Tier 0-1 autonomous validation
- [x] Evidence vault and findings lifecycle (dedup, state machine)
- [x] 5 connectors: Sentinel, Defender, ServiceNow, Teams, Slack
- [x] Ollama Bridge with prompt governance and model allowlisting
- [x] Executive, technical, and coverage reports (Markdown + JSON)
- [x] 12 agents wired across 4 squads
- [x] 13 validation playbooks (4 Tier 0, 6 Tier 1, 3 Tier 2) + playbook schema
- [x] CLI tool with asset, engagement, run, finding, and report commands
- [x] Full frontend integration (15 pages, live API data)

### Phase 2 — Production Hardening (In Progress)
- [x] JWT validation, auth rate limiting, account lockout (5 attempts → 15 min lockout)
- [x] Token blacklisting with SHA256 hashing and periodic cleanup
- [x] Tenant isolation (org_id enforcement on all handlers)
- [x] Docker Compose hardening (resource limits, log rotation, health checks)
- [x] Health check endpoints on all services (/healthz + /readyz)
- [x] Kill switch persistence across restarts
- [x] Graceful shutdown with timeouts on all gRPC services
- [x] Prometheus metrics middleware with path normalization
- [x] API retry logic with exponential backoff and jitter
- [x] Pagination bug fixes (correct total counts from DB)
- [x] Dashboard optimized with DB aggregate queries
- [x] Frontend inline edit/delete for assets and engagements
- [x] Full 12-agent pipeline wired in RunEngine (3-phase: plan → per-step → post-run)
- [x] HMAC-SHA256 receipt signing via `internal/receipt.Generator`
- [x] PolicyEnforcer allowlist enforcement for Tier 1+ actions
- [x] Coverage mapper, drift detection, and regression testing agents fully wired
- [x] Pre-run coverage snapshots for drift comparison
- [x] Connector resolution by category (siem, edr, itsm, notification)
- [x] Simulated/fake fallback data removed from all agents
- [ ] gVisor runner sandboxing
- [ ] Additional connectors (Entra ID, Splunk, Elastic, CrowdStrike, Jira, Okta)
- [ ] PDF report renderer
- [ ] WebSocket/SSE real-time updates
- [ ] Kubernetes Helm chart
- [ ] Integration and end-to-end tests

### Phase 3 — Enterprise Expansion
- [ ] Full coverage matrix visualization (ATT&CK x Asset x Telemetry heatmap in UI)
- [ ] SSO/OIDC integration
- [ ] HA and backup/restore
- [ ] Vertical-specific validation playbooks
- [ ] Compliance-ready exports

## License

AegisClaw is open-source software licensed under the [Apache License 2.0](LICENSE).

## Security

If you discover a security vulnerability in AegisClaw, please report it responsibly. See [SECURITY.md](SECURITY.md) for our disclosure policy.

---

<p align="center">
  Built for security teams who believe in <strong>proving</strong> their defenses work, not just hoping they do.
</p>
