# Contributing to AegisClaw

Thank you for your interest in contributing to AegisClaw! This document provides guidelines for contributing to the project.

## How to Contribute

### Reporting Issues

- Check existing [issues](https://github.com/alokemajumder/AegisClaw/issues) first
- Use the issue templates when available
- Include steps to reproduce, expected behavior, and actual behavior
- Include your environment details (OS, Go version, Docker version)

### Submitting Pull Requests

1. **Fork** the repository
2. **Create a branch** from `main`: `git checkout -b feature/your-feature`
3. **Make your changes** following the coding standards below
4. **Write tests** for new functionality
5. **Run checks** before submitting:
   ```bash
   make lint
   make test
   cd web && npm run lint && npm run build
   ```
6. **Submit a PR** against the `main` branch

### Pull Request Guidelines

- Keep PRs focused on a single change
- Write a clear PR description explaining what and why
- Reference related issues with `Fixes #123` or `Related to #456`
- All CI checks must pass before merge
- At least one maintainer review is required

## Development Setup

### Prerequisites
- Go 1.25+
- Node.js 20+
- Docker 24+ and Docker Compose v2
- Make
- At least 8 GB RAM for the full stack

### Getting Started

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/AegisClaw.git
cd AegisClaw

# Configure environment
cp .env.example .env
# Review .env and set AEGISCLAW_AUTH_JWT_SECRET

# Start infrastructure (PostgreSQL, NATS, MinIO, Prometheus, Grafana, Jaeger)
make infra-up

# Run database migrations
make migrate

# Install dependencies
go mod tidy
cd web && npm install && cd ..

# Run all tests (161 test functions, 400 test cases across 16 packages)
make test

# Run linters
make lint
```

### Running Services Locally

```bash
make dev-api          # API Gateway on :8080
make dev-orchestrator # Orchestrator on :9090
make dev-web          # Frontend on :3000 (proxies API via Next.js rewrites)
```

### Service Health Checks

All Go services expose `/healthz` HTTP endpoints. The API gateway uses port 8080; other services use their gRPC port + 1000 (e.g., Orchestrator gRPC on :9090, health on :10090).

## Coding Standards

### Go
- Follow standard Go conventions and idioms
- Use `gofmt` for formatting (enforced by CI)
- Use `golangci-lint` for linting (enforced by CI)
- Write table-driven tests with `testify` assertions
- Use structured logging via `slog`
- Use `context.Context` as the first parameter for functions with I/O
- Wrap errors with `fmt.Errorf("context: %w", err)`
- Interfaces belong in the consuming package

### Frontend (TypeScript/React)
- Follow ESLint and Prettier configuration
- Use TypeScript strict mode
- Use shadcn/ui components for UI elements
- Use Tailwind CSS for styling
- Keep components small and focused

### Commits
- Use imperative mood: "Add feature" not "Added feature"
- Keep the first line under 72 characters
- Focus on *why*, not *what*
- Reference issues when applicable

### Documentation
- Update docs for user-facing changes
- Add JSDoc/GoDoc comments for public APIs
- Keep README.md up to date

## Project Structure

| Directory | Description |
|-----------|-------------|
| `cmd/` | Service entrypoints (one per service) |
| `internal/` | Shared internal packages (not importable by external code) |
| `pkg/` | Public SDK packages (Connector SDK, Agent SDK) |
| `connectors/` | Built-in connector implementations |
| `agents/` | Agent squad implementations |
| `playbooks/` | Validation playbook YAML definitions |
| `web/` | Next.js frontend |
| `deploy/` | Docker, Compose, Helm, scripts |
| `docs/` | Documentation |
| `configs/` | Default configuration files |

## Areas for Contribution

### Good First Issues
Look for issues tagged with `good-first-issue` — these are accessible for newcomers.

### Connector Development
Adding new connectors is a great way to contribute. The platform currently ships 5 connectors (Sentinel, Defender, ServiceNow, Teams, Slack) with many more planned. See [Connector Development Guide](docs/connector-development.md).

### Agent Development
The platform uses 12 agents across 4 squads, all wired into a 3-phase `RunEngine` pipeline. Agents implement the `agentsdk.Agent` interface (`Name`, `Squad`, `Init`, `HandleTask`, `Shutdown`). Dependencies are injected via `agentsdk.AgentDeps` using `any`-typed fields to avoid circular imports — agents type-assert in their `Init()` methods. When adding or modifying agents, ensure they return honest data (zeros/empty) when dependencies are unavailable rather than simulated/fake results.

### Playbook Library
Expanding the validation playbook library helps everyone. Currently 13 playbooks across Tier 0-2. See [Playbook Authoring Guide](docs/playbook-authoring.md).

### Testing
We currently have 161 test functions (400 test cases including subtests) across 16 packages. Integration tests using `testcontainers-go` for real Postgres/NATS/MinIO are welcome.

### Documentation
Improving documentation is always welcome — tutorials, examples, and clarifications.

## Code of Conduct

Be respectful, inclusive, and constructive. We're all here to build better security tooling.

## License

By contributing to AegisClaw, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
