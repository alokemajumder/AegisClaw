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
- Go 1.23+
- Node.js 20+
- Docker and Docker Compose
- Make

### Getting Started

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/AegisClaw.git
cd AegisClaw

# Start infrastructure
make infra-up

# Install dependencies
go mod tidy
cd web && npm install && cd ..

# Run all tests
make test

# Run linters
make lint
```

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
Adding new connectors is a great way to contribute. See [Connector Development Guide](docs/connector-development.md).

### Playbook Library
Expanding the validation playbook library helps everyone. See [Playbook Authoring Guide](docs/playbook-authoring.md).

### Documentation
Improving documentation is always welcome — tutorials, examples, and clarifications.

## Code of Conduct

Be respectful, inclusive, and constructive. We're all here to build better security tooling.

## License

By contributing to AegisClaw, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
