# Hypasis Sync Protocol - Architecture

## Table of Contents

- [Overview](#overview)
- [Core Components](#core-components)
- [Data Flow](#data-flow)
- [Checkpoint System](#checkpoint-system)
- [Sync Engines](#sync-engines)
- [Storage Layer](#storage-layer)
- [P2P Communication](#p2p-communication)
- [Security Model](#security-model)

## Overview

Hypasis Sync Protocol is designed as a middleware service that sits between blockchain nodes and the P2P network, enabling bidirectional synchronization without requiring modifications to existing node clients.

### Design Principles

1. **Zero Client Modification**: Works with any blockchain client as-is
2. **Chain Agnostic**: Adaptable to any blockchain architecture
3. **Performance First**: Optimized for minimal latency and maximum throughput
4. **Secure by Default**: Cryptographic verification at every step
5. **Observable**: Comprehensive metrics and monitoring

## Core Components

### 1. Checkpoint Manager

**Purpose**: Manages trusted checkpoints for bootstrap initialization

**Responsibilities**:
- Fetch checkpoints from trusted sources (L1 contracts, beacon chain, etc.)
- Verify checkpoint signatures against validator set
- Maintain checkpoint registry with historical checkpoints
- Provide checkpoint state snapshots

**Key Types**:
```go
type Checkpoint struct {
    BlockNumber   uint64
    BlockHash     common.Hash
    StateRoot     common.Hash
    Timestamp     uint64
    ValidatorSigs []Signature  // 2/3+ validator signatures
}

type CheckpointSource interface {
    FetchLatest() (*Checkpoint, error)
    FetchByNumber(uint64) (*Checkpoint, error)
    VerifySignatures(*Checkpoint) error
}
```

### 2. Sync Coordinator

**Purpose**: Orchestrates forward and backward sync processes

**Responsibilities**:
- Manage dual sync pipelines (forward + backward)
- Resource allocation and throttling
- Conflict resolution between sync directions
- Progress tracking and reporting

**Architecture**:
```
SyncCoordinator
├── ForwardSyncer (High Priority)
│   ├── Block Fetcher
│   ├── Transaction Validator
│   └── State Updater
└── BackwardSyncer (Low Priority)
    ├── Historical Block Fetcher
    ├── Batch Processor
    └── Gap Filler
```

### 3. Storage Layer

**Purpose**: Gap-aware storage system for blockchain data

**Responsibilities**:
- Track available vs missing block ranges
- Efficient block/state storage and retrieval
- Cache frequently accessed data
- Provide query interface with gap awareness

**Data Structure**:
```go
type GapTracker struct {
    // Bitmap or interval tree of available blocks
    availableRanges *IntervalTree

    // Track what we have
    minBlock uint64
    maxBlock uint64
}

type Storage interface {
    StoreBlock(*Block) error
    GetBlock(uint64) (*Block, error)
    HasBlock(uint64) bool
    GetGaps() []Range
    GetAvailableRanges() []Range
}
```

### 4. P2P Layer

**Purpose**: Network communication between nodes and service

**Modes**:

**Mode A: HTTP/RPC Proxy**
- Node connects via HTTP/RPC
- Service translates to P2P requests
- Simplest integration

**Mode B: P2P Proxy**
- Service acts as devp2p/libp2p node
- Transparent to blockchain node
- Full protocol support

**Mode C: Sidecar**
- Runs alongside node in same pod
- Communicates via localhost
- Kubernetes-native

### 5. API & Metrics

**Purpose**: Observability and control interface

**REST API Endpoints**:
- `GET /api/v1/status` - Sync status
- `GET /api/v1/gaps` - Missing ranges
- `GET /api/v1/checkpoints` - Available checkpoints
- `POST /api/v1/sync/pause` - Pause backward sync
- `POST /api/v1/sync/resume` - Resume backward sync

**Prometheus Metrics**:
- Block sync rates (forward/backward)
- Bandwidth usage
- Cache hit ratios
- Peer connection stats

## Data Flow

### Bootstrap Flow

```
1. Service starts
   ↓
2. Fetch latest checkpoint from source
   ↓
3. Verify checkpoint signatures (2/3+ validators)
   ↓
4. Download checkpoint state snapshot (~50GB)
   ↓
5. Initialize storage with checkpoint state
   ↓
6. Service ready - node can connect
```

### Forward Sync Flow

```
1. Node requests current block (N)
   ↓
2. Service checks local storage
   ↓
3a. If available: Return from cache
3b. If missing: Fetch from P2P network
   ↓
4. Validate block
   ↓
5. Store block and update state
   ↓
6. Return to node
   ↓
7. Node advances to block N+1 (repeat)
```

### Backward Sync Flow (Background)

```
1. Identify largest gap: [0, checkpoint)
   ↓
2. Split into batches (e.g., 10K blocks each)
   ↓
3. Prioritize recent history first
   ↓
4. For each batch:
   a. Find peers with historical data
   b. Download blocks in parallel
   c. Validate blocks (optional)
   d. Store blocks
   e. Update gap tracker
   ↓
5. Throttle to avoid impacting forward sync
   ↓
6. Continue until all gaps filled
```

## Checkpoint System

### Checkpoint Sources by Chain

**Polygon PoS**:
- Source: Ethereum L1 contract
- Contract: `0x86E4Dc95c7FBdBf52e33D563BbDB00823894C287`
- Verification: Heimdall validator signatures
- Frequency: Every 256 blocks (sprint)

**Ethereum**:
- Source: Beacon Chain
- Verification: Beacon block signatures
- Frequency: Every epoch (~6.4 minutes)

**BSC**:
- Source: Native validator set
- Verification: BLS signatures
- Frequency: Every epoch

### Checkpoint Verification

```go
func (cm *CheckpointManager) Verify(cp *Checkpoint) error {
    // 1. Verify block hash matches state root
    if !verifyStateRoot(cp.BlockHash, cp.StateRoot) {
        return ErrInvalidStateRoot
    }

    // 2. Fetch validator set at checkpoint height
    validators := cm.getValidatorSet(cp.BlockNumber)

    // 3. Verify signatures (need 2/3+ majority)
    validSigs := 0
    totalStake := big.NewInt(0)
    signedStake := big.NewInt(0)

    for _, val := range validators {
        totalStake.Add(totalStake, val.Stake)

        for _, sig := range cp.ValidatorSigs {
            if verifySignature(val.PubKey, cp.Hash(), sig) {
                validSigs++
                signedStake.Add(signedStake, val.Stake)
                break
            }
        }
    }

    // 4. Check threshold (2/3+ by stake)
    threshold := new(big.Int).Mul(totalStake, big.NewInt(2))
    threshold.Div(threshold, big.NewInt(3))

    if signedStake.Cmp(threshold) < 0 {
        return ErrInsufficientSignatures
    }

    return nil
}
```

## Sync Engines

### Forward Sync Engine

**Characteristics**:
- **Priority**: High (always preferenced)
- **Goal**: Keep node caught up with chain tip
- **Strategy**: Sequential, low latency
- **Resource Allocation**: 70% of bandwidth/CPU

**Algorithm**:
```
1. Request next block from node perspective
2. Check local storage (cache)
3. If cached: return immediately
4. If not cached:
   a. Request from P2P peers
   b. Validate block
   c. Store in cache
   d. Return to node
5. Repeat
```

**Optimizations**:
- Pre-fetch upcoming blocks (look-ahead)
- Maintain hot cache of recent blocks
- Fast path for sequential access

### Backward Sync Engine

**Characteristics**:
- **Priority**: Low (background)
- **Goal**: Fill historical data gaps
- **Strategy**: Parallel batch downloads
- **Resource Allocation**: 30% of bandwidth/CPU

**Algorithm**:
```
1. Identify all gaps in storage
2. Prioritize gaps (recent first)
3. For each gap:
   a. Split into batches
   b. Find peers with historical data
   c. Download batches in parallel (4-8 workers)
   d. Light validation (headers only, optional full)
   e. Store blocks
   f. Update gap tracker
4. Throttle based on forward sync activity
5. Continue until all gaps filled
```

**Optimizations**:
- Adaptive batch sizes based on peer performance
- Compression for historical data transfer
- Skip full validation for old blocks (header-only)
- Resume from interruptions

## Storage Layer

### Architecture

```
Storage Layer
├── Block Storage (LevelDB/BadgerDB)
│   ├── Headers
│   ├── Bodies
│   └── Receipts
├── State Storage (Merkle Patricia Trie)
│   └── Account states, contract storage
├── Gap Tracker (In-memory + persistent)
│   └── Available block ranges
└── Cache Layer (LRU + Hot cache)
    └── Recent blocks, frequently accessed data
```

### Gap Tracking

**Data Structure**: Interval Tree

```go
type IntervalTree struct {
    // Stores non-overlapping ranges of available blocks
    intervals []*Interval
}

type Interval struct {
    Start uint64  // First available block
    End   uint64  // Last available block
}

// Example state:
// [0, 1000) - MISSING
// [1000, 5000) - AVAILABLE
// [5000, 50000) - MISSING
// [50000, 63000000) - AVAILABLE
```

**Operations**:
- `AddRange(start, end)` - Mark range as available
- `HasBlock(num)` - Check if block available
- `GetGaps()` - Return all missing ranges
- `GetNextGap()` - Get next gap to fill

### Cache Strategy

**Three-tier caching**:

1. **Hot Cache** (RAM, ~1GB)
   - Last 1000 blocks
   - Always in memory
   - Sub-millisecond access

2. **Warm Cache** (SSD, ~50GB)
   - Recent checkpoint state
   - Frequently accessed blocks
   - Millisecond access

3. **Cold Storage** (HDD/Network, unlimited)
   - Historical blocks
   - Accessed on-demand
   - Second-level access

## P2P Communication

### Protocol Support

- **Ethereum devp2p**: For Ethereum, Polygon, BSC
- **libp2p**: For modern chains (Cosmos, Polkadot)
- **Custom**: Chain-specific protocols

### Peer Management

```go
type PeerManager struct {
    // Active peer connections
    peers map[string]*Peer

    // Peer capabilities
    capabilities map[string]*PeerCaps

    // Reputation scoring
    reputation map[string]float64
}

type PeerCaps struct {
    HistoricalData bool      // Serves historical blocks
    FullNode       bool      // Has full chain
    ArchiveNode    bool      // Has all states
    BlockRange     Range     // Available block range
}
```

**Peer Selection Strategy**:
- For forward sync: Choose peers at chain tip
- For backward sync: Choose peers with historical data
- Prefer high-reputation peers
- Load balance across peers

## Security Model

### Threat Model

**Threats**:
1. Malicious checkpoint injection
2. Invalid block data from peers
3. Eclipse attacks (isolated from honest nodes)
4. DoS attacks (resource exhaustion)

**Mitigations**:
1. **Checkpoint Verification**: 2/3+ validator signatures required
2. **Block Validation**: Full validation for forward sync, light for backward
3. **Peer Diversity**: Connect to multiple diverse peers
4. **Rate Limiting**: Throttle requests to prevent DoS
5. **Reputation System**: Track peer behavior, ban malicious peers

### Trust Assumptions

1. **Checkpoint Source**: Trusted (L1 contract, beacon chain)
2. **Validator Set**: Honest majority (> 2/3 by stake)
3. **P2P Network**: Presence of honest peers (> 1)

### Cryptographic Verification

- Checkpoint signatures: BLS, ECDSA (chain-dependent)
- Block hashes: Keccak256, SHA256
- State roots: Merkle Patricia Trie verification

## Performance Considerations

### Bottlenecks

1. **Network Bandwidth**: Backward sync limited by download speed
2. **Disk I/O**: State updates during forward sync
3. **CPU**: Block validation, signature verification

### Optimizations

1. **Parallelization**: Multi-threaded sync engines
2. **Compression**: Compress historical data transfers
3. **Batching**: Group operations to reduce overhead
4. **Caching**: Multi-tier cache strategy
5. **Skip Validation**: Light validation for old blocks

### Resource Limits

```yaml
resources:
  cpu: 4 cores
  memory: 16GB
  disk: 2TB (full archive) or 500GB (pruned)
  bandwidth: 100 Mbps (50 up / 50 down)
```

## Failure Scenarios

### Node Restart

- **State**: Persisted to disk
- **Recovery**: Resume from last checkpoint
- **Impact**: < 1 minute downtime

### Network Partition

- **Detection**: Peer connectivity monitoring
- **Action**: Reconnect to new peers
- **Impact**: Forward sync pauses, resumes on reconnection

### Corrupted Data

- **Detection**: Hash mismatch on read
- **Action**: Re-download affected blocks
- **Impact**: Localized, automatic recovery

### Checkpoint Compromise (Rare)

- **Detection**: Community alert, manual override
- **Action**: Rollback to previous checkpoint
- **Impact**: Manual intervention required

## Future Enhancements

1. **ZK Proofs**: Use ZK proofs for instant state verification
2. **Incentive Layer**: Pay peers for historical data
3. **DHT Storage**: Distributed historical data storage
4. **Cross-chain Sync**: Sync multiple chains simultaneously
5. **AI Optimization**: ML-based peer selection and resource allocation

---

**Version**: 1.0-alpha
**Last Updated**: 2025-12-11
**Authors**: Hypasis Team
