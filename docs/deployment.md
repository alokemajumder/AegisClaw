# AegisClaw Deployment Guide

This guide covers everything needed to deploy AegisClaw, from a quick local setup to a production-hardened installation.

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Quick Start](#2-quick-start)
3. [Configuration](#3-configuration)
4. [Docker Compose Deployment](#4-docker-compose-deployment)
5. [Database Setup](#5-database-setup)
6. [Production Hardening Checklist](#6-production-hardening-checklist)
7. [Health Checks](#7-health-checks)
8. [CLI Tool](#8-cli-tool)
9. [Backup & Restore](#9-backup--restore)
10. [Troubleshooting](#10-troubleshooting)
11. [Service Ports Reference](#11-service-ports-reference)

---

## 1. Prerequisites

### Required Software

| Software            | Minimum Version | Notes                                      |
| ------------------- | --------------- | ------------------------------------------ |
| Docker              | 24.0+           | Docker Engine or Docker Desktop             |
| Docker Compose      | v2.20+          | Included with Docker Desktop; standalone OK |
| Git                 | 2.30+           | To clone the repository                     |
| `psql` (optional)   | 14+             | Only needed to run the seed script manually |

### System Requirements

| Resource | Minimum    | Recommended    |
| -------- | ---------- | -------------- |
| RAM      | 8 GB       | 16 GB          |
| CPU      | 4 cores    | 8 cores        |
| Disk     | 20 GB free | 50 GB free     |

> Ollama model storage can consume 4-10 GB per model. Plan disk accordingly if you pull multiple models.

### Required Ports

The following ports must be available on the host machine. If any are already in use, adjust mappings in `deploy/docker-compose.yml`.

| Port  | Service            |
| ----- | ------------------ |
| 3000  | Web UI (Next.js)   |
| 3001  | Grafana            |
| 4222  | NATS client        |
| 4317  | Jaeger OTLP        |
| 5432  | PostgreSQL         |
| 8080  | API Gateway        |
| 8222  | NATS monitoring    |
| 9000  | MinIO API          |
| 9001  | MinIO Console      |
| 9090  | Orchestrator gRPC  |
| 9091  | Runner gRPC        |
| 9092  | Evidence gRPC      |
| 9093  | Connector gRPC     |
| 9094  | Reporting gRPC     |
| 9095  | Ollama Bridge gRPC |
| 9096  | Scheduler gRPC     |
| 9190  | Prometheus         |
| 11434 | Ollama             |
| 16686 | Jaeger UI          |

---

## 2. Quick Start

Four steps to get AegisClaw running locally.

### Step 1 -- Clone the repository

```bash
git clone https://github.com/alokemajumder/AegisClaw.git
cd AegisClaw
```

### Step 2 -- Create your environment file

```bash
cp .env.example .env
```

Edit `.env` and set at minimum:

```bash
AEGISCLAW_AUTH_JWT_SECRET=<generate-a-strong-secret>
```

Generate a secret with:

```bash
openssl rand -hex 32
```

### Step 3 -- Start all services

```bash
docker compose -f deploy/docker-compose.yml up -d
```

Or use the Makefile shortcut:

```bash
make docker-up
```

Wait for all containers to become healthy (about 30-60 seconds):

```bash
docker compose -f deploy/docker-compose.yml ps
```

### Step 4 -- Seed the database

```bash
make seed
```

Or run the seed script directly:

```bash
bash deploy/scripts/seed.sh
```

The seed script creates:
- A **Default Organization**
- An **admin user** (`admin@aegisclaw.local` / `admin`)
- A **Default policy pack** with safe validation tier rules
- **Sample assets** (domain controller, workstation, web app, identity, cloud account)

You can now open the dashboard at [http://localhost:3000](http://localhost:3000) and log in with `admin@aegisclaw.local` / `admin`.

---

## 3. Configuration

AegisClaw is configured through environment variables and an optional YAML config file.

### Environment Variables

All environment variables are documented in [`.env.example`](../.env.example). The full reference is there -- copy it to `.env` and customize.

**Naming convention:** Environment variables use the `AEGISCLAW_` prefix. The prefix is stripped by the config loader (Viper), and underscores map to nested keys:

```
AEGISCLAW_DATABASE_HOST  -->  database.host
AEGISCLAW_AUTH_JWT_SECRET  -->  auth.jwt_secret
AEGISCLAW_NATS_URL  -->  nats.url
```

### YAML Config File

An optional YAML config file lives at `configs/aegisclaw.yaml`. Environment variables always take precedence over YAML values.

```yaml
server:
  api_port: 8080
  grpc_base_port: 9090
  playbook_dir: playbooks    # Path to playbook YAML directory (env: AEGISCLAW_SERVER_PLAYBOOK_DIR)

database:
  host: localhost
  port: 5432
  user: aegisclaw
  password: aegisclaw
  name: aegisclaw
  ssl_mode: disable
  max_conns: 25

nats:
  url: nats://localhost:4222
  max_reconnects: 60
  reconnect_wait: 2s

minio:
  endpoint: localhost:9000
  access_key: minioadmin
  secret_key: minioadmin
  bucket: aegisclaw-evidence
  use_ssl: false

ollama:
  url: http://localhost:11434
  default_model: llama3.1
  model_allowlist:
    - llama3.1
    - codellama
    - mistral
  timeout_seconds: 120

auth:
  jwt_secret: dev-secret-change-in-production
  token_expiry: 15m
  refresh_expiry: 168h
  receipt_hmac_key: dev-receipt-key-change-in-production

policy:
  default_pack: default
  global_rate_limit: 100
  global_concurrency_cap: 20

observability:
  tracing_endpoint: localhost:4317
  metrics_port: 9100
  log_level: info
```

### Key Configuration Groups

| Group           | Description                                                       |
| --------------- | ----------------------------------------------------------------- |
| `auth`          | JWT secret, token lifetimes, receipt HMAC signing key              |
| `database`      | PostgreSQL connection, SSL mode, pool size                        |
| `nats`          | NATS URL, reconnect behavior                                     |
| `minio`         | MinIO/S3 endpoint, credentials, bucket name                      |
| `ollama`        | Ollama URL, default model, model allowlist, timeout               |
| `server`        | API port, gRPC port, CORS origins, playbook directory             |
| `policy`        | Default playbook pack, rate limits, concurrency cap               |
| `observability` | OTEL tracing endpoint, Prometheus metrics port, log level         |

---

## 4. Docker Compose Deployment

The file `deploy/docker-compose.yml` defines all services, infrastructure, and networking.

### Services Overview

#### Infrastructure Services

| Service      | Image                            | Host Port(s) | Description                          | Resource Limits  |
| ------------ | -------------------------------- | ------------ | ------------------------------------ | ---------------- |
| `postgres`   | `postgres:16`                    | 5432         | Primary database                     | 2 CPU, 1 GB RAM  |
| `nats`       | `nats:2.10`                      | 4222, 8222   | Message broker with JetStream        | --               |
| `minio`      | `minio/minio`                    | 9000, 9001   | S3-compatible evidence vault         | 1 CPU, 512 MB    |
| `ollama`     | `ollama/ollama`                  | 11434        | Local LLM inference                  | --               |
| `jaeger`     | `jaegertracing/all-in-one:1.53`  | 16686, 4317  | Distributed tracing                  | --               |
| `grafana`    | `grafana/grafana`                | 3001         | Metrics dashboards                   | --               |
| `prometheus` | `prom/prometheus`                | 9190         | Metrics collection                   | --               |

#### Application Services

| Service              | gRPC Port | Health Port | Description                             | Resource Limits   |
| -------------------- | --------- | ----------- | --------------------------------------- | ----------------- |
| `api-gateway`        | --        | 8080        | REST API gateway (Chi router)           | 1 CPU, 512 MB     |
| `orchestrator`       | 9090      | 10090       | Engagement & agent squad orchestration  | 1 CPU, 512 MB     |
| `runner`             | 9091      | 10091       | Validation step execution               | 1 CPU, 512 MB     |
| `evidence-service`   | 9092      | 10092       | Evidence collection & MinIO storage     | 0.5 CPU, 256 MB   |
| `connector-service`  | 9093      | 10093       | External tool integrations              | 0.5 CPU, 256 MB   |
| `reporting-service`  | 9094      | 10094       | Report generation                       | 0.5 CPU, 256 MB   |
| `ollama-bridge`      | 9095      | 10095       | LLM reasoning proxy to Ollama           | 0.5 CPU, 256 MB   |
| `scheduler`          | 9096      | 10096       | Cron-based engagement scheduling        | 0.5 CPU, 256 MB   |
| `web`                | --        | 3000        | Next.js frontend                        | 0.5 CPU, 256 MB   |

### Persistent Volumes

| Volume           | Mounted In  | Purpose                    |
| ---------------- | ----------- | -------------------------- |
| `postgres-data`  | `postgres`  | Database files             |
| `minio-data`     | `minio`     | Evidence artifacts         |
| `ollama-models`  | `ollama`    | Downloaded LLM model files |

### Customizing Resource Limits

To adjust CPU and memory limits, edit the `deploy` section for the relevant service in `deploy/docker-compose.yml`:

```yaml
deploy:
  resources:
    limits:
      cpus: "2.0"
      memory: 1G
```

### Running a Subset of Services

Start only infrastructure (useful for local development against bare-metal Go binaries):

```bash
make infra-up
```

This starts: `postgres`, `nats`, `minio`, `ollama`, `jaeger`, `grafana`, `prometheus`.

Stop everything:

```bash
make docker-down
```

### Viewing Logs

Tail all service logs:

```bash
make docker-logs
```

Tail a specific service:

```bash
docker compose -f deploy/docker-compose.yml logs -f api-gateway
```

---

## 5. Database Setup

### Migrations

Database migrations live in `internal/database/migrations/` using the `golang-migrate` format. There are currently **3 migrations**:

1. `000001_initial_schema` — 13 core tables (organizations, users, assets, connectors, engagements, runs, run_steps, findings, approvals, audit_log, coverage_entries, policy_packs)
2. `000002_add_reports` — reports table
3. `000003_add_token_blacklist_and_login_attempts` — token_blacklist and login_attempts tables (persistent auth state)

**Automatic:** Migrations run automatically when the `api-gateway` service starts.

**Manual:** If you prefer to run migrations explicitly:

```bash
make migrate
```

To roll back the last migration:

```bash
make migrate-down
```

### Seeding

The seed script (`deploy/scripts/seed.sh`) populates the database with essential initial data. It requires `psql` on the host machine (or run it from inside the postgres container).

```bash
make seed
```

The seed script creates the following records (all using `ON CONFLICT DO NOTHING`, so it is safe to run multiple times):

| Record              | Details                                                        |
| ------------------- | -------------------------------------------------------------- |
| Default Organization| ID: `00000000-0000-0000-0000-000000000001`                    |
| Admin User          | Email: `admin@aegisclaw.local`, Password: `admin`, Role: admin |
| Default Policy Pack | Safe defaults: Tier 0-1 autonomous, Tier 2 needs approval, Tier 3 blocked |
| Sample Assets       | 5 assets: domain controller, workstation, web app, identity, cloud account |

### Default Admin Credentials

| Field    | Value                     |
| -------- | ------------------------- |
| Email    | `admin@aegisclaw.local`   |
| Password | `admin`                   |

**Change this password immediately in production.** The seeded password is a bcrypt hash of the string `admin`.

### Running Seed Inside Docker

If you do not have `psql` installed on the host, run the seed script from inside the Postgres container:

```bash
docker cp deploy/scripts/seed.sh aegisclaw-postgres:/tmp/seed.sh
docker exec aegisclaw-postgres bash /tmp/seed.sh
```

---

## 6. Production Hardening Checklist

Before exposing AegisClaw to a real environment, complete every item below.

### 6.1 Set Strong Auth Secrets

The JWT secret signs all authentication tokens. The receipt HMAC key signs immutable run receipts. Never use the defaults.

```bash
# Generate cryptographically random 256-bit secrets
openssl rand -hex 32  # For JWT secret
openssl rand -hex 32  # For receipt HMAC key
```

Set them in `.env`:

```bash
AEGISCLAW_AUTH_JWT_SECRET=<paste-jwt-secret-here>
AEGISCLAW_AUTH_RECEIPT_HMAC_KEY=<paste-receipt-key-here>
```

### 6.2 Change All Default Passwords

Replace every default password in `.env`:

```bash
# PostgreSQL
POSTGRES_USER=aegisclaw
POSTGRES_PASSWORD=<strong-password>

# MinIO
MINIO_ROOT_USER=<minio-admin-user>
MINIO_ROOT_PASSWORD=<strong-password>

# Grafana
GF_SECURITY_ADMIN_PASSWORD=<strong-password>
```

### 6.3 Restrict CORS Origins

Set `AEGISCLAW_SERVER_CORS_ORIGINS` to your actual frontend domain(s). Do not leave it as `http://localhost:3000`.

```bash
AEGISCLAW_SERVER_CORS_ORIGINS=https://aegisclaw.yourcompany.com
```

Multiple origins can be comma-separated:

```bash
AEGISCLAW_SERVER_CORS_ORIGINS=https://aegisclaw.yourcompany.com,https://admin.yourcompany.com
```

### 6.4 Enable TLS / Reverse Proxy

AegisClaw services do not terminate TLS themselves. Place a reverse proxy (nginx, Caddy, Traefik, or a cloud load balancer) in front of the API gateway and web frontend.

Example with nginx:

```nginx
server {
    listen 443 ssl;
    server_name aegisclaw.yourcompany.com;

    ssl_certificate     /etc/ssl/certs/aegisclaw.crt;
    ssl_certificate_key /etc/ssl/private/aegisclaw.key;

    location /api/ {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location / {
        proxy_pass http://localhost:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### 6.5 Enable Database SSL

Set the PostgreSQL SSL mode to `require` or `verify-full`:

```bash
AEGISCLAW_DATABASE_SSL_MODE=require
```

If using `verify-full`, you must also provide the CA certificate to each service.

### 6.6 Review Resource Limits

Examine and adjust `deploy.resources.limits` in `deploy/docker-compose.yml` based on your workload. The defaults are conservative and suitable for evaluation but may need scaling for production:

- `api-gateway`: Increase if handling high API traffic
- `orchestrator` / `runner`: Increase if running many concurrent validations
- `ollama`: Consider adding GPU resources and increased memory for LLM inference

### 6.7 Summary Checklist

```
[ ] AEGISCLAW_AUTH_JWT_SECRET set to a random 256-bit value
[ ] AEGISCLAW_AUTH_RECEIPT_HMAC_KEY set to a random 256-bit value
[ ] POSTGRES_PASSWORD changed from default
[ ] MINIO_ROOT_USER and MINIO_ROOT_PASSWORD changed from defaults
[ ] GF_SECURITY_ADMIN_PASSWORD changed from default
[ ] AEGISCLAW_SERVER_CORS_ORIGINS set to actual domain(s)
[ ] TLS termination configured via reverse proxy
[ ] AEGISCLAW_DATABASE_SSL_MODE=require (or verify-full)
[ ] Resource limits reviewed and adjusted for workload
[ ] Default admin password changed after first login
[ ] Firewall rules restrict access to management ports (9001, 3001, 16686, 8222, 9190)
[ ] AEGISCLAW_SERVER_PLAYBOOK_DIR set if playbooks are not in the default "playbooks" directory
```

---

## 7. Health Checks

Every AegisClaw service exposes an HTTP health endpoint. All return a JSON response in the format:

```json
{"status": "healthy", "service": "<service-name>"}
```

### Health Endpoints

| Service              | Health Endpoint                       |
| -------------------- | ------------------------------------- |
| api-gateway          | `http://localhost:8080/healthz`        |
| orchestrator         | `http://localhost:10090/healthz`       |
| runner               | `http://localhost:10091/healthz`       |
| evidence-service     | `http://localhost:10092/healthz`       |
| connector-service    | `http://localhost:10093/healthz`       |
| reporting-service    | `http://localhost:10094/healthz`       |
| ollama-bridge        | `http://localhost:10095/healthz`       |
| scheduler            | `http://localhost:10096/healthz`       |

### Checking All Services

Quick script to verify all services are healthy:

```bash
#!/bin/bash
services=(
  "api-gateway:8080"
  "orchestrator:10090"
  "runner:10091"
  "evidence-service:10092"
  "connector-service:10093"
  "reporting-service:10094"
  "ollama-bridge:10095"
  "scheduler:10096"
)

for svc in "${services[@]}"; do
  name="${svc%%:*}"
  port="${svc##*:}"
  status=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:${port}/healthz" 2>/dev/null)
  if [ "$status" = "200" ]; then
    echo "[OK]   $name (port $port)"
  else
    echo "[FAIL] $name (port $port) - HTTP $status"
  fi
done
```

### Docker Compose Health Status

```bash
docker compose -f deploy/docker-compose.yml ps
```

Healthy containers show `(healthy)` in the status column. Docker Compose uses the same `/healthz` endpoints internally.

---

## 8. CLI Tool

AegisClaw ships a CLI tool called `aegiscli` for scripted operations and administration.

### Installation

From the repository root (requires Go 1.25+):

```bash
go install ./cmd/aegiscli
```

Or build it with the Makefile:

```bash
make build
# Binary is at bin/aegiscli
```

### Configuration

Set the API gateway URL via environment variable:

```bash
export AEGISCLAW_API_URL=http://localhost:8080
```

### Login

```bash
aegiscli login --email admin@aegisclaw.local --password admin
```

The CLI stores the JWT token locally for subsequent commands.

### Common Commands

```bash
# List assets
aegiscli assets list

# List engagements
aegiscli engagements list

# View findings
aegiscli findings list

# Trigger a validation run
aegiscli runs create --engagement-id <uuid>

# View run status
aegiscli runs get <run-id>

# List connector instances
aegiscli connectors list
```

---

## 9. Backup & Restore

### PostgreSQL Backup

Use `pg_dump` to create a full database backup:

```bash
# Backup
docker exec aegisclaw-postgres pg_dump \
  -U aegisclaw \
  -d aegisclaw \
  --format=custom \
  --file=/tmp/aegisclaw-backup.dump

# Copy backup file to host
docker cp aegisclaw-postgres:/tmp/aegisclaw-backup.dump ./aegisclaw-backup.dump
```

### PostgreSQL Restore

```bash
# Copy backup file into container
docker cp ./aegisclaw-backup.dump aegisclaw-postgres:/tmp/aegisclaw-backup.dump

# Restore
docker exec aegisclaw-postgres pg_restore \
  -U aegisclaw \
  -d aegisclaw \
  --clean \
  --if-exists \
  /tmp/aegisclaw-backup.dump
```

### MinIO Backup (Evidence Vault)

Use the MinIO client (`mc`) to mirror the evidence bucket:

```bash
# Configure mc alias
mc alias set aegisclaw http://localhost:9000 minioadmin minioadmin

# Mirror bucket to local directory
mc mirror aegisclaw/aegisclaw-evidence ./backup/evidence/

# Mirror to another S3-compatible target
mc mirror aegisclaw/aegisclaw-evidence s3/backup-bucket/aegisclaw-evidence/
```

### MinIO Restore

```bash
# Restore from local directory
mc mirror ./backup/evidence/ aegisclaw/aegisclaw-evidence
```

### Automated Backup Script

```bash
#!/bin/bash
set -euo pipefail

BACKUP_DIR="./backups/$(date +%Y-%m-%d)"
mkdir -p "$BACKUP_DIR"

echo "Backing up PostgreSQL..."
docker exec aegisclaw-postgres pg_dump \
  -U aegisclaw -d aegisclaw --format=custom \
  --file=/tmp/aegisclaw.dump
docker cp aegisclaw-postgres:/tmp/aegisclaw.dump "$BACKUP_DIR/aegisclaw.dump"

echo "Backing up MinIO evidence..."
mc mirror --quiet aegisclaw/aegisclaw-evidence "$BACKUP_DIR/evidence/"

echo "Backup complete: $BACKUP_DIR"
```

---

## 10. Troubleshooting

### Service Won't Start

**Symptom:** A container exits immediately or keeps restarting.

1. Check the logs:
   ```bash
   docker compose -f deploy/docker-compose.yml logs <service-name>
   ```

2. Common causes:
   - **Missing or invalid JWT secret:** The api-gateway and orchestrator require `AEGISCLAW_AUTH_JWT_SECRET`. If unset or empty, the service exits with an error. Set it in `.env`.
   - **Database connection refused:** Ensure the `postgres` container is healthy before application services start. Docker Compose `depends_on` with `condition: service_healthy` handles this, but verify with:
     ```bash
     docker exec aegisclaw-postgres pg_isready -U aegisclaw
     ```
   - **Port conflict:** Another process is using a required port. Check with:
     ```bash
     lsof -i :8080  # or whichever port
     ```

### Login Fails

**Symptom:** `401 Unauthorized` when logging in with `admin@aegisclaw.local`.

1. Verify the seed script ran successfully:
   ```bash
   docker exec aegisclaw-postgres psql -U aegisclaw -d aegisclaw \
     -c "SELECT email, role FROM users WHERE email = 'admin@aegisclaw.local';"
   ```
   If no rows returned, run `make seed` again.

2. Verify the password hash is valid. The seed script inserts a bcrypt hash for the password `admin`. If you modified the seed script, ensure the hash matches the intended password. Generate a new hash:
   ```bash
   # Using htpasswd
   htpasswd -nbBC 10 "" "your-password" | cut -d: -f2
   ```

3. Check that the JWT secret is consistent across services. The `api-gateway` signs tokens and other services verify them. All must use the same `AEGISCLAW_AUTH_JWT_SECRET`.

### CORS Errors

**Symptom:** Browser console shows `Access-Control-Allow-Origin` errors.

1. Check that `AEGISCLAW_SERVER_CORS_ORIGINS` includes the exact origin the browser uses (protocol + host + port):
   ```bash
   # Correct for local development:
   AEGISCLAW_SERVER_CORS_ORIGINS=http://localhost:3000
   ```

2. If using a reverse proxy, ensure it does not strip or duplicate CORS headers. The api-gateway handles CORS -- do not add CORS headers at the proxy layer.

3. Restart the api-gateway after changing the CORS setting:
   ```bash
   docker compose -f deploy/docker-compose.yml restart api-gateway
   ```

### NATS Connection Issues

**Symptom:** Services log `could not connect to NATS` or `connection refused`.

1. Verify NATS is running and healthy:
   ```bash
   docker compose -f deploy/docker-compose.yml ps nats
   ```

2. Check NATS monitoring endpoint:
   ```bash
   curl http://localhost:8222/varz
   ```

3. Inside Docker, services connect to `nats://nats:4222` (the container hostname). On bare-metal, use `nats://localhost:4222`. Ensure `AEGISCLAW_NATS_URL` matches your setup.

4. NATS reconnect settings: By default, services retry 60 times with a 2-second delay. If NATS starts slowly, this should be sufficient. Increase `AEGISCLAW_NATS_MAX_RECONNECTS` if needed.

### Ollama Model Not Found

**Symptom:** LLM reasoning tasks fail with "model not found".

1. Pull the required model into Ollama:
   ```bash
   docker exec aegisclaw-ollama ollama pull llama3.1
   ```

2. Verify the model is available:
   ```bash
   docker exec aegisclaw-ollama ollama list
   ```

3. Check that `AEGISCLAW_OLLAMA_DEFAULT_MODEL` matches a model you have pulled.

### Evidence Upload Failures

**Symptom:** Evidence collection fails with S3/MinIO errors.

1. Verify MinIO is healthy:
   ```bash
   curl http://localhost:9000/minio/health/live
   ```

2. Check that the evidence bucket exists:
   ```bash
   mc alias set aegisclaw http://localhost:9000 minioadmin minioadmin
   mc ls aegisclaw/aegisclaw-evidence
   ```

3. Create the bucket if it does not exist:
   ```bash
   mc mb aegisclaw/aegisclaw-evidence
   ```

---

## 11. Service Ports Reference

### Application Ports

| Service              | API Port | gRPC Port | Health Port |
| -------------------- | -------- | --------- | ----------- |
| api-gateway          | 8080     | --        | 8080        |
| orchestrator         | --       | 9090      | 10090       |
| runner               | --       | 9091      | 10091       |
| evidence-service     | --       | 9092      | 10092       |
| connector-service    | --       | 9093      | 10093       |
| reporting-service    | --       | 9094      | 10094       |
| ollama-bridge        | --       | 9095      | 10095       |
| scheduler            | --       | 9096      | 10096       |
| web (frontend)       | 3000     | --        | 3000        |

### Infrastructure Ports

| Service      | Port(s)      | Protocol | Purpose                     |
| ------------ | ------------ | -------- | --------------------------- |
| PostgreSQL   | 5432         | TCP      | Database connections        |
| NATS         | 4222         | TCP      | Client connections          |
| NATS         | 8222         | HTTP     | Monitoring API              |
| MinIO        | 9000         | HTTP     | S3 API                      |
| MinIO        | 9001         | HTTP     | Web Console                 |
| Ollama       | 11434        | HTTP     | LLM inference API           |
| Jaeger       | 16686        | HTTP     | Tracing UI                  |
| Jaeger       | 4317         | gRPC     | OTLP collector              |
| Grafana      | 3001         | HTTP     | Dashboard UI                |
| Prometheus   | 9190         | HTTP     | Metrics UI / scrape config  |

### Makefile Targets Reference

| Target              | Command                                        | Description                          |
| ------------------- | ---------------------------------------------- | ------------------------------------ |
| `make docker-up`    | `docker compose -f deploy/docker-compose.yml up -d` | Start all services                   |
| `make docker-down`  | `docker compose -f deploy/docker-compose.yml down`  | Stop all services                    |
| `make docker-build` | Build all Docker images                        | Build images without starting        |
| `make docker-logs`  | Tail logs from all services                    | Follow combined log output           |
| `make infra-up`     | Start infrastructure only                      | Postgres, NATS, MinIO, Ollama, etc.  |
| `make migrate`      | Run database migrations                        | Apply pending schema changes         |
| `make migrate-down` | Rollback last migration                        | Undo the most recent migration       |
| `make seed`         | Seed database                                  | Insert default org, admin, assets    |
| `make build`        | Build all Go binaries                          | Output to `bin/` directory           |
| `make test`         | Run all tests                                  | With race detection                  |
| `make lint`         | Run linters                                    | Requires `golangci-lint`             |
