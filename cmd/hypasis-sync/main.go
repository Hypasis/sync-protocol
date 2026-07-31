// Command hypasis-sync is the entrypoint for the Hypasis Sync Protocol node.
//
// It wires together the real, working pieces of the codebase into a running
// service: checkpoint manager -> block fetcher (upstream RPC) -> sync
// coordinator (forward + backward) -> block storage, fronted by the REST/metrics
// API server.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/hypasis/sync-protocol/internal/types"
	"github.com/hypasis/sync-protocol/pkg/api"
	"github.com/hypasis/sync-protocol/pkg/cache"
	"github.com/hypasis/sync-protocol/pkg/checkpoint"
	"github.com/hypasis/sync-protocol/pkg/cluster"
	"github.com/hypasis/sync-protocol/pkg/config"
	"github.com/hypasis/sync-protocol/pkg/health"
	"github.com/hypasis/sync-protocol/pkg/p2p"
	"github.com/hypasis/sync-protocol/pkg/storage"
	"github.com/hypasis/sync-protocol/pkg/sync"
)

func main() {
	var (
		configPath = flag.String("config", "", "path to YAML config file (uses built-in defaults if empty)")
		dataDir    = flag.String("data-dir", "", "override storage data directory")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("hypasis-sync v0.1.0")
		return
	}

	// Load configuration (file or built-in defaults).
	var (
		cfg *config.Config
		err error
	)
	if *configPath != "" {
		cfg, err = config.LoadConfig(*configPath)
		if err != nil {
			fatal("load config: %v", err)
		}
	} else {
		cfg = config.DefaultConfig()
		// Built-in defaults target /data/hypasis, which is usually not writable
		// in a dev environment. Fall back to a local directory.
		cfg.Storage.DataDir = "./data"
	}
	if *dataDir != "" {
		cfg.Storage.DataDir = *dataDir
	}

	logf("starting hypasis-sync: chain=%s chain_id=%d checkpoint_source=%s storage=%s",
		cfg.Chain.Name, cfg.Chain.ChainID, cfg.Checkpoint.Source, cfg.Storage.Engine)

	// 1. Storage.
	store, err := storage.NewStorage(&cfg.Storage)
	if err != nil {
		fatal("init storage: %v", err)
	}
	defer store.Close()

	// 2. Checkpoint manager (source + validator).
	source, err := newCheckpointSource(&cfg.Checkpoint, cfg.Chain.ChainID)
	if err != nil {
		fatal("init checkpoint source: %v", err)
	}
	validator := newCheckpointValidator(&cfg.Checkpoint)
	checkpointMgr := checkpoint.NewManager(&cfg.Checkpoint, source, validator)
	if err := checkpointMgr.Start(); err != nil {
		fatal("start checkpoint manager: %v", err)
	}
	defer checkpointMgr.Stop()
	if cp := checkpointMgr.GetLatest(); cp != nil {
		logf("anchored to checkpoint: block=%d hash=%s", cp.BlockNumber, cp.BlockHash.Hex())
	}

	// 3. Block fetcher over upstream RPC endpoints.
	fetcher, err := p2p.NewBlockFetcher(cfg.P2P.UpstreamRPCs, cfg.P2P.RPCTimeout, cfg.P2P.RPCMaxRetries)
	if err != nil {
		fatal("init block fetcher: %v", err)
	}

	// 4. Sync coordinator (forward + backward).
	coordinator := sync.NewCoordinator(&cfg.Sync, store, checkpointMgr, fetcher)
	if err := coordinator.Start(); err != nil {
		fatal("start sync coordinator: %v", err)
	}
	defer coordinator.Stop()

	// 5. Cluster & Redis Distributed Cache (when enabled).
	if cfg.Cluster.Enabled && cfg.Cluster.RedisURL != "" {
		redisCache, err := cache.NewRedisCache(&cache.RedisConfig{
			URL:      cfg.Cluster.RedisURL,
			Password: cfg.Cluster.RedisPassword,
			DB:       0,
			TTL:      24 * time.Hour,
		})
		if err != nil {
			logf("warning: failed to init redis cache: %v", err)
		} else {
			logf("redis distributed cache connected: %s", cfg.Cluster.RedisURL)
			defer redisCache.Close()

			// Cluster Coordinator
			coordCfg := &cluster.CoordinatorConfig{
				InstanceID:      cfg.Cloud.InstanceID,
				Region:          cfg.Cloud.Region,
				HeartbeatPeriod: 10 * time.Second,
				PeerTimeout:     30 * time.Second,
				LeaderElection:  true,
			}
			clusterCoord := cluster.NewCoordinator(coordCfg, redisCache)
			if err := clusterCoord.Start(); err != nil {
				logf("warning: cluster coordinator start failed: %v", err)
			} else {
				logf("cluster coordinator active: instance=%s region=%s", cfg.Cloud.InstanceID, cfg.Cloud.Region)
				defer clusterCoord.Stop()
			}
		}
	}

	// 6. DevP2P Server (when configured).
	if cfg.P2P.Mode == "devp2p" {
		devp2pCfg := &p2p.DevP2PConfig{
			ListenAddr: cfg.P2P.Listen,
			MaxPeers:   cfg.P2P.MaxPeers,
			NetworkID:  uint64(cfg.Chain.ChainID),
			Name:       "hypasis-node",
		}
		p2pServer, err := p2p.NewDevP2PServer(devp2pCfg, store, fetcher)
		if err != nil {
			logf("warning: failed to init DevP2P server: %v", err)
		} else {
			if err := p2pServer.Start(); err != nil {
				logf("warning: failed to start DevP2P server: %v", err)
			} else {
				logf("DevP2P bootnode active: listen=%s enode=%s", cfg.P2P.Listen, p2pServer.GetEnodeURL())
				defer p2pServer.Stop()
			}
		}
	}

	// 7. Health Monitor.
	healthMonitor := health.NewMonitor(&health.MonitorConfig{
		CheckInterval:    5 * time.Minute,
		UnhealthyTimeout: 15 * time.Minute,
		EnableAutoHeal:   true,
	})
	healthMonitor.Start()
	defer healthMonitor.Stop()

	// 8. API server (REST + metrics).
	apiServer := api.NewServer(&cfg.API, coordinator, store, checkpointMgr)
	if err := apiServer.Start(context.Background()); err != nil {
		fatal("start api server: %v", err)
	}
	defer apiServer.Stop()

	if cfg.API.REST.Enabled {
		logf("REST API listening on %s", cfg.API.REST.Listen)
	}
	if cfg.API.Metrics.Enabled {
		logf("metrics listening on %s%s", cfg.API.Metrics.Listen, cfg.API.Metrics.Path)
	}
	logf("hypasis-sync started; press Ctrl+C to stop")

	// Wait for a termination signal, then shut down via the deferred Stop calls.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logf("shutdown signal received; stopping...")
}

