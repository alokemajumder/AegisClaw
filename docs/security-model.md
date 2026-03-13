# AegisClaw Security Model

## Overview

AegisClaw enforces a layered security model with multiple non-bypassable controls. The core principle: **autonomous where safe, human-gated where sensitive, blocked where dangerous**.

## Governance Tiers

### Tier 0 — Passive Validation (Fully Autonomous)
- Telemetry pipeline health checks
- Configuration posture verification
- Detection rule deployment verification
- SIEM/EDR connectivity monitoring
- **No impact** on target systems

### Tier 1 — Benign Emulation (Fully Autonomous + Cleanup)
- Safe atomic-style test execution (e.g., benign file creation, scheduled task trigger)
- ATT&CK technique marker placement
- Safe network behavior simulation
- **Mandatory cleanup verification** — step is not marked complete until cleanup is confirmed
- Rollback on failure

### Tier 2 — Sensitive Validation (Requires Human Approval)
- Actions that may impact authentication flows
- Tests affecting operational stability
- Credential-adjacent validation
- **Explicit approval required** from configured approver (CISO or delegated security owner)
- Approval has configurable expiry (default: 24 hours)

### Tier 3 — Prohibited by Default (Always Blocked)
- Denial of service / stress testing
- Data exfiltration behavior
- Destructive actions
- Uncontrolled payload execution
- **Cannot be unblocked** through normal policy changes

## Safety Controls

### Target Allowlists and Exclusions
- Engagements define explicit target allowlists (asset UUIDs)
- Exclusions take precedence over allowlists
- **Hard enforcement**: the Policy Enforcer agent rejects any step targeting an asset not in the allowlist or in the exclusion list
- No override mechanism — changing the allowlist requires updating the engagement configuration

### Rate Limiting
- **Global rate limit**: Maximum actions per minute across all engagements
- **Per-engagement rate limit**: Configurable per engagement
- **Per-connector rate limit**: Protects external systems from overwhelming requests
- Enforced at both the policy engine and API gateway levels

### Concurrency Caps
- Global concurrency cap limits simultaneous active runs
- Per-engagement concurrency cap limits parallel steps
- Backpressure via NATS consumer flow control

### Time Controls
- **Run windows**: Engagements can specify allowed execution time ranges
- **Blackout windows**: Define periods when no validation runs (e.g., maintenance windows, business-critical hours)
- Enforced by both the Scheduler and Policy Enforcer

### Circuit Breakers
- Per-connector circuit breakers (error rate / count threshold)
- States: Closed (normal) → Open (rejecting) → Half-Open (testing recovery)
- Configurable thresholds and reset timeouts
- Unhealthy connectors are isolated without affecting other connectors

### Kill Switch
- **Global kill switch**: Immediately stops all active runs across all engagements
- **Per-run kill switch**: Stops a specific run
- Kill switch confirms cleanup status for all stopped steps
- Kill switch engagement is logged in the immutable audit log
- **Persistent across restarts**: State is loaded from the audit log on API gateway startup
- Kill switch propagates via NATS to the orchestrator, which cancels all in-flight run contexts
- Orchestrator checks `killSwitch.IsEngaged()` before executing each step
- Accessible via API (`POST /api/v1/admin/system/kill-switch`), UI (red button in top bar), and CLI (`aegiscli kill-switch engage/disengage`)

## Authentication and Authorization

### Authentication
- **JWT tokens** with short expiry (default: 15 minutes) + refresh tokens (default: 7 days)
- **JWT secret validation**: API gateway refuses to start with an empty JWT secret and warns on default values
- **SSO/OIDC** integration for enterprise identity providers (Entra ID, Okta, etc.) — planned
- Local login available for development; SSO recommended for production

### Account Lockout
- **5 failed login attempts** trigger a **15-minute lockout** per email address
- Lockout is tracked in-memory at the API gateway
- Lockout counter resets on successful authentication
- Protects against brute-force credential attacks

### Auth Rate Limiting
- Login and refresh endpoints are rate-limited to **10 requests per minute per IP** (stricter than the global 100/min API rate limit)
- Enforced via `httprate` middleware at the API gateway

### Frontend Auth Middleware
- Next.js middleware intercepts all requests and redirects unauthenticated users to `/login`
- Auth token is stored as both a cookie (for SSR middleware checks) and localStorage (for API calls)
- Automatic redirect on 401 responses from the API

### RBAC Roles
| Role | Permissions |
|------|------------|
| `admin` | Full access — user management, policy changes, kill switch, all CRUD operations |
| `operator` | Manage engagements, runs, assets, findings. Cannot modify users or policies |
| `approver` | Approve/deny Tier 2+ requests. Read access to all entities |
| `viewer` | Read-only access to all entities |

### Authorization Enforcement
- RBAC checked at API gateway middleware level (every request)
- Policy changes require `admin` role
- Tier 2+ approvals require `admin` or `approver` role

### Tenant Isolation
- All 30+ API handlers enforce **org_id ownership checks** on every data access
- Users can only access resources belonging to their organization
- Cross-tenant access returns **404 Not Found** (not 403) to prevent information leakage
- Enforced at the repository query level (WHERE org_id = $claims.OrgID)

## Audit Trail

### Immutable Audit Log
- Every action (user, agent, system) is recorded in the `audit_log` table
- The table is **append-only** — no UPDATE or DELETE operations are permitted
- Each entry records: actor (type + ID), action, resource, details, IP address, timestamp

### Run Receipts
- Every completed run generates an immutable **run receipt**
- Receipts include: scope snapshot, all steps with timestamps, evidence manifest, tool versions
- Receipts are **HMAC-SHA256 signed** to ensure tamper evidence
- Stored in MinIO with versioning enabled (immutable bucket policy recommended)

## Data Security

### Encryption
- **In transit**: TLS for all HTTP/gRPC communication; mTLS recommended in production
- **At rest**: MinIO supports server-side encryption; PostgreSQL supports TDE
- **Secrets**: Stored via vault references, never in database config fields

### Data Locality
- All data stays within the customer network/VPC
- Ollama runs locally — no external LLM API calls
- External enrichment sources are disabled by default (configurable allowlist)

### Redaction
- No customer PII/secrets stored in evidence vault by default
- Configurable redaction masks for sensitive data patterns
- Configurable retention policies per data type

## Connector Security

### Credential Handling
- Connector credentials stored via `secret_ref` pointing to external vault
- Supported auth methods: API keys, OAuth2, service principals, certificates
- Credentials validated at connection test time, never logged

### Network Isolation
- Runner containers use gVisor with deny-by-default egress
- Egress controlled via allowlisted destinations only
- Per-connector network scoping possible via runner segmentation

## Threat Model

### What AegisClaw Defends Against
- Accidental scope creep (hard allowlist enforcement)
- Runaway automation (rate limits, concurrency caps, circuit breakers)
- Unauthorized escalation (tier enforcement, approval gates)
- Evidence tampering (HMAC-signed receipts, immutable audit log)
- Credential leakage (vault references, never stored in config)

### What is Explicitly Out of Scope
- Free-form exploitation or exploit development
- DoS/stress testing against production
- Data exfiltration or handling sensitive payloads beyond policy
- Targeting assets outside the allowlist
