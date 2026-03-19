.PHONY: build test lint clean docker-up docker-down migrate seed help

# Go settings
GOBIN := $(shell go env GOPATH)/bin
MODULE := github.com/alokemajumder/AegisClaw

# Services
SERVICES := api-gateway orchestrator runner evidence-service connector-service reporting-service ollama-bridge scheduler
CLI := aegiscli

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build all services
	@echo "Building services..."
	@for svc in $(SERVICES); do \
		echo "  Building $$svc..."; \
		go build -o bin/$$svc ./cmd/$$svc; \
	done
	@echo "  Building CLI..."
	@go build -o bin/$(CLI) ./cmd/$(CLI)
	@echo "All services built successfully"

build-%: ## Build a specific service (e.g., make build-api-gateway)
	go build -o bin/$* ./cmd/$*

test: ## Run all tests
	go test ./... -v -race -count=1

test-short: ## Run tests without integration tests
	go test ./... -short -v -race -count=1

test-coverage: ## Run tests with coverage report
	go test ./... -v -race -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint: ## Run linters
	golangci-lint run ./...

lint-fix: ## Run linters with auto-fix
	golangci-lint run --fix ./...

fmt: ## Format code
	gofmt -s -w .
	goimports -w .

tidy: ## Tidy Go modules
	go mod tidy

clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html

# Database
migrate: ## Run database migrations
	@echo "Running migrations..."
	@go run ./cmd/api-gateway migrate

migrate-cli: ## Run migrations via golang-migrate CLI (alternative)
	migrate -path internal/database/migrations -database "postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(POSTGRES_DB)?sslmode=disable" up

# Docker
docker-up: ## Start all services with Docker Compose
	docker compose -f deploy/docker-compose.yml up -d

docker-down: ## Stop all Docker Compose services
	docker compose -f deploy/docker-compose.yml down

nvidia-up: ## Start with NVIDIA GPU acceleration (requires nvidia-docker2)
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.nvidia.yml up -d

nvidia-down: ## Stop NVIDIA GPU services
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.nvidia.yml down

docker-build: ## Build all Docker images
	@for svc in $(SERVICES); do \
		echo "Building Docker image for $$svc..."; \
		docker build -f deploy/docker/Dockerfile.$$svc -t aegisclaw-$$svc:latest .; \
	done

docker-logs: ## Tail logs from all services
	docker compose -f deploy/docker-compose.yml logs -f

# Development
dev-api: ## Run API gateway in development mode
	go run ./cmd/api-gateway

dev-orchestrator: ## Run orchestrator in development mode
	go run ./cmd/orchestrator

dev-web: ## Run frontend in development mode
	cd web && npm run dev

# Infrastructure
infra-up: ## Start infrastructure only (DB, NATS, MinIO, Ollama)
	docker compose -f deploy/docker-compose.yml up -d postgres nats minio ollama jaeger grafana prometheus

infra-down: ## Stop infrastructure
	docker compose -f deploy/docker-compose.yml down

seed: ## Seed database with sample data
	@echo "Seeding database..."
	@bash deploy/scripts/seed.sh

# Security
security-scan: ## Run security scanners
	gosec ./...
	trivy fs .

# Generate
generate: ## Run code generation (sqlc, protobuf, etc.)
	@echo "Running code generation..."
	# sqlc generate
	# protoc generation would go here
