# NVIDIA GPU Deployment Guide

This guide covers deploying AegisClaw with NVIDIA GPU acceleration, from a single consumer GPU for SMB/SME to multi-GPU enterprise configurations.

---

## Table of Contents

1. [Deployment Profiles](#1-deployment-profiles)
2. [Consumer GPU Setup (SMB/SME)](#2-consumer-gpu-setup-smbsme)
3. [NVIDIA NIM Setup](#3-nvidia-nim-setup)
4. [NeMo Guardrails](#4-nemo-guardrails)
5. [Model Selection Guide](#5-model-selection-guide)
6. [Cost Optimization](#6-cost-optimization)
7. [Hardware Sizing](#7-hardware-sizing)

---

## 1. Deployment Profiles

AegisClaw supports three LLM deployment modes. Choose based on your budget and hardware.

| Profile | Backend | Hardware Needed | Monthly Cost | Best For |
|---------|---------|-----------------|--------------|----------|
| **Free / CPU** | Ollama (CPU-only) | Any x86 server, 16GB+ RAM | $0 (electricity) | Evaluation, small orgs |
| **Consumer GPU** | Ollama + GPU | RTX 3060-5090 | ~$30-65 electricity | SMB/SME (1-50 employees) |
| **NIM Cloud** | NVIDIA API Catalog | None (API only) | ~$30-90 API fees | Teams without GPU hardware |
| **NIM Self-Hosted** | NIM container + GPU | RTX 4090+ or DGX | ~$50-100 electricity | Mid-market, data sovereignty |
| **Enterprise** | NIM + Guardrails + DGX | DGX Spark/Station/BasePOD | Varies | Enterprise (500+ employees) |

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

NIM provides optimized inference containers for Nemotron and other models. Two options: cloud API or self-hosted.

### Option A: NVIDIA API Catalog (Zero Hardware)

The fastest way to use Nemotron models. Pay per token, no GPU required.

```bash
# 1. Get an API key at https://build.nvidia.com
# 2. Add to .env:
AEGISCLAW_NVIDIA_NIM_ENABLED=true
AEGISCLAW_NVIDIA_NIM_URL=https://integrate.api.nvidia.com/v1
AEGISCLAW_NVIDIA_NIM_API_KEY=nvapi-xxxxxxxxxxxx
AEGISCLAW_NVIDIA_NIM_DEFAULT_MODEL=nvidia/nemotron-super-49b-v1

# 3. Deploy
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.nvidia.yml up -d
```

### Option B: Self-Hosted NIM (Data Sovereignty)

Run NIM containers on your own hardware. All data stays on-premise.

```bash
# 1. Get NGC API key at https://ngc.nvidia.com
export NGC_API_KEY=<your-ngc-key>

# 2. Login to NVIDIA container registry
docker login nvcr.io -u '$oauthtoken' -p $NGC_API_KEY

# 3. Choose model based on your GPU:

# RTX 4090/5090 (24-32GB VRAM) — Nemotron Nano
docker run -d --gpus all \
  -e NGC_API_KEY=$NGC_API_KEY \
  -v nim-cache:/opt/nim/.cache \
  -p 8000:8000 \
  nvcr.io/nim/nvidia/nemotron-nano-8b-v1:latest

# 2x RTX 4090 or A100 (48-80GB VRAM) — Nemotron Super
docker run -d --gpus all \
  -e NGC_API_KEY=$NGC_API_KEY \
  -v nim-cache:/opt/nim/.cache \
  -p 8000:8000 \
  nvcr.io/nim/nvidia/nemotron-super-49b-v1:latest

# DGX Spark/Station (128-784GB) — Nemotron Ultra
docker run -d --gpus all \
  -e NGC_API_KEY=$NGC_API_KEY \
  -v nim-cache:/opt/nim/.cache \
  -p 8000:8000 \
  nvcr.io/nim/nvidia/nemotron-ultra-253b-v1:latest

# 4. Point AegisClaw at local NIM:
AEGISCLAW_NVIDIA_NIM_ENABLED=true
AEGISCLAW_NVIDIA_NIM_URL=http://localhost:8000/v1
AEGISCLAW_NVIDIA_NIM_DEFAULT_MODEL=nvidia/nemotron-nano-8b-v1
```

### NIM Health Check

```bash
# Verify NIM is running
curl http://localhost:8000/v1/health/ready

# List available models
curl http://localhost:8000/v1/models

# Test inference
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"nvidia/nemotron-nano-8b-v1","messages":[{"role":"user","content":"Hello"}]}'
```

---

## 4. NeMo Guardrails

NeMo Guardrails NIMs add safety layers to all LLM prompts: content safety, jailbreak detection, and topic control.

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

## 5. Model Selection Guide

### Nemotron Models (via NIM)

| Model | Parameters | VRAM Required | Strengths | Use Case |
|-------|-----------|---------------|-----------|----------|
| **Nemotron Nano** | 8B (3B active) | 8-16GB | Fast, efficient, hybrid Mamba-Transformer | SMB: finding summaries, playbook analysis |
| **Nemotron Super** | 49B (12B active) | 48-80GB | Best quality/cost, multi-agent optimized | Mid-market: full agent reasoning pipeline |
| **Nemotron Ultra** | 253B | 500GB+ | Maximum reasoning, complex analysis | Enterprise: threat modeling, compliance reports |

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

> **SMB Recommendation**: Start with `llama3.1` (8B) on Ollama. It's free, runs on an RTX 3060+, and provides solid reasoning for security validation. Upgrade to Nemotron Nano via NIM when budget allows.

---

## 6. Cost Optimization

### Self-Hosted vs. Cloud (Break-Even Analysis)

| Scenario | Monthly Cost | Notes |
|----------|-------------|-------|
| **Ollama on CPU** | $0 | Slow (~5 tok/s), but functional |
| **Ollama on RTX 4090** | ~$50/mo electricity | ~120 tok/s, one-time $1,600 GPU cost |
| **Ollama on RTX 3060** | ~$20/mo electricity | ~80 tok/s for 3B model, one-time $200-300 GPU cost |
| **NIM Cloud API (light)** | ~$30/mo | 50K tokens/day, no hardware |
| **NIM Cloud API (heavy)** | ~$90/mo | 200K tokens/day, no hardware |
| **Self-hosted NIM + RTX 4090** | ~$50/mo electricity | Unlimited tokens, one-time $1,600 GPU |

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
3. Run `llama3.1` (8B) or quantized 70B via Ollama, or Nemotron Nano via NIM
4. Monthly electricity: ~$50

### Enterprise (NVIDIA Inception Program)

NVIDIA's Inception program offers **75% discount** on NVIDIA AI Enterprise licenses ($4,500/GPU/year → ~$1,125/GPU/year). Includes enterprise support for NIM, NeMo Guardrails, and Morpheus.

Apply at: https://www.nvidia.com/en-us/startups/

---

## 7. Hardware Sizing

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
LLM:  llama3.1 (8B) or nemotron-nano-8b via NIM
```

### Performance (Mid-Market — 100-500 users)

```
CPU:  16 cores
RAM:  64 GB
GPU:  2× RTX 4090 or 1× A100 80GB
Disk: 500 GB NVMe SSD
LLM:  nemotron-super-49b via NIM
Guardrails: Content Safety + Jailbreak Detection NIMs
```

### Enterprise (500+ users)

```
CPU:  32+ cores
RAM:  256+ GB
GPU:  DGX Spark (128GB) or DGX Station (784GB)
Disk: 1 TB+ NVMe
LLM:  nemotron-ultra-253b via NIM
Guardrails: Full NeMo stack
```

### Enterprise at Scale

```
NVIDIA DGX BasePOD or SuperPOD
Multi-node NIM deployment with load balancing
NeMo Guardrails cluster
Morpheus for GPU-accelerated security log analysis
Full NVIDIA AI Enterprise license
```
