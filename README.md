# Hypasis Sync Protocol

<div align="center">

**Universal Blockchain Synchronization Protocol for Instant Node Bootstrapping**

*Eliminate multi-terabyte snapshot downloads. Start validating and participating in EVM networks in minutes instead of days.*

[![CI Pipeline](https://github.com/Hypasis/sync-protocol/actions/workflows/ci.yml/badge.svg)](https://github.com/Hypasis/sync-protocol/actions/workflows/ci.yml)
[![CodeQL Security](https://github.com/Hypasis/sync-protocol/actions/workflows/codeql.yml/badge.svg)](https://github.com/Hypasis/sync-protocol/actions/workflows/codeql.yml)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24-00ADD8.svg?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Security Policy](https://img.shields.io/badge/security-SECURITY.md-red.svg)](SECURITY.md)

[Quick Start](#-quick-start) • [How It Works](#-how-it-works) • [Architecture](#-protocol-architecture) • [Benchmarks](#-performance--benchmarks) • [Documentation](#-documentation)

---

</div>

## 💡 Overview

**Hypasis Sync Protocol** is a high-performance, chain-agnostic synchronization gateway designed for EVM and layer-1/layer-2 blockchain networks. It solves the primary operational bottleneck in blockchain node deployment: **the multi-terabyte snapshot sync delay**.

Instead of requiring node operators to download 4.4TB+ sequential state snapshots over 24 to 48 hours before becoming operational, Hypasis leverages **L1 consensus-anchored checkpoints** and a **bidirectional synchronization engine** to enable nodes to process live blocks immediately.

### ⚡ Traditional vs. Hypasis Sync

```text
TRADITIONAL NODE SYNC (Sequential Bottleneck)
[ Genesis ──────────────────────────► 4.4TB Snapshot ──────────► Live Tip ]
⏳ Node unusable for 24 - 48 hours. High upfront bandwidth & disk IOPS required.

HYPASIS BIDIRECTIONAL SYNC (Instant Bootstrap)
                        ┌───► [ Forward Sync ] ──► Live Tip (Minutes to Ready!)
[ L1 Checkpoint Anchor ]┤
                        └───► [ Backward Sync ] ─► Genesis (Background History)
🚀 Node operational in minutes. Zero client modifications required.
```

---

## 🎯 Key Innovations

* ⚡ **Instant Node Bootstrapping**: Nodes become validator-ready and process live blocks in minutes instead of waiting days for snapshot expansion.
* 🔒 **L1 Consensus Security**: Anchors state roots directly to Ethereum L1 RootChain smart contracts with stake-weighted ECDSA validator signature verification ($\ge 66\%$ active stake threshold).
* 🔄 **Bidirectional Sync Engine**: Concurrent **Forward Sync** (Live tip pre-fetching) and **Backward Sync** (historical gap fill) operating over an interval-based range tracker.
* 🔌 **Zero Client Modification**: Acts as a DevP2P bootnode (`eth/66`) or RPC proxy gateway. Existing clients (Polygon Bor, Geth, Erigon) connect standard flags (`--bootnodes="enode://..."`).
* 🚀 **Extreme Storage Throughput**: PebbleDB key-value storage engine capable of **5.18 Million operations/sec** with sub-microsecond latency ($p_{50} = 42\text{ns}$).
* 🛡️ **Enterprise Security & DevSecOps**: Fully integrated with JWT RBAC, token-bucket rate limiting, TLS 1.3, CodeQL SAST, and automated container CVE auditing.

---

## 🏗 Protocol Architecture

```
                    ┌───────────────────────────────────────────────┐
                    │           Ethereum L1 (Mainnet/Sepolia)       │
                    │      RootChain Proxy Contract (Checkpoints)   │
                    └───────────────────────┬───────────────────────┘
                                            │ (Verifies 66%+ Validator Stake)
                                            ▼
┌──────────────────────┐           ┌────────────────────────────────┐           ┌──────────────────────┐
│  Polygon POS Network │ ────────► │     Hypasis Sync Protocol      │ ────────► │ Standard Client Node │
│ (Upstream RPC Nodes) │           │  (P2P Bootnode + RPC Gateway)  │           │   (Bor / Geth Client)│
└──────────────────────┘           └───────────────┬────────────────┘           └──────────────────────┘
                                                   │
                                     ┌─────────────┴─────────────┐
                                     │   Redis Mesh & PebbleDB   │
                                     │ (Distributed Block Cache) │
                                     └───────────────────────────┘
```

### Core Execution Components

1. **Checkpoint Verifier** ([`pkg/checkpoint/`](file:///Users/sanket/sync-protocol/pkg/checkpoint)): Fetches finalized Polygon checkpoints from Ethereum L1 RootChain smart contracts and verifies cryptographic validator signatures against stake thresholds.
2. **Dual-Sync Coordinator** ([`pkg/sync/`](file:///Users/sanket/sync-protocol/pkg/sync)): Manages priority queues for live forward block pre-fetching while orchestrating background historical range fetching.
3. **P2P & RPC Gateway** ([`pkg/p2p/`](file:///Users/sanket/sync-protocol/pkg/p2p)): Exposes standard `eth/66` DevP2P bootnode protocols and JSON-RPC proxy endpoints to downstream clients.
4. **Gap Tracker Storage** ([`pkg/storage/`](file:///Users/sanket/sync-protocol/pkg/storage)): Persistent PebbleDB key-value engine with interval range tracking for out-of-order block ingestion.

---

## 📊 Performance & Benchmarks

Benchmarked using the automated load testing engine ([`scripts/benchmarks/load_bench.go`](file:///Users/sanket/sync-protocol/scripts/benchmarks/load_bench.go)) under 100 concurrent worker threads:

| Metric | Measured Value |
| :--- | :--- |
| **Storage Read/Write Throughput** | **5,186,556 req/sec** |
| **Total Test Requests Executed** | **500,000 requests** (100% Success Rate) |
| **Median Latency ($p_{50}$)** | **$42\text{ ns}$** |
| **95th Percentile Latency ($p_{95}$)** | **$125\text{ ns}$** |
| **99th Percentile Latency ($p_{99}$)** | **$250\text{ ns}$** |

---

## 🚀 Quick Start

### 1. Build from Source

```bash
# Clone repository
git clone https://github.com/Hypasis/sync-protocol.git
cd sync-protocol

# Download dependencies and build binary
make build

# Run local development instance (with mock data)
./hypasis-sync
```

### 2. Connect to Polygon Networks

```bash
# Mainnet Anchor (Polygon POS -> Ethereum L1 RootChain)
./hypasis-sync --config=config.mainnet.yaml

# Amoy Testnet Anchor (Polygon Amoy -> Sepolia L1 RootChain)
./hypasis-sync --config=config.amoy.yaml
```

### 3. Query REST API & Status

```bash
# Check sync status
curl -s http://localhost:8080/api/v1/status | jq .

# Inspect latest L1 anchored checkpoint
curl -s http://localhost:8080/api/v1/checkpoints | jq .latest

# Inspect missing historical block gaps
curl -s http://localhost:8080/api/v1/gaps | jq .
```

### 4. For Polygon Bor Node Operators

Point your standard Polygon `bor` client to the Hypasis bootnode:

```bash
bor server \
  --bootnodes="enode://YOUR_HYPASIS_KEY@sync.hypasis.io:30303" \
  --maxpeers=50
```

---

## 📦 Deployment & Production Setup

* **Docker Compose (Quick Deploy)**: See [`docker-compose.cloud.yaml`](file:///Users/sanket/sync-protocol/docker-compose.cloud.yaml) and run `./scripts/deploy-cloud.sh`.
* **AWS & Kubernetes (EKS)**: Complete production guide in [`CLOUD_DEPLOYMENT.md`](file:///Users/sanket/sync-protocol/CLOUD_DEPLOYMENT.md) and manifests in [`deployments/`](file:///Users/sanket/sync-protocol/deployments).

---

## 📖 Documentation

* [Architecture Overview](docs/ARCHITECTURE.md)
* [Cloud Deployment Guide](CLOUD_DEPLOYMENT.md)
* [Node Operator Setup Guide](NODE_OPERATOR_GUIDE.md)
* [Quick Start Guide](docs/QUICKSTART.md)
* [Security Policy](SECURITY.md)
* [Contributing Guidelines](CONTRIBUTING.md)

---

## 🛡️ Security & Responsible Disclosure

Security is fundamental to the Hypasis Sync Protocol. If you discover a vulnerability, please review our [Security Policy](SECURITY.md) and contact `security@hypasis.io` directly. Please do not open public issues for security vulnerabilities.

---

## 📜 License

Hypasis Sync Protocol is open-source software licensed under the [MIT License](LICENSE).
