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
    │             OpenClaw Orchestrator            │
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

### Agent Squads
- **Governance Squad**: Policy enforcement, approval gates, immutable run receipts
- **Emulation Squad (Red)**: Validation planning, safe execution, evidence capture
- **Validation Squad (Blue)**: Telemetry verification, detection evaluation, automated ticketing
- **Improvement Squad (Purple)**: Coverage matrix, regression testing, drift detection

### Enterprise-Grade Controls
- Audit-grade immutable run receipts (HMAC-signed)
- Target allowlists + exclusions (hard enforced)
- Time windows, blackout periods, rate limits, concurrency caps
- Circuit breakers and global kill switch
- RBAC with SSO (OIDC) integration

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
| **SIEM** | Splunk, Elastic Security, IBM QRadar | Planned |
| **EDR/XDR** | CrowdStrike Falcon, SentinelOne | Planned |
| **ITSM** | Jira Service Management | Planned |
| **Identity** | Microsoft Entra ID, Okta | Planned |

Additional connectors can be built using the [Connector SDK](docs/connector-development.md).

### CISO-Ready Reporting
- **Executive report**: Posture overview, top gaps, drift trends, SLA adherence
- **Technical report**: Evidence-linked findings, remediation steps, retest outcomes
- **Coverage report**: Blind spots and detection gaps
- **Delta report**: New vs fixed vs unchanged since last run
- Export: PDF, Markdown, JSON

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

# Seed initial admin user (admin@aegisclaw.local / changeme)
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
| Runner Sandbox | gVisor |
| Observability | OpenTelemetry, Prometheus, Grafana, Jaeger |

### Services

| Service | Port | Health Port | Description |
|---------|------|-------------|-------------|
| `api-gateway` | 8080 | 8080 | REST API, auth, RBAC, SSO |
| `orchestrator` | 9090 | 10090 | Agent lifecycle, run execution, policy enforcement |
| `runner` | 9091 | 10091 | Sandboxed validation step execution |
| `evidence-service` | 9092 | 10092 | Evidence vault (MinIO), receipt storage |
| `connector-service` | 9093 | 10093 | Connector lifecycle, execution, health checks |
| `reporting-service` | 9094 | 10094 | Report generation (PDF/MD/JSON) |
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
| [API Reference](docs/api/openapi.yaml) | OpenAPI 3.1 specification |

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
- [x] Database schema and migrations (16 tables)
- [x] Service skeletons with gRPC/REST (8 services)
- [x] Agent SDK and Connector SDK
- [x] Docker Compose development stack
- [x] Frontend shell with navigation

### Phase 1 — MVP (Complete)
- [x] End-to-end Tier 0-1 autonomous validation
- [x] Evidence vault and findings lifecycle (dedup, state machine)
- [x] 5 connectors: Sentinel, Defender, ServiceNow, Teams, Slack
- [x] Ollama Bridge with prompt governance and model allowlisting
- [x] Executive, technical, and coverage reports (Markdown + JSON)
- [x] 12 agents wired across 4 squads
- [x] 13 validation playbooks (Tier 0-2)
- [x] CLI tool with 20 commands
- [x] Full frontend integration (15 pages, live data)

### Phase 2 — Production Hardening (In Progress)
- [x] JWT validation, auth rate limiting, account lockout
- [x] Tenant isolation (org_id enforcement on all handlers)
- [x] Docker Compose hardening (resource limits, env vars, health checks)
- [x] Health check endpoints on all services
- [x] Kill switch persistence across restarts
- [ ] gVisor runner sandboxing
- [ ] Additional connectors (Entra ID, Splunk, Elastic, CrowdStrike, Jira)
- [ ] WebSocket/SSE real-time updates
- [ ] Kubernetes Helm chart
- [ ] Integration and end-to-end tests

### Phase 3 — Enterprise Expansion
- [ ] Full coverage matrix (ATT&CK x Asset x Telemetry heatmap)
- [ ] Regression and drift detection
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
