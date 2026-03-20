# NVIDIA GPU Deployment Guide

This guide covers deploying AegisClaw with [NVIDIA AI agent ecosystem](https://nvidianews.nvidia.com/news/ai-agents) acceleration, from a single consumer GPU for SMB/SME to multi-GPU enterprise configurations. AegisClaw integrates with [NVIDIA NIM](https://build.nvidia.com), [Nemotron 3](https://developer.nvidia.com/nemotron) models, [OpenShell](https://docs.nvidia.com/openshell/latest/index.html) sandboxing, [NeMoClaw](https://build.nvidia.com/nemoclaw) orchestration, [NeMo Guardrails](https://developer.nvidia.com/nemo-guardrails), and [Morpheus](https://developer.nvidia.com/morpheus-cybersecurity) analytics.

---

## Table of Contents

1. [Deployment Profiles](#1-deployment-profiles)
2. [Consumer GPU Setup (SMB/SME)](#2-consumer-gpu-setup-smbsme)
3. [NVIDIA NIM Setup](#3-nvidia-nim-setup)
4. [Nemotron 3 Model Family](#4-nemotron-3-model-family)
5. [OpenShell Agent Sandboxing](#5-openshell-agent-sandboxing)
6. [NeMo Guardrails](#6-nemo-guardrails)
7. [NVIDIA Morpheus Analytics](#7-nvidia-morpheus-analytics)
8. [Model Selection Guide](#8-model-selection-guide)
9. [Cost Optimization](#9-cost-optimization)
10. [Hardware Sizing](#10-hardware-sizing)

---

## 1. Deployment Profiles

AegisClaw supports three LLM deployment modes. Choose based on your budget and hardware.

| Profile | Backend | Hardware Needed | Monthly Cost | Best For |
|---------|---------|-----------------|--------------|----------|
| **Free / CPU** | Ollama (CPU-only) | Any x86 server, 16GB+ RAM | $0 (electricity) | Evaluation, small orgs |
| **Consumer GPU** | Ollama + GPU | RTX 3060-5090 | ~$30-65 electricity | SMB/SME (1-50 employees) |
| **NIM Cloud** | [NVIDIA API Catalog](https://build.nvidia.com) | None (API only) | ~$30-90 API fees | Teams without GPU hardware |
| **NIM Self-Hosted** | [NIM](https://build.nvidia.com) container + GPU | RTX 4090+ or DGX | ~$50-100 electricity | Mid-market, data sovereignty |
| **Enterprise** | [NIM](https://build.nvidia.com) + [Guardrails](https://developer.nvidia.com/nemo-guardrails) + [OpenShell](https://docs.nvidia.com/openshell/latest/index.html) + [DGX](https://www.nvidia.com/en-us/products/workstations/dgx-spark/) | DGX Spark/Station/BasePOD | Varies | Enterprise (500+ employees) |

---

## 2. Consumer GPU Setup (SMB/SME)

The cheapest way to run AegisClaw with GPU-accelerated LLM inference. Uses Ollama with a consumer NVIDIA GPU.

### Prerequisites

```bash
# 1. Install NVIDIA GPU driver (535+)
# Ubuntu/Debian:
sudo apt install nvidia-driver-535

# 2. Install NVIDIA Container Toolkit
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
sudo apt update && sudo apt install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker

# 3. Verify GPU is visible to Docker
docker run --rm --gpus all nvidia/cuda:12.4.0-base-ubuntu22.04 nvidia-smi
```

### Deploy with GPU

```bash
cd AegisClaw
cp .env.example .env
# Edit .env — set AEGISCLAW_AUTH_JWT_SECRET

# Deploy with GPU overlay
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.nvidia.yml up -d

# Pull a model sized for your GPU
docker exec aegisclaw-ollama ollama pull llama3.1      # 8B — needs 8GB VRAM
docker exec aegisclaw-ollama ollama pull llama3.2       # 3B — needs 4GB VRAM
docker exec aegisclaw-ollama ollama pull mistral         # 7B — needs 8GB VRAM

# Run migrations and seed
make migrate && make seed
```

### GPU-to-Model Mapping

| GPU | VRAM | Recommended Model | Tokens/sec (approx) |
|-----|------|-------------------|---------------------|
| RTX 3060 | 12GB | llama3.2 (3B) | ~80 tok/s |
| RTX 4060 Ti | 16GB | llama3.1 (8B) | ~60 tok/s |
| RTX 4070 Ti Super | 16GB | llama3.1 (8B) | ~75 tok/s |
| RTX 4080 Super | 16GB | mistral (7B) | ~90 tok/s |
| RTX 4090 | 24GB | llama3.1 (8B) or llama3.1:70b-q4 | ~120 / ~25 tok/s |
| RTX 5090 | 32GB | llama3.1:70b-q4 | ~35 tok/s |

> **Tip**: For budget-constrained deployments, a used RTX 3060 12GB (~$200) running llama3.2 provides adequate reasoning for security validation at minimal cost.

---

## 3. NVIDIA NIM Setup

[NVIDIA NIM](https://build.nvidia.com) (NeMo Inference Microservices) provides optimized inference containers for [Nemotron 3](https://developer.nvidia.com/nemotron) and other models. Two options: [cloud API](https://build.nvidia.com) or self-hosted.

### Option A: [NVIDIA API Catalog](https://build.nvidia.com) (Zero Hardware)

The fastest way to use [Nemotron](https://developer.nvidia.com/nemotron) models. Pay per token, no GPU required. Access 34+ hosted models including the full Nemotron 3 family.

```bash
# 1. Get an API key at https://build.nvidia.com
# 2. Add to .env:
AEGISCLAW_NVIDIA_NIM_ENABLED=true
AEGISCLAW_NVIDIA_NIM_URL=https://integrate.api.nvidia.com/v1
AEGISCLAW_NVIDIA_NIM_API_KEY=nvapi-xxxxxxxxxxxx
AEGISCLAW_NVIDIA_NIM_DEFAULT_MODEL=nvidia/nemotron-3-super-120b-a12b
# Optional: configure thinking budget for deeper reasoning (0=default, 1-10 scale)
AEGISCLAW_NVIDIA_NIM_THINKING_BUDGET=5

# 3. Deploy
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.nvidia.yml up -d
```

### Option B: Self-Hosted NIM (Data Sovereignty)

Run [NIM](https://build.nvidia.com) containers on your own hardware. All data stays on-premise.

```bash
# 1. Get NGC API key at https://ngc.nvidia.com
export NGC_API_KEY=<your-ngc-key>

# 2. Login to NVIDIA container registry
docker login nvcr.io -u '$oauthtoken' -p $NGC_API_KEY

# 3. Choose model based on your GPU:

# RTX 4090/5090 (24-32GB VRAM) — Nemotron 3 Nano (30B MoE, 3B active params)
docker run -d --gpus all \
  -e NGC_API_KEY=$NGC_API_KEY \
  -v nim-cache:/opt/nim/.cache \
  -p 8000:8000 \
  nvcr.io/nim/nvidia/nemotron-3-nano-30b-a3b:latest

# 2x RTX 4090 or A100 (48-80GB VRAM) — Nemotron 3 Super (120B MoE, 12B active)
docker run -d --gpus all \
  -e NGC_API_KEY=$NGC_API_KEY \
  -v nim-cache:/opt/nim/.cache \
  -p 8000:8000 \
  nvcr.io/nim/nvidia/nemotron-3-super-120b-a12b:latest

# DGX Spark/Station (128-784GB) — Nemotron Ultra (253B, maximum reasoning)
docker run -d --gpus all \
  -e NGC_API_KEY=$NGC_API_KEY \
  -v nim-cache:/opt/nim/.cache \
  -p 8000:8000 \
  nvcr.io/nim/nvidia/llama-nemotron-ultra-253b:latest

# 4. Point AegisClaw at local NIM:
AEGISCLAW_NVIDIA_NIM_ENABLED=true
AEGISCLAW_NVIDIA_NIM_URL=http://localhost:8000/v1
AEGISCLAW_NVIDIA_NIM_DEFAULT_MODEL=nvidia/nemotron-3-nano-30b-a3b
```

### NIM Health Check

```bash
# Verify NIM is running
curl http://localhost:8000/v1/health/ready

# List available models
curl http://localhost:8000/v1/models

# Test inference with tool-calling
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nvidia/nemotron-3-nano-30b-a3b",
    "messages": [{"role": "user", "content": "Analyze this SIEM alert"}],
    "tools": [{"type": "function", "function": {"name": "query_siem", "description": "Query SIEM for events", "parameters": {"type": "object", "properties": {"query": {"type": "string"}}}}}],
    "tool_choice": "auto"
  }'
```

---

## 4. Nemotron 3 Model Family

The [Nemotron 3](https://developer.nvidia.com/nemotron) family represents NVIDIA's latest generation of models, featuring hybrid Mamba-Transformer Mixture-of-Experts (MoE) architecture with **1 million token context windows**. These models are purpose-built for [AI agent](https://nvidianews.nvidia.com/news/ai-agents) workloads like AegisClaw's multi-agent security validation pipeline.

### Key Benefits for Security Validation

- **1M context window** — Ingest entire SIEM log batches, full playbook definitions, and multi-step evidence chains in a single prompt
- **Hybrid Mamba-Transformer MoE** — Only activates a fraction of parameters per token (e.g., 3B active out of 30B total), delivering high quality at low latency
- **Configurable thinking budget** — Tune reasoning depth (0-10 scale) per task: shallow for fast triage, deep for complex threat analysis
- **[Tool-calling](https://docs.nvidia.com/nim/large-language-models/latest/function-calling.html)** — OpenAI-compatible function calling enables agents to invoke SIEM queries, EDR checks, and ticket creation directly from LLM reasoning

### Model Lineup

| Model | Architecture | Active Params | Total Params | Context | Hardware | Best For |
|-------|-------------|---------------|-------------|---------|----------|----------|
| [**Nemotron 3 Nano**](https://developer.nvidia.com/nemotron) | Mamba-Transformer MoE | 3B | 30B | 1M | RTX 4090/5090 (24GB) | SMB: fast triage, finding summaries |
| [**Nemotron 3 Super**](https://developer.nvidia.com/nemotron) | Mamba-Transformer MoE | 12B | 120B | 1M | 2×RTX or A100 | Multi-agent enterprise reasoning |
| [**Nemotron Ultra**](https://developer.nvidia.com/nemotron) | Dense | 253B | 253B | 128K | DGX / multi-GPU | Maximum reasoning, compliance reports |
| [**Nemotron Nano VL**](https://developer.nvidia.com/nemotron) | Vision-Language | 12B | 12B | — | RTX 4090 | Document intelligence, video analysis |
| [**Nemotron Safety**](https://developer.nvidia.com/nemotron) | Classifier | — | — | — | Any GPU | Multilingual safety classification |

### Specialized Models via [NIM](https://build.nvidia.com)

AegisClaw also supports these models through the NIM OpenAI-compatible API:

| Model | Provider | Strengths | Use Case |
|-------|----------|-----------|----------|
| [**DeepSeek V3**](https://build.nvidia.com) | DeepSeek AI | 685B, 128K context, strict function calling | Complex multi-step agent tool-calling |
| [**Meta Llama 3.3 70B**](https://build.nvidia.com) | Meta | Strong general reasoning | Fallback for broad analysis tasks |

### Configuring Thinking Budget

The thinking budget controls how deeply [Nemotron](https://developer.nvidia.com/nemotron) models reason before responding. Higher values produce more thorough analysis at the cost of latency.

```bash
# In .env — set reasoning depth (0=default model behavior, 1-10 scale)
AEGISCLAW_NVIDIA_NIM_THINKING_BUDGET=0   # Default: fastest responses
AEGISCLAW_NVIDIA_NIM_THINKING_BUDGET=3   # Light reasoning: finding classification
AEGISCLAW_NVIDIA_NIM_THINKING_BUDGET=7   # Deep reasoning: threat modeling, coverage analysis
AEGISCLAW_NVIDIA_NIM_THINKING_BUDGET=10  # Maximum: complex compliance reports, attack chain analysis
```

---

## 5. OpenShell Agent Sandboxing

[NVIDIA OpenShell](https://docs.nvidia.com/openshell/latest/index.html) is an Apache 2.0 runtime that provides sandboxed execution for AI agents. AegisClaw uses OpenShell to isolate each validation step in a security sandbox with 4 layers of protection, ensuring that even compromised agent logic cannot escape its governance tier boundaries.

### Why OpenShell for Security Validation?

Traditional sandboxing (Docker, gVisor) provides process isolation but lacks the fine-grained, policy-driven controls needed for security agent execution. [OpenShell](https://docs.nvidia.com/openshell/latest/index.html) provides:

1. **[Landlock LSM](https://docs.nvidia.com/openshell/latest/index.html)** — Kernel-level filesystem access control. Tier 0 agents get read-only access; Tier 1 can write to `/sandbox` and `/tmp`; Tier 2 gets connector-scoped write access.
2. **[seccomp](https://docs.nvidia.com/openshell/latest/index.html)** — System call filtering. Each tier defines which binaries can execute (Tier 0: `curl` only; Tier 1: `curl` + `python3`; Tier 2: `curl` + `python3` + `bash`).
3. **Network namespacing** — Deny-by-default egress with per-tier allowlists. Sandboxes can only reach explicitly permitted endpoints.
4. **[Privacy Router](https://docs.nvidia.com/openshell/latest/index.html)** — Sandboxes call `inference.local` for LLM access. The Gateway routes this to Ollama or [NIM](https://build.nvidia.com), stripping sandbox credentials and injecting real provider credentials. Agents never see API keys.

### Architecture

```
  Agent Step (Tier 1)
        │
        ▼
  ┌─────────────────────┐
  │  OpenShell Gateway   │  ◄── mTLS / token auth
  │  Policy Manager      │
  └────┬──────┬──────┬──┘
       │      │      │
  ┌────▼──┐ ┌─▼───┐ ┌▼────────────┐
  │Sandbox│ │Policy│ │Privacy Router│
  │(step) │ │(v1)  │ │inference.local│
  └───────┘ └──────┘ └──────┬───────┘
                            │
                     ┌──────▼──────┐
                     │ Ollama/NIM  │
                     └─────────────┘
```

### Tier-to-Policy Mapping

AegisClaw's `PolicyGenerator` automatically maps governance tiers to [OpenShell v1 policies](https://docs.nvidia.com/openshell/latest/index.html):

| Tier | Filesystem | Network | Binaries | Landlock |
|------|-----------|---------|----------|----------|
| **0 — Passive** | Read-only (`/etc`, `/usr`, `/opt`) | REST/443 only | `curl` | Hard requirement |
| **1 — Benign** | `/sandbox` + `/tmp` writable | Connectors + DNS (53) | `curl`, `python3` | Hard requirement |
| **2 — Sensitive** | Connector-scoped read-write | Full connector access | `curl`, `python3`, `bash` | Hard requirement |
| **3 — Prohibited** | Rejected — no sandbox created | — | — | — |

### Setup

```bash
# 1. Enable sandboxing in .env
AEGISCLAW_SANDBOX_ENABLED=true
AEGISCLAW_SANDBOX_RUNTIME_URL=https://localhost:8765

# 2. Choose auth mode:

# Option A: mTLS (recommended for production)
AEGISCLAW_SANDBOX_AUTH_MODE=mtls
AEGISCLAW_SANDBOX_CERT_FILE=/etc/aegisclaw/certs/client.crt
AEGISCLAW_SANDBOX_KEY_FILE=/etc/aegisclaw/certs/client.key
AEGISCLAW_SANDBOX_CA_FILE=/etc/aegisclaw/certs/ca.crt

# Option B: Bearer token
AEGISCLAW_SANDBOX_AUTH_MODE=token
AEGISCLAW_SANDBOX_GATEWAY_TOKEN=<your-gateway-token>

# Option C: Plaintext (dev/trusted proxy only)
AEGISCLAW_SANDBOX_AUTH_MODE=none

# 3. Optional: Enable GPU passthrough for in-sandbox inference
AEGISCLAW_SANDBOX_GPU=true
```

### Privacy Router Configuration

The [OpenShell Privacy Router](https://docs.nvidia.com/openshell/latest/index.html) ensures sandboxed agents can use LLM inference without accessing provider credentials:

```bash
# AegisClaw automatically configures the Privacy Router on startup.
# It routes inference.local → your configured Ollama/NIM endpoint.
# No manual configuration needed — the Runner's ConnectGateway() handles this.

# The Ollama URL is passed through to the Privacy Router:
AEGISCLAW_OLLAMA_URL=http://localhost:11434

# If NIM is enabled, the Privacy Router routes to NIM as primary:
AEGISCLAW_NVIDIA_NIM_ENABLED=true
AEGISCLAW_NVIDIA_NIM_URL=http://localhost:8000/v1
```

---

## 6. NeMo Guardrails

[NeMo Guardrails](https://developer.nvidia.com/nemo-guardrails) NIMs add safety layers to all LLM prompts: content safety, jailbreak detection, and topic control. When enabled, every prompt sent to Ollama or [NIM](https://build.nvidia.com) is screened before reaching the inference backend.

### When to Enable

- **Required**: Production deployments with untrusted inputs, compliance-sensitive environments
- **Recommended**: Any multi-user deployment
- **Optional**: Single-operator, air-gapped deployments

### Setup

```bash
# Each guardrail runs as a lightweight NIM container (~2GB VRAM each)
# You can run all three on a single GPU alongside Ollama or NIM

# Content Safety
docker run -d --gpus all \
  -e NGC_API_KEY=$NGC_API_KEY \
  -p 8180:8000 \
  nvcr.io/nim/nvidia/nemo-guardrails-content-safety:latest

# Jailbreak Detection
docker run -d --gpus all \
  -e NGC_API_KEY=$NGC_API_KEY \
  -p 8181:8000 \
  nvcr.io/nim/nvidia/nemo-guardrails-jailbreak:latest

# Enable in .env
AEGISCLAW_NEMO_GUARDRAILS_ENABLED=true
AEGISCLAW_NEMO_GUARDRAILS_CONTENT_SAFETY_URL=http://localhost:8180/v1
AEGISCLAW_NEMO_GUARDRAILS_JAILBREAK_URL=http://localhost:8181/v1
```

---

## 7. NVIDIA Morpheus Analytics

[NVIDIA Morpheus](https://developer.nvidia.com/morpheus-cybersecurity) is a GPU-accelerated security analytics framework that uses [Triton Inference Server](https://developer.nvidia.com/triton-inference-server) to run AI models on security data at scale. AegisClaw integrates Morpheus as a connector for real-time threat detection and log analysis.

### Why Morpheus for Security Validation?

- **GPU-accelerated inference** — Process security logs 10-100x faster than CPU-based ML pipelines
- **Pre-built security models** — Sensitive Information Detection (SID), Digital Fingerprinting, Phishing Detection, Root Cause Analysis
- **[Triton Inference Server](https://developer.nvidia.com/triton-inference-server)** — Production-grade model serving with dynamic batching, model versioning, and multi-model concurrency
- **Real-time streaming** — Kafka-based data pipeline for continuous security event analysis

### AegisClaw Morpheus Connector

The Morpheus connector (`connectors/analytics/morpheus/morpheus.go`) integrates via the [Triton Inference Server v2 API](https://developer.nvidia.com/triton-inference-server):

| Capability | API Endpoint | Purpose |
|-----------|-------------|---------|
| Health check | `GET /v2/health/live` | Verify Triton server is running |
| Model readiness | `GET /v2/models/{model}/ready` | Check if security model is loaded |
| Inference | `POST /v2/models/{model}/infer` | Run threat detection on security events |

### Setup

```bash
# 1. Configure Morpheus connector via API or Settings UI:
#    - triton_url: http://triton-server:8000
#    - kafka_brokers: kafka:9092
#    - model_name: sid-minibert (default)
#    - api_key: (optional, for authenticated Triton endpoints)

# 2. Deploy Triton with a security model:
docker run -d --gpus all \
  -v /models:/models \
  -p 8000:8000 -p 8001:8001 -p 8002:8002 \
  nvcr.io/nvidia/tritonserver:24.01-py3 \
  tritonserver --model-repository=/models
```

### Supported Models

| Model | Type | Purpose |
|-------|------|---------|
| **SID-Minibert** | NLP | Sensitive Information Detection in logs and alerts |
| **Digital Fingerprinting** | Anomaly | Behavioral anomaly detection for users and machines |
| **Phishing Detection** | Classification | Email and URL phishing analysis |
| **Root Cause Analysis** | Graph | Automated incident root cause identification |

---

## 8. Model Selection Guide

### [Nemotron 3](https://developer.nvidia.com/nemotron) Models (via [NIM](https://build.nvidia.com))

| Model | Architecture | Active / Total Params | VRAM | Context | Use Case |
|-------|-------------|----------------------|------|---------|----------|
| [**Nemotron 3 Nano**](https://developer.nvidia.com/nemotron) | Mamba-Transformer MoE | 3B / 30B | 24GB | 1M | SMB: finding summaries, fast triage |
| [**Nemotron 3 Super**](https://developer.nvidia.com/nemotron) | Mamba-Transformer MoE | 12B / 120B | 48-80GB | 1M | Multi-agent enterprise reasoning |
| [**Nemotron Ultra**](https://developer.nvidia.com/nemotron) | Dense | 253B | 500GB+ | 128K | Maximum reasoning, compliance reports |
| [**Nemotron Nano VL**](https://developer.nvidia.com/nemotron) | Vision-Language | 12B | 24GB | — | Document/video analysis |
| [**Nemotron Safety**](https://developer.nvidia.com/nemotron) | Classifier | — | 4GB | — | Multilingual safety classification |

### Open-Source Models (via Ollama — free)

| Model | Size | VRAM | Best For |
|-------|------|------|----------|
| **llama3.2** | 3B | 4GB | Budget deployments, fast responses |
| **llama3.1** | 8B | 8GB | General purpose, good reasoning |
| **mistral** | 7B | 8GB | Fast, good at structured output |
| **phi3** | 3.8B | 4GB | Extremely efficient, Microsoft-optimized |
| **gemma2** | 9B | 10GB | Strong reasoning, Google-optimized |
| **qwen2.5** | 7B | 8GB | Multilingual, strong coding |
| **llama3.1:70b-q4** | 70B (Q4) | 24GB | Near-Nemotron quality on consumer GPU |
| **deepseek-r1:8b** | 8B | 8GB | Strong reasoning, chain-of-thought |

> **SMB Recommendation**: Start with `llama3.1` (8B) on Ollama. It's free, runs on an RTX 3060+, and provides solid reasoning for security validation. Upgrade to [Nemotron 3 Nano](https://developer.nvidia.com/nemotron) via [NIM](https://build.nvidia.com) when budget allows — the 30B MoE with only 3B active params delivers significantly better quality at comparable latency.

---

## 9. Cost Optimization

### Self-Hosted vs. Cloud (Break-Even Analysis)

| Scenario | Monthly Cost | Notes |
|----------|-------------|-------|
| **Ollama on CPU** | $0 | Slow (~5 tok/s), but functional |
| **Ollama on RTX 4090** | ~$50/mo electricity | ~120 tok/s, one-time $1,600 GPU cost |
| **Ollama on RTX 3060** | ~$20/mo electricity | ~80 tok/s for 3B model, one-time $200-300 GPU cost |
| **[NIM Cloud API](https://build.nvidia.com) (light)** | ~$30/mo | 50K tokens/day, no hardware |
| **[NIM Cloud API](https://build.nvidia.com) (heavy)** | ~$90/mo | 200K tokens/day, no hardware |
| **Self-hosted [NIM](https://build.nvidia.com) + RTX 4090** | ~$50/mo electricity | Unlimited tokens, one-time $1,600 GPU |

**Break-even for self-hosted RTX 4090 vs. cloud API**: 6-12 months at moderate usage.

### Budget Deployment (Under $500 Total)

For the smallest viable GPU deployment:

1. Used RTX 3060 12GB: ~$200-250
2. Basic server (used mini PC/workstation): ~$200-300
3. Run `llama3.2` (3B) via Ollama
4. Monthly electricity: ~$15-20

**Total first-year cost: ~$700** with unlimited inference.

### Mid-Range Deployment ($2,000-$5,000)

1. RTX 4090 24GB: ~$1,600
2. Server with 32GB+ RAM: ~$800-1,500
3. Run `llama3.1` (8B) or quantized 70B via Ollama, or [Nemotron 3 Nano](https://developer.nvidia.com/nemotron) via [NIM](https://build.nvidia.com)
4. Monthly electricity: ~$50

### Enterprise ([NVIDIA Inception Program](https://www.nvidia.com/en-us/startups/))

NVIDIA's [Inception program](https://www.nvidia.com/en-us/startups/) offers **75% discount** on [NVIDIA AI Enterprise](https://www.nvidia.com/en-us/data-center/products/ai-enterprise/) licenses ($4,500/GPU/year → ~$1,125/GPU/year). Includes enterprise support for [NIM](https://build.nvidia.com), [NeMo Guardrails](https://developer.nvidia.com/nemo-guardrails), [Morpheus](https://developer.nvidia.com/morpheus-cybersecurity), and [OpenShell](https://docs.nvidia.com/openshell/latest/index.html).

Apply at: https://www.nvidia.com/en-us/startups/

---

## 10. Hardware Sizing

### Minimum Viable (SMB — 1-10 users)

```
CPU:  4 cores (8 recommended)
RAM:  16 GB (32 recommended)
GPU:  RTX 3060 12GB (optional but recommended)
Disk: 50 GB SSD
LLM:  llama3.2 (3B) via Ollama
```

### Standard (SME — 10-100 users)

```
CPU:  8 cores
RAM:  32 GB
GPU:  RTX 4090 24GB
Disk: 100 GB NVMe SSD
LLM:  llama3.1 (8B) or Nemotron 3 Nano via NIM
Sandbox: OpenShell Gateway (optional)
```

### Performance (Mid-Market — 100-500 users)

```
CPU:  16 cores
RAM:  64 GB
GPU:  2× RTX 4090 or 1× A100 80GB
Disk: 500 GB NVMe SSD
LLM:  Nemotron 3 Super (120B MoE) via NIM
Guardrails: Content Safety + Jailbreak Detection NIMs
Sandbox: OpenShell Gateway with mTLS
Analytics: Morpheus + Triton Inference Server
```

### Enterprise (500+ users)

```
CPU:  32+ cores
RAM:  256+ GB
GPU:  DGX Spark (128GB) or DGX Station (784GB)
Disk: 1 TB+ NVMe
LLM:  Nemotron Ultra 253B via NIM
Guardrails: Full NeMo stack
Sandbox: OpenShell Gateway cluster
Analytics: Morpheus cluster
```

### Enterprise at Scale

```
NVIDIA DGX BasePOD or SuperPOD
Multi-node NIM deployment with load balancing
NeMo Guardrails cluster
OpenShell Gateway cluster with mTLS
Morpheus for GPU-accelerated security log analysis
Full NVIDIA AI Enterprise license
```

---

## NVIDIA Resource Links

| Resource | URL | Role in AegisClaw |
|----------|-----|-------------------|
| [NVIDIA AI Agents](https://nvidianews.nvidia.com/news/ai-agents) | https://nvidianews.nvidia.com/news/ai-agents | Ecosystem overview for autonomous AI agent development |
| [NVIDIA NIM / API Catalog](https://build.nvidia.com) | https://build.nvidia.com | Optimized inference microservices — primary high-performance LLM backend |
| [NeMoClaw](https://build.nvidia.com/nemoclaw) | https://build.nvidia.com/nemoclaw | Always-on assistant stack powering multi-agent orchestration |
| [Nemotron](https://developer.nvidia.com/nemotron) | https://developer.nvidia.com/nemotron | Nemotron 3 model family (Nano, Super, Ultra, VL, Safety) — hybrid MoE architecture with 1M context |
| [OpenShell](https://docs.nvidia.com/openshell/latest/index.html) | https://docs.nvidia.com/openshell/latest/index.html | Apache 2.0 agent sandboxing runtime — Landlock, seccomp, Privacy Router |
| [NeMo Guardrails](https://developer.nvidia.com/nemo-guardrails) | https://developer.nvidia.com/nemo-guardrails | Prompt safety layer — content safety, jailbreak detection, topic control |
| [Morpheus](https://developer.nvidia.com/morpheus-cybersecurity) | https://developer.nvidia.com/morpheus-cybersecurity | GPU-accelerated security analytics via Triton Inference Server |
| [Triton Inference Server](https://developer.nvidia.com/triton-inference-server) | https://developer.nvidia.com/triton-inference-server | Production model serving for Morpheus security models |
| [DGX Spark](https://www.nvidia.com/en-us/products/workstations/dgx-spark/) | https://www.nvidia.com/en-us/products/workstations/dgx-spark/ | Desktop AI supercomputer ($3,999) for small teams |
| [NVIDIA Inception](https://www.nvidia.com/en-us/startups/) | https://www.nvidia.com/en-us/startups/ | 75% discount on AI Enterprise licenses for startups |
| [NVIDIA AI Enterprise](https://www.nvidia.com/en-us/data-center/products/ai-enterprise/) | https://www.nvidia.com/en-us/data-center/products/ai-enterprise/ | Enterprise support and licensing for NIM, Guardrails, Morpheus |
