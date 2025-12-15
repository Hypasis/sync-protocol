# Hypasis Sync Protocol - Cloud Deployment Guide

Complete guide for deploying Hypasis Sync Protocol in production to support 1000+ Polygon node operators.

---

## 🎯 Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Load Balancer / DNS                      │
│              sync.hypasis.io (API endpoint)                  │
│     sync1/2/3.hypasis.io (individual bootnodes)             │
└────────────────────────┬────────────────────────────────────┘
                         │
        ┌────────────────┼────────────────┐
        │                │                │
   ┌────▼───┐      ┌────▼───┐      ┌────▼───┐
   │Hypasis1│      │Hypasis2│      │Hypasis3│
   │US-East │      │US-West │      │EU-Cent │
   └────┬───┘      └────┬───┘      └────┬───┘
        │                │                │
        └────────────────┼────────────────┘
                         │
                    ┌────▼────┐
                    │  Redis  │
                    │ Cluster │
                    └─────────┘
```

Each instance handles:
- **500 Bor node operators** (P2P connections)
- **~10,000 RPC requests/sec**
- **Bidirectional sync** (forward + backward)
- **Distributed caching** via Redis
- **Health monitoring** & auto-failover

---

## 📋 Prerequisites

### Infrastructure Requirements

**Per Instance (Minimum):**
- **CPU**: 16 cores
- **RAM**: 32 GB
- **Storage**: 2 TB NVMe SSD
- **Network**: 1 Gbps bandwidth
- **OS**: Ubuntu 22.04 LTS or Amazon Linux 2

**Redis Cluster:**
- **CPU**: 4 cores
- **RAM**: 16 GB
- **Storage**: 50 GB SSD
- **High availability**: Master + 2 replicas

### Required Services

1. **Ethereum L1 RPC** - Alchemy/Infura (for checkpoints)
2. **Polygon RPC** - Alchemy/Infura (for block fetching)
3. **Domain Names**:
   - `sync.hypasis.io` - Main API endpoint
   - `sync1.hypasis.io` - Instance 1 bootnode
   - `sync2.hypasis.io` - Instance 2 bootnode
   - `sync3.hypasis.io` - Instance 3 bootnode
4. **SSL Certificates** - Let's Encrypt or commercial CA

---

## 🚀 Deployment Options

### Option 1: Docker (Recommended for Testing)

```bash
# 1. Clone repository
git clone https://github.com/hypasis/sync-protocol.git
cd sync-protocol

# 2. Set environment variables
cp .env.example .env
# Edit .env with your API keys

# 3. Start all services
docker-compose -f docker-compose.cloud.yaml up -d

# 4. Check status
docker-compose ps
docker logs hypasis-sync-1

# 5. View enode URLs (for node operators)
docker exec hypasis-sync-1 cat /data/hypasis/enode.txt
```

### Option 2: Kubernetes (Recommended for Production)

```bash
# 1. Create namespace
kubectl create namespace hypasis

# 2. Create secrets
kubectl create secret generic hypasis-secrets \
  --from-literal=eth-l1-rpc-url="YOUR_ETHEREUM_RPC" \
  --from-literal=polygon-rpc-url="YOUR_POLYGON_RPC" \
  --from-literal=redis-password="STRONG_PASSWORD" \
  --from-literal=jwt-secret="RANDOM_SECRET" \
  -n hypasis

# 3. Deploy Redis
kubectl apply -f deployments/kubernetes/redis.yaml -n hypasis

# 4. Deploy Hypasis instances
kubectl apply -f deployments/kubernetes/hypasis-sync.yaml -n hypasis

# 5. Deploy load balancer
kubectl apply -f deployments/kubernetes/ingress.yaml -n hypasis

# 6. Check status
kubectl get pods -n hypasis
kubectl logs -f hypasis-sync-1-0 -n hypasis
```

### Option 3: AWS (Terraform)

```bash
# 1. Initialize Terraform
cd deployments/terraform/aws
terraform init

# 2. Configure variables
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars

# 3. Plan deployment
terraform plan

# 4. Deploy
terraform apply

# 5. Get outputs
terraform output enode_urls
terraform output api_endpoint
```

---

## ⚙️ Configuration

### Environment Variables

Create a `.env` file with these variables:

```bash
# Instance Configuration
INSTANCE_ID=hypasis-sync-1          # Unique ID (1, 2, 3)
REGION=us-east-1                    # AWS region or datacenter
EXTERNAL_IP=auto                    # Public IP (auto-detect)

# Ethereum L1 (for Polygon checkpoints)
ETH_L1_RPC_URL=https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY

# Polygon RPC (for block fetching)
POLYGON_RPC_1=https://polygon-mainnet.g.alchemy.com/v2/YOUR_KEY
POLYGON_RPC_2=https://polygon-rpc.com

# Redis (cluster coordination)
REDIS_URL=redis://cluster.hypasis.internal:6379
REDIS_PASSWORD=STRONG_RANDOM_PASSWORD

