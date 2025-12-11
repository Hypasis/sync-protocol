# Hypasis Sync Protocol - Quick Start Guide

This guide will help you get started with Hypasis Sync Protocol in just a few minutes.

## Prerequisites

- Go 1.21 or higher
- Docker (optional)
- Kubernetes (optional)

## Option 1: Run from Source

### 1. Clone the repository

```bash
git clone https://github.com/hypasis/sync-protocol.git
cd sync-protocol
```

### 2. Download dependencies

```bash
go mod download
```

### 3. Build the binary

```bash
make build
# or
go build -o hypasis-sync ./cmd/hypasis-sync
```

### 4. Run the service

```bash
./hypasis-sync --chain=polygon-mainnet --data-dir=./data
```

### 5. Check the status

```bash
curl http://localhost:8080/api/v1/status
```

## Option 2: Run with Docker

### 1. Build the Docker image

```bash
docker build -t hypasis/sync-protocol:latest .
```

### 2. Run the container

```bash
docker run -d \
  --name hypasis-sync \
  -p 8080:8080 \
  -p 9090:9090 \
  -v ./data:/data/hypasis \
  hypasis/sync-protocol:latest
```

### 3. Check logs

```bash
docker logs -f hypasis-sync
```

### 4. Check status

```bash
curl http://localhost:8080/api/v1/status
```

## Option 3: Run with Docker Compose

### 1. Navigate to deployments

```bash
cd deployments/docker
```

### 2. Start services

```bash
docker-compose up -d
```

### 3. Check status

```bash
docker-compose logs -f hypasis-sync
curl http://localhost:8080/api/v1/status
```

## Option 4: Deploy to Kubernetes

### 1. Apply Kubernetes manifests

```bash
kubectl apply -f deployments/kubernetes/hypasis-sync.yaml
```

### 2. Check pod status

```bash
kubectl get pods -l app=hypasis-sync
```

### 3. Port forward to access API

```bash
kubectl port-forward svc/hypasis-sync 8080:8080
```

### 4. Check status

```bash
curl http://localhost:8080/api/v1/status
```

## Configuration

### Basic Configuration

Create a `config.yaml` file:

```yaml
chain:
  name: polygon-pos
  network: mainnet

sync:
  forward:
    enabled: true
    workers: 8
  backward:
    enabled: true
    workers: 4
    batch_size: 10000

storage:
  data_dir: /data/hypasis
  cache_size: "100GB"
```

Run with config:

```bash
./hypasis-sync --config=config.yaml
```

### Advanced Configuration

See [config.example.yaml](../config.example.yaml) for all available options.

## API Endpoints

### Check Sync Status

```bash
curl http://localhost:8080/api/v1/status
```

Response:
```json
{
  "forward_sync": {
    "current_block": 63000000,
    "target_block": 63000100,
    "progress": 99.8,
    "syncing": true
  },
  "backward_sync": {
    "progress": 75.2,
    "syncing": true
  },
  "validator_ready": true
}
```

### Check Missing Gaps

```bash
curl http://localhost:8080/api/v1/gaps
```

### Pause Backward Sync

```bash
curl -X POST http://localhost:8080/api/v1/sync/pause
```

### Resume Backward Sync

```bash
curl -X POST http://localhost:8080/api/v1/sync/resume
```

## Monitoring

### Prometheus Metrics

Access metrics at:

```
http://localhost:9090/metrics
```

Key metrics:
- `hypasis_forward_sync_blocks_total`
- `hypasis_backward_sync_blocks_total`
- `hypasis_cache_hit_ratio`
- `hypasis_bandwidth_usage_bytes`

### Health Check

```bash
curl http://localhost:8080/health
```

## Integrating with Your Node

### Polygon Bor Example

Configure Bor to use Hypasis:

```bash
bor --syncurl="http://localhost:8080"
```

Or in docker-compose:

```yaml
services:
  bor:
    image: maticnetwork/bor:latest
    environment:
      - SYNC_URL=http://hypasis-sync:8080
```

## Troubleshooting

### Service won't start

Check logs:
```bash
./hypasis-sync 2>&1 | tee hypasis.log
```

### Slow sync

- Increase workers:
  ```yaml
  sync:
    forward:
      workers: 16
    backward:
      workers: 8
  ```

- Check network connectivity:
  ```bash
  curl http://localhost:8080/api/v1/status
  ```

### High memory usage

- Reduce cache size:
  ```yaml
  storage:
    cache_size: "50GB"
  ```

### Gaps not filling

- Check backward sync status:
  ```bash
  curl http://localhost:8080/api/v1/gaps
  ```

- Ensure backward sync is enabled and running

## Next Steps

- Read the [Architecture](ARCHITECTURE.md) documentation
- Configure for your specific chain
- Set up monitoring and alerts
- Join our [Discord community](https://discord.gg/hypasis)

## Support

- Documentation: https://docs.hypasis.io
- Discord: https://discord.gg/hypasis
- GitHub Issues: https://github.com/hypasis/sync-protocol/issues
