# AegisClaw — Project Instructions

## Overview
AegisClaw is an Autonomous Security Validation Platform. It runs safe, ATT&CK-mapped security validations continuously via agent squads (Red + Blue + Purple loop), proves outcomes with audit-grade receipts, and keeps all data and LLM reasoning local via Ollama.

## Tech Stack
- **Backend**: Go 1.23+ (monorepo with go.work)
- **Frontend**: Next.js 15 + React 19 + Tailwind CSS + shadcn/ui
- **Database**: PostgreSQL 16 (migrations via golang-migrate, queries via sqlc)
- **Message Broker**: NATS + JetStream
- **Blob Storage**: MinIO (S3-compatible)
- **LLM**: Ollama (local inference)
- **Sandbox**: gVisor (runner isolation)
- **Observability**: OpenTelemetry + Prometheus + Grafana + Jaeger

## Go Conventions
- Module path: `github.com/alokemajumder/AegisClaw`
- Use `slog` for structured logging (no third-party loggers)
- Use `context.Context` as first parameter for all public functions
- Use table-driven tests with `testify` assertions
- Error wrapping with `fmt.Errorf("operation: %w", err)`
- No global state; pass dependencies via structs
- Interfaces in the package that consumes them, not the package that implements them
- `internal/` for private packages, `pkg/` for public SDK packages

## API Conventions
- REST API via Chi router on api-gateway (port 8080)
- Internal services communicate via gRPC (ports 9090-9096)
- Async operations via NATS JetStream pub/sub
- All API responses use consistent JSON envelope: `{"data": ..., "error": ..., "meta": ...}`
- Pagination via `?page=1&per_page=50` with `meta.total` and `meta.page` in response
- UUIDs for all entity IDs

## Database Conventions
- Migrations in `internal/database/migrations/` using golang-migrate format
- All tables have `id UUID PRIMARY KEY`, `created_at TIMESTAMPTZ`, `updated_at TIMESTAMPTZ`
- Use `JSONB` for flexible metadata fields
- Audit log table is append-only (never UPDATE or DELETE)

## Testing
- Unit tests alongside source files (`*_test.go`)
- Integration tests use `testcontainers-go` for real Postgres/NATS/MinIO
- Run tests: `make test`
- Run lints: `make lint`

## Git Rules
- Never mention AI tools or assistants in commit messages or PR descriptions
- Commit messages: imperative mood, concise, focused on "why"
- Never commit secrets, .env files, or credentials
