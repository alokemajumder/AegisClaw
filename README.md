<p align="center">
  <h1 align="center">AegisClaw</h1>
  <p align="center"><strong>Autonomous Security Validation Platform</strong></p>
  <p align="center">
    Continuously validate your security controls with safe, ATT&CK-mapped emulations.<br/>
    Prove outcomes with audit-grade receipts. Keep all data and LLM reasoning local.
  </p>
  <p align="center">
    <a href="#quick-start">Quick Start</a> &bull;
    <a href="#how-it-works">How It Works</a> &bull;
    <a href="docs/architecture.md">Architecture</a> &bull;
    <a href="docs/deployment.md">Deployment</a> &bull;
    <a href="docs/security-model.md">Security Model</a> &bull;
    <a href="CONTRIBUTING.md">Contributing</a>
  </p>
</p>

---

## Why AegisClaw?

Security teams run periodic red/blue exercises and hope their controls hold in between. AegisClaw replaces hope with proof — it runs safe, bounded adversary emulations **continuously and autonomously**, then verifies whether your controls actually **observe, detect, alert, and ticket** as expected.

Every validation run produces a cryptographically signed receipt. Every finding is backed by evidence. All data stays in your network.

### NVIDIA-Accelerated AI Security

AegisClaw integrates deeply with the [NVIDIA AI agent ecosystem](https://nvidianews.nvidia.com/news/ai-agents) to deliver enterprise-grade LLM reasoning and sandboxed agent execution:

- **[NVIDIA NIM](https://build.nvidia.com)** — Optimized inference microservices for [Nemotron](https://developer.nvidia.com/nemotron) models with configurable thinking budgets and OpenAI-compatible tool-calling
- **[NVIDIA OpenShell](https://docs.nvidia.com/openshell/latest/index.html)** — Apache 2.0 runtime for sandboxed agent execution with Landlock LSM, seccomp, network namespacing, and a Privacy Router for secure LLM inference
- **[NeMoClaw](https://build.nvidia.com/nemoclaw)** — Always-on assistant stack powering AegisClaw's multi-agent orchestration with 34+ hosted models
- **[NVIDIA Morpheus](https://developer.nvidia.com/morpheus-cybersecurity)** — GPU-accelerated security analytics via Triton Inference Server for real-time threat detection

## Quick Start

```bash
git clone https://github.com/alokemajumder/AegisClaw.git
cd AegisClaw

cp .env.example .env
# Set AEGISCLAW_AUTH_JWT_SECRET to a strong random value (openssl rand -hex 32)

docker compose -f deploy/docker-compose.yml up -d
make migrate && make seed

# UI: http://localhost:3000  (admin@aegisclaw.local / admin)
# API: http://localhost:8080
```

See the [Deployment Guide](docs/deployment.md) for production setup, hardening, and Kubernetes.

## How It Works

```
  Trigger (schedule / manual / API)
              |
              v
  ┌───────────────────────────┐
  │     Orchestrator          │
  │  Policy + Run Lifecycle   │
  └─────┬───────────┬────────┘
        |           |
  ┌─────v─────┐ ┌───v──────┐
  │ Emulation │ │Validation│    12 agents across 4 squads
  │  (Red)    │ │ (Blue)   │    execute a 3-phase pipeline
  │ Plan+Exec │ │Verify+Eval│   on every run
  └─────┬─────┘ └───┬──────┘
        |           |
        v           v
  ┌───────────────────────────┐
  │   Improvement (Purple)    │
  │ Coverage + Drift + Regr.  │
  └─────────────┬─────────────┘
                v
  ┌───────────────────────────┐
  │    Evidence Vault         │
  │ Findings  Receipts  Rpts  │
  └───────────────────────────┘
```

**Governance Tiers** control what runs autonomously:

| Tier | Scope | Approval |
|------|-------|----------|
| 0 - Passive | Telemetry health, config posture | Autonomous |
| 1 - Benign | Safe atomic tests (EICAR, allowlisted commands, SIEM/EDR queries) | Autonomous |
| 2 - Sensitive | Auth-adjacent, operational impact | Human approval required |
| 3 - Prohibited | DoS, exfil, destructive | Always blocked |

## Key Capabilities

**Continuous Validation** — Schedule ATT&CK-mapped emulations with cron expressions, blackout windows, rate limits, and concurrency caps. The orchestrator runs 12 agents through plan, execute, and verify phases automatically.

**Audit-Grade Evidence** — Every run produces HMAC-SHA256 signed receipts capturing scope, steps, evidence, and outcomes. Findings are deduplicated via SHA256 clustering. All artifacts stored in an immutable MinIO vault.

**12 Connectors** — Query telemetry from your SIEM, verify detections in your EDR, create tickets in your ITSM, run GPU-accelerated analytics, and send notifications — all through a settings-driven UI with no code changes required.

| Category | Connectors |
|----------|------------|
| SIEM | Sentinel, Splunk, Elastic Security |
| EDR/XDR | Defender for Endpoint, CrowdStrike Falcon |
| ITSM | ServiceNow, Jira Service Management |
| Analytics | [NVIDIA Morpheus](https://developer.nvidia.com/morpheus-cybersecurity) (GPU-accelerated threat detection via Triton) |
| Notifications | Teams, Slack |
| Identity | Entra ID, Okta |

**Flexible LLM Backend** — Runs on any hardware from a $200 used GPU to [NVIDIA DGX Spark](https://www.nvidia.com/en-us/products/workstations/dgx-spark/):
- **Ollama** (default) — Free, open-source, runs on consumer GPUs (RTX 3060+) or CPU-only
- **[NVIDIA NIM](https://build.nvidia.com)** — [Nemotron 3](https://developer.nvidia.com/nemotron) models (Nano 30B, Super 120B, Ultra 253B) with hybrid Mamba-Transformer MoE architecture, 1M context windows, configurable thinking budgets, and OpenAI-compatible [tool-calling](https://docs.nvidia.com/nim/large-language-models/latest/function-calling.html) for agent function invocation
- **[NeMo Guardrails](https://developer.nvidia.com/nemo-guardrails)** — Optional content safety, jailbreak detection, and topic control NIMs for LLM prompt safety
- **[NVIDIA OpenShell](https://docs.nvidia.com/openshell/latest/index.html)** — Sandboxed agent execution with 4-layer isolation (Landlock LSM, seccomp, network namespacing, Privacy Router) and tier-based policy generation

All reasoning stays within your infrastructure. See the [NVIDIA Deployment Guide](docs/nvidia-deployment.md) for GPU sizing and cost optimization.

**Enterprise Safety Controls** — Fail-closed policy enforcement, hard target allowlists, circuit breakers on all connector calls, global kill switch (NATS-propagated, persistent across restarts), RBAC on all 59 API endpoints, persistent token blacklisting, and account lockout.

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.25+ (8 microservices + CLI) |
| Frontend | Next.js 15, React 19, Tailwind, shadcn/ui |
| Database | PostgreSQL 16 (16 tables, golang-migrate) |
| Messaging | NATS + JetStream (5 streams) |
| Evidence | MinIO (S3-compatible) |
| LLM | Ollama (local, free) or [NVIDIA NIM](https://build.nvidia.com) + [Nemotron 3](https://developer.nvidia.com/nemotron) (high-perf, 1M context, tool-calling) |
| LLM Safety | [NeMo Guardrails](https://developer.nvidia.com/nemo-guardrails) (content safety, jailbreak detection) |
| Agent Sandbox | [NVIDIA OpenShell](https://docs.nvidia.com/openshell/latest/index.html) (Landlock + seccomp + netns isolation) |
| Analytics | [NVIDIA Morpheus](https://developer.nvidia.com/morpheus-cybersecurity) (GPU-accelerated security analytics) |
| Observability | OpenTelemetry, Prometheus, Grafana, Jaeger |

## Documentation

| Document | Description |
|----------|-------------|
| **[Architecture](docs/architecture.md)** | Services, agent squads, data flows, NATS streams, database schema |
| **[Security Model](docs/security-model.md)** | Governance tiers, safety controls, auth, RBAC, threat model |
| **[Deployment Guide](docs/deployment.md)** | Docker Compose, production hardening, backup/restore, CLI, troubleshooting |
| **[NVIDIA GPU Deployment](docs/nvidia-deployment.md)** | GPU sizing, [NIM](https://build.nvidia.com) setup, [Nemotron 3](https://developer.nvidia.com/nemotron) models, [OpenShell](https://docs.nvidia.com/openshell/latest/index.html) sandboxing, cost optimization |
| **[Connector Development](docs/connector-development.md)** | Build custom connectors using the Connector SDK |
| **[Playbook Authoring](docs/playbook-authoring.md)** | Create validation playbooks (YAML format) |

## Project Structure

```
AegisClaw/
├── cmd/           Service entrypoints (9 binaries)
├── internal/      Shared packages (config, auth, database, nats, policy, receipt, evidence, ...)
├── pkg/           Public SDKs (connectorsdk, agentsdk)
├── agents/        12 agents across 4 squads
├── connectors/    12 connector implementations
├── playbooks/     13 validation playbooks (Tier 0-2)
├── web/           Next.js frontend (15 pages)
├── deploy/        Docker Compose, Dockerfiles, scripts
├── docs/          Documentation
└── configs/       Default YAML configuration
```

## Roadmap

### Complete
- Core platform with 8 Go microservices + CLI + Next.js frontend
- Full end-to-end validation pipeline (12 agents, 3-phase RunEngine)
- 12 connectors (SIEM, EDR, ITSM, Analytics, Notifications, Identity) including [NVIDIA Morpheus](https://developer.nvidia.com/morpheus-cybersecurity)
- 13 playbooks with real execution (SIEM queries, EDR health, EICAR markers, detection verification)
- Evidence vault, finding dedup, HMAC-SHA256 receipt signing
- JWT auth, RBAC, token blacklisting, account lockout, kill switch
- Docker Compose with health checks, resource limits, graceful shutdown
- Production audit: 22 security fixes, connection pool hardening, input validation
- [NVIDIA NIM](https://build.nvidia.com) integration with [Nemotron 3](https://developer.nvidia.com/nemotron) model family (Nano 30B, Super 120B, Ultra 253B — hybrid Mamba-Transformer MoE, 1M context)
- [NeMo Guardrails](https://developer.nvidia.com/nemo-guardrails) integration (content safety, jailbreak detection, topic control)
- GPU deployment profiles for consumer GPUs (RTX 3060-5090) through [DGX Spark](https://www.nvidia.com/en-us/products/workstations/dgx-spark/)
- [NVIDIA OpenShell](https://docs.nvidia.com/openshell/latest/index.html) agent sandboxing — Gateway v1 API client, tier-based policy generation, [Privacy Router](https://docs.nvidia.com/openshell/latest/index.html) for secure LLM inference
- SSE (Server-Sent Events) real-time updates — NATS-to-browser bridge with global and per-run event streams
- [NeMoClaw](https://build.nvidia.com/nemoclaw) multi-agent orchestration with configurable thinking budgets and tool-calling

### In Progress
- PDF report renderer
- Kubernetes Helm chart
- Integration and end-to-end tests

### Planned
- Full ATT&CK coverage heatmap visualization
- SSO/OIDC integration
- HA and backup/restore automation
- Vertical-specific playbook packs
- Compliance-ready exports (SOC 2, ISO 27001)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions and coding standards.

```bash
git checkout -b feature/your-feature
make lint && make test
# Submit a pull request
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).

## Security

Report vulnerabilities responsibly. See [SECURITY.md](SECURITY.md) for our disclosure policy.

---

<p align="center">
  Built for security teams who believe in <strong>proving</strong> their defenses work.
</p>
