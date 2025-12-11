package sync

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hypasis/sync-protocol/internal/types"
	"github.com/hypasis/sync-protocol/pkg/checkpoint"
	"github.com/hypasis/sync-protocol/pkg/config"
	"github.com/hypasis/sync-protocol/pkg/storage"
	"github.com/hypasis/sync-protocol/pkg/validation"
)

// BlockFetcher interface for fetching blocks (to avoid circular imports)
type BlockFetcher interface {
	FetchBlock(ctx context.Context, blockNum uint64) (*types.Block, error)
	FetchBlocks(ctx context.Context, start, end uint64) ([]*types.Block, error)
	FetchLatestBlockNumber(ctx context.Context) (uint64, error)
}

// ForwardSyncer handles forward synchronization
type ForwardSyncer struct {
	config       *config.ForwardSyncConfig
	storage      storage.Storage
	checkpoint   *checkpoint.Manager
	blockFetcher BlockFetcher
	validator    validation.BlockValidator

	currentBlock atomic.Uint64
	targetBlock  atomic.Uint64
	blocksPerSec atomic.Uint64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewForwardSyncer creates a new forward syncer
func NewForwardSyncer(
	cfg *config.ForwardSyncConfig,
	storage storage.Storage,
	checkpointMgr *checkpoint.Manager,
	fetcher BlockFetcher,
	validator validation.BlockValidator,
) *ForwardSyncer {
	return &ForwardSyncer{
		config:       cfg,
		storage:      storage,
		checkpoint:   checkpointMgr,
		blockFetcher: fetcher,
		validator:    validator,
	}
}

// Start starts the forward syncer
func (f *ForwardSyncer) Start(parentCtx context.Context) error {
	f.ctx, f.cancel = context.WithCancel(parentCtx)

	// Get latest checkpoint as starting point
	cp := f.checkpoint.GetLatest()
	if cp == nil {
		return fmt.Errorf("no checkpoint available")
	}

	f.currentBlock.Store(cp.BlockNumber)
	f.targetBlock.Store(cp.BlockNumber + 1000) // Start with target 1000 blocks ahead

	// Start sync workers
	for i := 0; i < f.config.Workers; i++ {
		f.wg.Add(1)
		go f.syncWorker(i)
	}

	// Start target block updater
	f.wg.Add(1)
	go f.targetUpdater()

	return nil
}

// Stop stops the forward syncer
func (f *ForwardSyncer) Stop() {
	if f.cancel != nil {
		f.cancel()
	}
	f.wg.Wait()
}

// GetStatus returns the current forward sync status
func (f *ForwardSyncer) GetStatus() types.ForwardSyncStatus {
	current := f.currentBlock.Load()
	target := f.targetBlock.Load()

	progress := float64(0)
	if target > 0 {
		progress = (float64(current) / float64(target)) * 100.0
		if progress > 100 {
			progress = 100
		}
	}

	return types.ForwardSyncStatus{
		CurrentBlock: current,
		TargetBlock:  target,
		Progress:     progress,
		Syncing:      true,
		BlocksPerSec: float64(f.blocksPerSec.Load()),
	}
}

// syncWorker is a worker goroutine that syncs blocks
func (f *ForwardSyncer) syncWorker(workerID int) {
	defer f.wg.Done()

	for {
		select {
		case <-f.ctx.Done():
			return
		default:
			// Get next block to sync
			blockNum := f.currentBlock.Add(1) - 1

			// Check if we've reached target
			if blockNum >= f.targetBlock.Load() {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Sync the block
			if err := f.syncBlock(blockNum); err != nil {
				// Log error and retry
				time.Sleep(1 * time.Second)
				f.currentBlock.Add(^uint64(0)) // Decrement by 1 (two's complement)
				continue
			}

			// Update metrics
			f.blocksPerSec.Add(1)
		}
	}
}

// syncBlock syncs a single block
func (f *ForwardSyncer) syncBlock(blockNum uint64) error {
	// Check if we already have the block
	if f.storage.HasBlock(f.ctx, blockNum) {
		return nil
	}

	// Fetch block from P2P network
	if f.blockFetcher == nil {
		return fmt.Errorf("block fetcher not initialized")
	}

	block, err := f.blockFetcher.FetchBlock(f.ctx, blockNum)
	if err != nil {
		return fmt.Errorf("failed to fetch block %d: %w", blockNum, err)
	}

	// Validate the block
	if f.validator != nil {
		// Try to get parent block for validation
		var parent *types.Block
		if blockNum > 0 {
			parent, _ = f.storage.GetBlock(f.ctx, blockNum-1)
		}

		// Validate the block (with or without parent)
		if err := f.validator.ValidateBlock(f.ctx, block, parent); err != nil {
			return fmt.Errorf("block %d validation failed: %w", blockNum, err)
		}
	}

	// Store the block
	if err := f.storage.StoreBlock(f.ctx, block); err != nil {
		return fmt.Errorf("failed to store block %d: %w", blockNum, err)
	}

	return nil
}

// targetUpdater periodically updates the target block
func (f *ForwardSyncer) targetUpdater() {
	defer f.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			// Query network for latest block
			if f.blockFetcher != nil {
				latest, err := f.blockFetcher.FetchLatestBlockNumber(f.ctx)
				if err != nil {
					// Log error but continue
					fmt.Printf("Warning: failed to fetch latest block number: %v\n", err)
					continue
				}
				f.targetBlock.Store(latest)
			}

			// Reset blocks per second counter
			f.blocksPerSec.Store(0)
		}
	}
}
