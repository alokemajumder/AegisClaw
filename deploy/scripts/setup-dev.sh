#!/bin/bash
set -euo pipefail

echo "=== AegisClaw Development Setup ==="

# Check prerequisites
command -v go >/dev/null 2>&1 || { echo "Go is required but not installed."; exit 1; }
command -v node >/dev/null 2>&1 || { echo "Node.js is required but not installed."; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "Docker is required but not installed."; exit 1; }

echo "1. Installing Go dependencies..."
go mod tidy

echo "2. Installing frontend dependencies..."
cd web && npm install && cd ..

echo "3. Starting infrastructure (Postgres, NATS, MinIO, Ollama)..."
docker compose -f deploy/docker-compose.yml up -d postgres nats minio ollama

echo "4. Waiting for Postgres to be ready..."
until docker compose -f deploy/docker-compose.yml exec -T postgres pg_isready -U aegisclaw; do
    sleep 1
done

echo "5. Running database migrations..."
# Migrations would be run here once the migrate command is implemented

echo "6. Pulling Ollama model..."
docker compose -f deploy/docker-compose.yml exec ollama ollama pull llama3.1 || echo "Skipping model pull (can be done later)"

echo ""
echo "=== Setup Complete ==="
echo "Infrastructure is running. Start development with:"
echo "  make dev-api        # Start API gateway"
echo "  make dev-web        # Start frontend"
echo "  make dev-orchestrator  # Start orchestrator"