# Security
JWT_SECRET=RANDOM_SECRET_KEY

# Performance Tuning
MAX_PEERS=500                       # Max Bor nodes per instance
PER_OPERATOR_RPS=100                # Rate limit per operator
GLOBAL_RPS=50000                    # Global rate limit
MAX_CONNECTIONS=500                 # Connection pool size

# Feature Flags
ENABLE_CACHE=true
ENABLE_CLUSTER=true
ENABLE_DEVP2P=true
```

### config.cloud.yaml

The cloud configuration file is pre-configured for production. Key sections:

```yaml
# Cloud instance settings
cloud:
  instance_id: "hypasis-sync-1"     # Set via ENV
  region: "us-east-1"                # Set via ENV
  external_ip: "auto"

# Cluster mesh configuration
cluster:
  enabled: true
  redis_url: "${REDIS_URL}"
  heartbeat_period: 10s

# P2P DevP2P bootnode mode
p2p:
  mode: devp2p
  listen: "0.0.0.0:30303"
  max_peers: 500

# Rate limiting
ratelimit:
  enabled: true
  per_operator_rps: 100
  global_rps: 50000
```

---

## 📊 Monitoring & Observability

### Health Checks

**API Health Endpoint:**
```bash
curl https://sync.hypasis.io/health
```

**Response:**
```json
{
  "status": "healthy",
  "checks": {
    "storage": {"healthy": true, "message": "OK"},
    "cache": {"healthy": true, "message": "OK"},
    "sync": {"healthy": true, "message": "OK"},
    "p2p": {"healthy": true, "message": "412 peers connected"}
  },
  "uptime_hours": 720
}
```

### Prometheus Metrics

**Available at:** `http://sync.hypasis.io:9090/metrics`

Key metrics:
- `hypasis_forward_sync_blocks_total` - Forward sync progress
- `hypasis_backward_sync_blocks_total` - Backward sync progress
- `hypasis_connected_peers_total` - Number of Bor nodes connected
- `hypasis_rpc_requests_total` - RPC request count
- `hypasis_cache_hit_ratio` - Cache effectiveness
- `hypasis_rate_limit_rejected_total` - Rate-limited requests

### Grafana Dashboards

Import pre-built dashboards from `deployments/monitoring/grafana/dashboards/`

---

## 🔐 Security Best Practices

### 1. Firewall Rules

```bash
# Allow only necessary ports
ufw allow 8080/tcp   # REST API (HTTPS via load balancer)
ufw allow 8545/tcp   # RPC (internal or via load balancer)
ufw allow 30303/tcp  # DevP2P
ufw allow 30303/udp  # DevP2P discovery
ufw deny 9090/tcp    # Metrics (internal only)
```

### 2. SSL/TLS

Use Let's Encrypt for SSL certificates:

```bash
# Install certbot
apt-get install certbot

# Generate certificates
certbot certonly --standalone -d sync.hypasis.io \
  -d sync1.hypasis.io -d sync2.hypasis.io -d sync3.hypasis.io

# Configure auto-renewal
crontab -e
0 0 * * * certbot renew --quiet
```

### 3. JWT Authentication

Generate secure JWT secret:

```bash
openssl rand -base64 32
```

Add to `.env`:
```bash
JWT_SECRET=<generated-secret>
```

### 4. Redis Security

```bash
# Set strong password
redis-cli CONFIG SET requirepass "YOUR_STRONG_PASSWORD"

# Disable dangerous commands
redis-cli CONFIG SET rename-command FLUSHALL ""
redis-cli CONFIG SET rename-command FLUSHDB ""
```

---

## 📈 Scaling Guide

### Horizontal Scaling

**Adding a 4th Instance:**

1. Deploy new instance with `INSTANCE_ID=hypasis-sync-4`
2. Update load balancer to include new instance
3. Instance automatically joins cluster via Redis
4. No downtime required

**Capacity Planning:**
- Each instance: ~500 operators
- 3 instances: ~1,500 operators
- 4 instances: ~2,000 operators
- 5 instances: ~2,500 operators

### Vertical Scaling

**Increasing per-instance capacity:**

```yaml
# Update config.cloud.yaml
p2p:
  max_peers: 1000  # Increase from 500

connection_pool:
  max_connections: 1000

ratelimit:
  global_rps: 100000  # Increase from 50000
```

**Resource requirements scale linearly:**
- 1000 peers ≈ 32 GB RAM → 64 GB RAM
- 1000 peers ≈ 16 cores → 32 cores

---

## 🌐 DNS Configuration

### A Records (for direct bootnode access)

```
sync1.hypasis.io    A    54.123.45.1
sync2.hypasis.io    A    54.123.45.2
sync3.hypasis.io    A    54.123.45.3
```

### Round-Robin DNS (simple load balancing)

```
sync.hypasis.io     A    54.123.45.1
sync.hypasis.io     A    54.123.45.2
sync.hypasis.io     A    54.123.45.3
```

### CNAME (with load balancer)

