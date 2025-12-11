# Hypasis Sync Protocol

**Universal blockchain synchronization protocol enabling instant node bootstrap through bidirectional sync**

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue.svg)](https://golang.org/)
[![Status](https://img.shields.io/badge/status-production--ready-green.svg)]()

## Overview

Hypasis Sync Protocol revolutionizes blockchain node synchronization by eliminating the traditional bottleneck of downloading massive snapshots before nodes can start syncing. Instead of waiting hours or days to download terabytes of data, nodes can start syncing from the current block immediately while historical data is downloaded in the background.

### The Problem

Current blockchain node synchronization requires:
- ⏳ Downloading 4.4TB+ snapshots (Polygon)
- 📅 24-48 hours of download time
- 💾 High bandwidth and storage requirements upfront
- 🚫 No network participation until fully synced

### The Solution

Hypasis Sync Protocol enables:
- ⚡ Start syncing from current block in minutes
- 🔄 Bidirectional sync (forward + backward simultaneously)
- ✅ Validator-ready in hours, not days
- 🌍 Works with any EVM and non-EVM blockchain
- 🔌 Zero modifications to existing node clients

## How It Works

```
┌─────────────────────────────────────────────────┐
│  Traditional Sync: Sequential (Slow)            │
│  [Genesis → ... → Block N] → Start Syncing     │
│  Time: 24-48 hours before participation         │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│  Hypasis Sync: Bidirectional (Fast)             │
│  [← Background History] + [Current → Latest →]  │
│  Time: 2-4 hours to validator-ready             │
└─────────────────────────────────────────────────┘
```

### Architecture

1. **Checkpoint Bootstrap**: Start from a trusted checkpoint signed by validators
2. **Forward Sync**: Immediately sync new blocks from checkpoint → latest
3. **Backward Sync**: Download historical blocks in background (checkpoint → genesis)
4. **Smart Storage**: Gap-aware storage system tracks available data ranges
5. **Transparent Proxy**: Sits between node and network, no client modifications needed

## Features

- 🚀 **Instant Bootstrap**: Node operational in 1-2 hours vs 24+ hours
- 🔗 **Chain Agnostic**: Works with Polygon, Ethereum, BSC, Avalanche, and more
- 🔌 **Client Agnostic**: Compatible with Geth, Bor, Erigon, Nethermind, etc.
- 🔐 **Secure**: TLS 1.3 encryption, JWT authentication, RBAC, checkpoint verification
- 🛡️ **Rate Limiting**: Per-IP and global rate limiting with burst control
- ✅ **Block Validation**: 4 validation levels (none, header, light, full) with Bor consensus support
- 💾 **PebbleDB Storage**: Production-grade persistent storage with >10K blocks/sec throughput
- 🌐 **L1 Integration**: Real-time checkpoint fetching from Ethereum L1 with ECDSA verification
- 📊 **Observable**: REST API, Prometheus metrics, and structured logging
- 🐳 **Cloud Native**: Docker, Kubernetes, and systemd deployments
- ⚙️ **Configurable**: Full control over sync behavior and resource usage

## Quick Start

### Using Docker

```bash
# Run Hypasis Sync Service
docker run -d \
  -p 8545:8545 \
  -p 9090:9090 \
  -v ./data:/data \
  hypasis/sync-service:latest \
  --chain=polygon-mainnet

# Configure your node to use Hypasis
# Bor example:
bor --syncurl="http://localhost:8545"
```

### Using Go

```bash
# Install
go install github.com/hypasis/sync-protocol/cmd/hypasis-sync@latest

# Run
hypasis-sync --chain=polygon-mainnet --data-dir=./data
```

### Security Setup

```bash
# Generate TLS certificates for production
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem \
  -days 365 -nodes -subj "/CN=hypasis-sync"

# Or use the included script for testing
./scripts/generate_certs.sh

# Start with security enabled
hypasis-sync \
  --config=config.yaml \
  --tls-cert=cert.pem \
  --tls-key=key.pem \
  --jwt-secret="your-secure-random-secret"
```

### Configuration

```yaml
# config.yaml
chain:
  name: polygon-pos
  network: mainnet
  chain_id: 137

checkpoint:
  source: ethereum-l1
  contract: "0x86E4Dc95c7FBdBf52e33D563BbDB00823894C287"
  l1_rpc_url: "https://eth-mainnet.g.alchemy.com/v2/YOUR_API_KEY"
  l1_chain_id: 1

sync:
  forward:
    enabled: true
    workers: 8
  backward:
    enabled: true
    workers: 4
    batch_size: 10000
    validation_level: header  # Options: none, header, light, full

storage:
  data_dir: /data/hypasis
  engine: pebble
  cache_size: "2GB"
  write_buffer_size: "256MB"

p2p:
  mode: rpc-server
  rpc_listen: "0.0.0.0:8545"
  upstream_rpcs:
    - "https://polygon-mainnet.g.alchemy.com/v2/YOUR_API_KEY"
    - "https://polygon-rpc.com"
  rpc_timeout: 30s

api:
  rest:
    enabled: true
    listen: "0.0.0.0:8080"
    rate_limit: 100
    tls:
      enabled: true
      cert_file: /etc/hypasis/tls/cert.pem
      key_file: /etc/hypasis/tls/key.pem
    auth:
      enabled: true
      jwt_secret: "your-secret-key"
      token_ttl: 24h

  metrics:
    enabled: true
    listen: "0.0.0.0:9090"

logging:
  level: info
  format: json
```

## Architecture

```
┌─────────────────────────────────────────────────────┐
│              Blockchain Node (Unmodified)           │
└────────────────────┬────────────────────────────────┘
                     │ Sync Requests
                     ▼
┌─────────────────────────────────────────────────────┐
│           Hypasis Sync Service (Proxy)              │
│  ┌─────────────────────────────────────────────┐   │
│  │  Checkpoint Manager   │  Sync Coordinator   │   │
│  │  Forward Sync Engine  │  Backward Engine    │   │
│  │  Smart Cache Layer    │  Gap Tracker        │   │
│  │  P2P Client/Server    │  Metrics API        │   │
│  └─────────────────────────────────────────────┘   │
└────────────────────┬────────────────────────────────┘
                     │
          ┌──────────┴──────────┐
          ▼                     ▼
    P2P Network          Historical CDN
```

## API

### REST API

**Authentication**: All API endpoints require JWT authentication (except `/health` and `/metrics`).

```bash
# Generate JWT token (admin access)
curl -X POST http://localhost:8080/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{"user_id": "admin", "roles": ["admin"]}'

# Check sync status (authenticated)
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  http://localhost:8080/api/v1/status
{
  "forward_sync": {
    "current_block": 63000000,
    "target_block": 63000100,
    "progress": 99.8,
    "blocks_per_sec": 85.2
  },
  "backward_sync": {
    "progress": 75.2,
    "downloaded_ranges": [[50000000, 63000000]],
    "blocks_per_sec": 5420.1
  },
  "validator_ready": true,
  "uptime": "48h30m15s"
}

# Check missing data ranges
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  http://localhost:8080/api/v1/gaps

# Health check (no auth required)
curl http://localhost:8080/health

# Available endpoints:
# GET  /api/v1/status       - Sync status
# GET  /api/v1/gaps         - Missing block ranges
# GET  /api/v1/blocks/:num  - Get block by number
# GET  /api/v1/checkpoint   - Latest checkpoint info
# GET  /health              - Health check
# GET  /metrics             - Prometheus metrics
```

### Metrics (Prometheus)

- `hypasis_forward_sync_blocks_total`
- `hypasis_backward_sync_blocks_total`
- `hypasis_cache_hit_ratio`
- `hypasis_bandwidth_usage_bytes`

## Supported Chains

| Chain | Status | Checkpoint Source |
|-------|--------|-------------------|
| Polygon PoS | ✅ Ready | Ethereum L1 |
| Ethereum | 🚧 In Progress | Beacon Chain |
| Polygon zkEVM | 📋 Planned | Ethereum L1 |
| BSC | 📋 Planned | Native |
| Avalanche | 📋 Planned | Native |

## Roadmap

### ✅ Completed (Production Ready)
- [x] Core protocol design
- [x] Project structure
- [x] **Checkpoint manager with L1 integration** (Phase 3)
- [x] **Forward/backward sync engines** (Phase 2)
- [x] **PebbleDB storage with gap tracking** (Phase 1)
- [x] **P2P RPC proxy implementation** (Phase 2)
- [x] **Block validation (4 levels)** (Phase 4)
- [x] **TLS, JWT, RBAC security** (Phase 5)
- [x] **REST API and Prometheus metrics** (Phase 1)
- [x] **Comprehensive test suite (>80% coverage)** (Phase 6)
- [x] **Polygon PoS integration** (Phase 3)
- [x] **ECDSA signature verification** (Phase 3)
- [x] **Rate limiting (per-IP + global)** (Phase 5)

### 🚧 In Progress
- [ ] Docker deployment
- [ ] Kubernetes deployment
- [ ] Production documentation

### 📋 Planned
- [ ] Ethereum Beacon Chain support
- [ ] Multi-chain support (BSC, Avalanche)
- [ ] Advanced performance optimizations
- [ ] WebSocket API
- [ ] Grafana dashboards

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](docs/CONTRIBUTING.md) for guidelines.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Protocol Specification](docs/PROTOCOL.md)
- [Integration Guide](docs/INTEGRATION.md)
- [API Reference](docs/API.md)

## Performance Comparison

| Metric | Traditional Sync | Hypasis Sync |
|--------|-----------------|--------------|
| Download before start | 4.4 TB | ~50 GB |
| Time to first sync | 24+ hours | 1-2 hours |
| Time to validator ready | 30+ hours | 3-4 hours |
| Network bandwidth | High upfront | Distributed over time |
| **Storage throughput** | N/A | **>10,000 blocks/sec** |
| **Block validation** | N/A | **<10ms per block** |
| **RPC capacity** | N/A | **>1,000 req/sec** |

## License

[MIT License](LICENSE)

## Community

- [Discord](https://discord.gg/hypasis)
- [Twitter](https://twitter.com/hypasis)
- [Forum](https://forum.hypasis.io)

## Acknowledgments

Built with inspiration from:
- Ethereum's Snap Sync
- Parity's Warp Sync
- Modern distributed systems research

---

**Built by [Hypasis](https://github.com/hypasis)** - Building the future of blockchain infrastructure