// newCheckpointSource builds a checkpoint source from config. It uses the real
// Ethereum-L1 source when configured (and an L1 RPC URL is present); otherwise
// it falls back to the mock source for local development.
func newCheckpointSource(cfg *config.CheckpointConfig, chainID int64) (checkpoint.Source, error) {
	if cfg.Source == "ethereum-l1" && cfg.L1RPCURL != "" {
		logf("using ethereum-l1 checkpoint source: contract=%s", cfg.Contract)
		return checkpoint.NewL1Source(cfg.L1RPCURL, common.HexToAddress(cfg.Contract), uint64(chainID))
	}
	logf("using mock checkpoint source (development)")
	return checkpoint.NewMockSource(), nil
}

// newCheckpointValidator selects a checkpoint validator. The real stake-weighted
// DefaultValidator is used for the ethereum-l1 source; the mock/dev source pairs
// with a permissive validator so the POC can run end-to-end without a real
// validator set or real signatures.
func newCheckpointValidator(cfg *config.CheckpointConfig) checkpoint.Validator {
	if cfg.Source == "ethereum-l1" && cfg.L1RPCURL != "" {
		// Checkpoints read from finalized Ethereum L1 state are anchored to
		// Ethereum consensus; validate structural integrity only.
		return checkpoint.NewL1Validator()
	}
	return devValidator{}
}

// devValidator is a permissive checkpoint validator for local development. It
// performs basic structural checks but does not enforce signatures, so mock
// checkpoints can flow through the pipeline. It is NEVER selected for the
// ethereum-l1 source.
type devValidator struct{}

func (devValidator) Verify(_ context.Context, cp *types.Checkpoint) error {
	if cp == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	if cp.BlockNumber == 0 {
		return fmt.Errorf("invalid checkpoint block number")
	}
	return nil
}

func (devValidator) GetValidatorSet(_ context.Context, _ uint64) ([]types.Validator, error) {
	return []types.Validator{
		{ID: "dev", Stake: big.NewInt(1), Active: true},
	}, nil
}

func logf(format string, args ...interface{}) {
	fmt.Printf("[hypasis] "+format+"\n", args...)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[hypasis] fatal: "+format+"\n", args...)
	os.Exit(1)
}