```
sync.hypasis.io     CNAME   lb.hypasis.io
api.hypasis.io      CNAME   lb.hypasis.io
```

---

## 👥 For Node Operators

### How to Use Hypasis Sync

**Step 1: Get Bootnode URLs**

Visit: `https://sync.hypasis.io/api/v1/bootnodes`

```json
{
  "bootnodes": [
    "enode://abc123...@sync1.hypasis.io:30303",
    "enode://def456...@sync2.hypasis.io:30303",
    "enode://ghi789...@sync3.hypasis.io:30303"
  ],
  "recommended": "Use all 3 for best redundancy"
}
```

**Step 2: Configure Bor**

Add to your Bor startup command:

```bash
bor server \
  --datadir=/data/bor \
  --bootnodes="enode://abc123...@sync1.hypasis.io:30303,enode://def456...@sync2.hypasis.io:30303,enode://ghi789...@sync3.hypasis.io:30303" \
  --maxpeers=50 \
  --heimdall="http://localhost:1317" \
  # ... your other flags
```

Or in `config.toml`:

```toml
[p2p]
maxpeers = 50
bootnodes = [
  "enode://abc123...@sync1.hypasis.io:30303",
  "enode://def456...@sync2.hypasis.io:30303",
  "enode://ghi789...@sync3.hypasis.io:30303"
]
```

**Step 3: Start Bor**

```bash
bor server --config /path/to/config.toml
```

**That's it!** Your node will now sync 10x faster:
- ✅ Validator-ready in 3-4 hours (vs 24+ hours)
- ✅ No snapshot downloads needed
- ✅ Bidirectional sync (forward + backward)

---

## 🛠️ Troubleshooting

### Issue: High Memory Usage

**Solution:**
```yaml
# Reduce cache size in config.cloud.yaml
storage:
  cache_size: "2GB"  # Reduce from 4GB

cache:
  max_size: "5GB"    # Reduce from 10GB
```

### Issue: Peers Not Connecting

**Check:**
1. Firewall allows port 30303 (TCP + UDP)
2. External IP is correct: `curl https://ifconfig.me`
3. Enode URL is accessible: Test with another Bor node

**Debug:**
```bash
# Check DevP2P logs
docker logs hypasis-sync-1 | grep "DevP2P"

# Check peer count
curl https://api.hypasis.io/api/v1/status
```

### Issue: Redis Connection Failures

**Check:**
1. Redis is running: `redis-cli ping`
2. Password is correct in `.env`
3. Network connectivity: `telnet redis-host 6379`

**Restart Redis:**
```bash
docker-compose restart redis
# or
kubectl rollout restart statefulset redis -n hypasis
```

### Issue: Rate Limiting Too Aggressive

**Temporarily disable for specific operator:**
```bash
# Get operator ID (usually IP or node ID)
OPERATOR_ID="10.20.30.40"

# Increase limit for specific operator
curl -X POST https://api.hypasis.io/api/v1/ratelimit/operator/$OPERATOR_ID \
  -H "Authorization: Bearer YOUR_JWT" \
  -d '{"rps": 1000}'
```

---

## 📞 Support & Monitoring

### Status Page

Public status: https://status.hypasis.io

Shows:
- ✅ Service health
- 👥 Connected operators
- 📊 Sync progress
- 🌍 Regional availability

### Admin Dashboard

Internal dashboard: https://dashboard.hypasis.internal

Access requires:
- VPN connection
- JWT authentication
- Admin role

### Alerts

Configure alerts in Prometheus:

```yaml
groups:
  - name: hypasis
    rules:
      - alert: HighErrorRate
        expr: rate(hypasis_errors_total[5m]) > 10
        for: 5m
        annotations:
          summary: "High error rate detected"

      - alert: LowPeerCount
        expr: hypasis_connected_peers_total < 100
        for: 10m
        annotations:
          summary: "Peer count below threshold"
```

---

## 🎓 Advanced Topics

### Custom Validation Levels

```yaml
sync:
  forward:
    validation_level: full  # Options: none, header, light, full

  backward:
    validation_level: header  # Lighter for background sync
```

### Custom Rate Limits per Operator

```bash
# Set custom limit for premium operator
curl -X POST https://api.hypasis.io/api/v1/operators/premium-123/limits \
  -H "Authorization: Bearer $JWT" \
  -d '{
    "rps": 500,
    "burst": 1000
  }'
```

### Disaster Recovery

**Backup critical data:**
```bash
# Backup Redis
redis-cli --rdb /backup/dump.rdb

# Backup node keys
cp /data/hypasis/nodekey /backup/
```

**Restore procedure:**
1. Deploy new instance
2. Restore node key (keeps enode URL same)
3. Redis cluster resyncs automatically
4. Update DNS if IP changed

---

## 📄 License

MIT License - See LICENSE file

---

## 🤝 Contributing

Contributions welcome! See CONTRIBUTING.md

---

**Built by Hypasis Team** - Building the future of blockchain infrastructure 🚀
