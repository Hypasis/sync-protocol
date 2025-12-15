# Hypasis Sync - Quick Start for Polygon Node Operators

## 🎯 What is Hypasis Sync?

Hypasis Sync is a **free public service** that allows Polygon validators to sync their nodes **10x faster**:

- ⚡ **Validator-ready in 3-4 hours** (instead of 24+ hours)
- 📦 **No snapshot downloads** (saves 4.4 TB download)
- 🔄 **Bidirectional sync** (forward + backward simultaneously)
- 🔌 **Zero code changes** to your Bor node

---

## 🚀 Quick Start (30 seconds)

### Step 1: Copy These Bootnode URLs

```text
enode://PUBKEY1@sync1.hypasis.io:30303
enode://PUBKEY2@sync2.hypasis.io:30303
enode://PUBKEY3@sync3.hypasis.io:30303
```

> **Note:** Get the latest bootnodes at: https://sync.hypasis.io/bootnodes

### Step 2: Add to Your Bor Configuration

**Option A: Command Line**

```bash
bor server \
  --datadir=/data/bor \
  --bootnodes="enode://PUBKEY1@sync1.hypasis.io:30303,enode://PUBKEY2@sync2.hypasis.io:30303,enode://PUBKEY3@sync3.hypasis.io:30303" \
  --maxpeers=50 \
  --heimdall="http://localhost:1317"
  # ... your other flags
```

**Option B: Config File** (`config.toml`)

```toml
[p2p]
maxpeers = 50
bootnodes = [
  "enode://PUBKEY1@sync1.hypasis.io:30303",
  "enode://PUBKEY2@sync2.hypasis.io:30303",
  "enode://PUBKEY3@sync3.hypasis.io:30303"
]
```

### Step 3: Start Bor

```bash
# With command line flags
bor server --config /path/to/config.toml

# Or if using systemd
systemctl restart bor
```

**That's it!** Your node will now sync significantly faster.

---

## ✅ Verify It's Working

### Check Bor Logs

```bash
# Check if connected to Hypasis bootnodes
tail -f /var/log/bor/bor.log | grep "sync.hypasis.io"
```

You should see:
```
INFO [12-15|10:30:15] Peer connected name=Hypasis... addr=sync1.hypasis.io:30303
```

### Check Sync Status

```bash
# Query Bor sync status
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"eth_syncing","params":[],"id":1}'
```

Expected response while syncing:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "startingBlock": "0x0",
    "currentBlock": "0x3c0f000",
    "highestBlock": "0x3c0f100"
  }
}
```

When fully synced:
```json
{
  "jsonrpc": "2.0",
  "result": false
}
```

---

## 📊 Performance Comparison

| Method | Time to Validator-Ready | Download Size | Network Usage |
|--------|------------------------|---------------|---------------|
| **Traditional Snapshot** | 24-48 hours | 4.4 TB | Very High (upfront) |
| **Hypasis Sync** | **3-4 hours** | **~50 GB** | **Distributed over time** |

---

## 🔧 Configuration Recommendations

### Recommended Bor Settings

```toml
# config.toml

[p2p]
maxpeers = 50  # Good balance for most nodes
maxpendpeers = 25

bootnodes = [
  "enode://PUBKEY1@sync1.hypasis.io:30303",
  "enode://PUBKEY2@sync2.hypasis.io:30303",
  "enode://PUBKEY3@sync3.hypasis.io:30303"
]

[eth]
syncmode = "full"
snapshot = true

[miner]
gaslimit = 20000000
gasprice = "30000000000"
```

### Heimdall Configuration

**No changes needed!** Heimdall continues to use your local Bor instance:

```toml
# heimdall config (app.toml)
bor_rpc_url = "http://localhost:8545"  # No change needed
```

### Ports Required

Make sure these ports are accessible:
- **30303** (TCP + UDP) - Bor P2P
- **8545** (TCP) - Bor RPC (local only)

---

## 🌍 Architecture: How It Works

```
Your Bor Node
     ↓ (connects to bootnodes)
Hypasis Sync Network (3 instances)
     ↓ (fetches blocks)
Polygon Network + Ethereum L1 (checkpoints)
```

### What Hypasis Does:

1. **Fetches checkpoints** from Ethereum L1 (validator-signed)
2. **Syncs forward** from latest checkpoint → current block
3. **Fills gaps backward** (checkpoint → genesis) in background
4. **Serves blocks** to your Bor node via P2P
5. **No trust required** - your node validates everything

---

## ❓ FAQ

### Is Hypasis Sync safe?

**Yes!** Hypasis doesn't modify your node software. Your Bor node still:
- ✅ Validates all blocks cryptographically
- ✅ Verifies all transactions
- ✅ Checks all consensus rules
- ✅ Connects to Heimdall normally

Hypasis just helps you **fetch blocks faster**.

### Does this cost money?

**No!** Hypasis Sync is completely **free** for all Polygon node operators.

### Can I remove Hypasis bootnodes later?

**Yes!** Once fully synced, you can remove the bootnode URLs and your node will continue syncing normally from the regular Polygon P2P network.

### What if Hypasis goes down?

Your node automatically falls back to regular Polygon P2P peers. No downtime or data loss.

### Do I need to trust Hypasis?

**No!** Your Bor node cryptographically validates every block. Hypasis can't send you invalid blocks.

### Can I run my own Hypasis instance?

**Yes!** Hypasis is open source. See [CLOUD_DEPLOYMENT.md](CLOUD_DEPLOYMENT.md) for instructions.

---

## 🐛 Troubleshooting

### Problem: Not connecting to Hypasis bootnodes

**Solution:**
1. Check firewall allows port 30303:
   ```bash
   sudo ufw allow 30303/tcp
   sudo ufw allow 30303/udp
   ```

2. Verify DNS resolution:
   ```bash
   ping sync1.hypasis.io
   ```

3. Check Bor logs:
   ```bash
   journalctl -u bor -f | grep "Hypasis"
   ```

### Problem: Sync seems slow

**Possible causes:**
- Network bandwidth limitations on your side
- Disk I/O bottleneck (use NVMe SSDs)
- CPU limitations (need 8+ cores recommended)

**Check your sync rate:**
```bash
# Should see blocks increasing rapidly
watch -n 1 'curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  --data "{\"jsonrpc\":\"2.0\",\"method\":\"eth_blockNumber\",\"params\":[],\"id\":1}" \
  | jq -r ".result" | xargs printf "%d\n"'
```

### Problem: Heimdall can't connect to Bor

**Solution:**
Heimdall connects to your **local** Bor (port 8545), not Hypasis. Check:

```bash
# Test Bor RPC locally
curl http://localhost:8545 \
  -X POST \
  -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

If this fails, Bor isn't listening. Check your Bor configuration:
```toml
[jsonrpc]
http.addr = "0.0.0.0"  # or "localhost"
http.port = 8545
```

---

## 📞 Support

### Get Help

- **Discord:** https://discord.gg/hypasis
- **GitHub Issues:** https://github.com/hypasis/sync-protocol/issues
- **Documentation:** https://docs.hypasis.io

### Network Status

Check real-time status: https://status.hypasis.io

Shows:
- ✅ Service health
- 👥 Connected node operators
- 📊 Network sync progress
- 🌍 Regional availability

### Report Issues

If you encounter issues:

1. Collect logs:
   ```bash
   journalctl -u bor -n 100 > bor.log
   journalctl -u heimdall -n 100 > heimdall.log
   ```

2. Create GitHub issue: https://github.com/hypasis/sync-protocol/issues
3. Include: Bor version, OS, logs

---

## 🎓 Advanced Usage

### Use Only Specific Region

If you want lower latency, use bootnodes closest to you:

**US East:**
```toml
bootnodes = ["enode://PUBKEY1@sync1.hypasis.io:30303"]
```

**US West:**
```toml
bootnodes = ["enode://PUBKEY2@sync2.hypasis.io:30303"]
```

**EU Central:**
```toml
bootnodes = ["enode://PUBKEY3@sync3.hypasis.io:30303"]
```

**Recommendation:** Use all 3 for best redundancy and speed.

### Monitor Hypasis Connection

```bash
# Check connected peers
curl -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"admin_peers","params":[],"id":1}' \
  | jq '.result[] | select(.name | contains("Hypasis"))'
```

### Integration with Monitoring Tools

Add to your Prometheus config:

```yaml
scrape_configs:
  - job_name: 'hypasis'
    static_configs:
      - targets: ['sync.hypasis.io:9090']
    metrics_path: '/metrics'
```

---

## 📝 Example: Complete Validator Setup

```bash
# 1. Install Bor and Heimdall (standard process)
# ... (follow Polygon docs)

# 2. Create Bor config with Hypasis bootnodes
cat > /etc/bor/config.toml <<EOF
[p2p]
maxpeers = 50
bootnodes = [
  "enode://PUBKEY1@sync1.hypasis.io:30303",
  "enode://PUBKEY2@sync2.hypasis.io:30303",
  "enode://PUBKEY3@sync3.hypasis.io:30303"
]

[eth]
syncmode = "full"
snapshot = true

[jsonrpc]
http.addr = "0.0.0.0"
http.port = 8545
http.api = ["eth", "net", "web3", "bor"]
EOF

# 3. Start services
systemctl start heimdall
systemctl start bor

# 4. Monitor sync progress
watch -n 5 'curl -s -X POST http://localhost:8545 \
  -H "Content-Type: application/json" \
  --data "{\"jsonrpc\":\"2.0\",\"method\":\"eth_syncing\",\"params\":[],\"id\":1}"'

# 5. Wait for "result": false (fully synced)
# Typical time: 3-4 hours with Hypasis
```

---

## 🎉 Success!

Once fully synced, your node is ready to:
- ✅ Validate transactions
- ✅ Participate in consensus
- ✅ Earn staking rewards

**Welcome to the Hypasis community!** 🚀

---

**Questions?** Join our Discord: https://discord.gg/hypasis
