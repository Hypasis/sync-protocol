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

// BackwardSyncer handles backward/historical synchronization
type BackwardSyncer struct {
	config       *config.BackwardSyncConfig
	storage      storage.Storage
	checkpoint   *checkpoint.Manager
	blockFetcher BlockFetcher
	validator    validation.BlockValidator

	downloadedBlocks atomic.Uint64
	totalBlocks      atomic.Uint64
	blocksPerSec     atomic.Uint64

	paused atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBackwardSyncer creates a new backward syncer
func NewBackwardSyncer(
	cfg *config.BackwardSyncConfig,
	storage storage.Storage,
	checkpointMgr *checkpoint.Manager,
	fetcher BlockFetcher,
	validator validation.BlockValidator,
) *BackwardSyncer {
	return &BackwardSyncer{
		config:       cfg,
		storage:      storage,
		checkpoint:   checkpointMgr,
		blockFetcher: fetcher,
		validator:    validator,
	}
}

// Start starts the backward syncer
func (b *BackwardSyncer) Start(parentCtx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(parentCtx)

	// Calculate total blocks to download
	cp := b.checkpoint.GetLatest()
	if cp != nil {
		b.totalBlocks.Store(cp.BlockNumber)
	}

	// Start sync workers
	for i := 0; i < b.config.Workers; i++ {
		b.wg.Add(1)
		go b.syncWorker(i)
	}

	return nil
}

// Stop stops the backward syncer
func (b *BackwardSyncer) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
}

// Pause pauses the backward sync
func (b *BackwardSyncer) Pause() {
	b.paused.Store(true)
}

// Resume resumes the backward sync
func (b *BackwardSyncer) Resume() {
	b.paused.Store(false)
}

// GetStatus returns the current backward sync status
func (b *BackwardSyncer) GetStatus() types.BackwardSyncStatus {
	downloaded := b.downloadedBlocks.Load()
	total := b.totalBlocks.Load()

	progress := float64(0)
	if total > 0 {
		progress = (float64(downloaded) / float64(total)) * 100.0
	}

	return types.BackwardSyncStatus{
		DownloadedRanges: b.storage.GetAvailableRanges(),
		Progress:         progress,
		Syncing:          !b.paused.Load(),
		BlocksPerSec:     float64(b.blocksPerSec.Load()),
		TotalBlocks:      total,
		DownloadedBlocks: downloaded,
	}
}

// syncWorker is a worker goroutine that syncs historical blocks
func (b *BackwardSyncer) syncWorker(workerID int) {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			return
		default:
			// Check if paused
			if b.paused.Load() {
				time.Sleep(1 * time.Second)
				continue
			}

			// Get next gap to fill
			gap := b.storage.GetNextGap()
			if gap == nil {
				// No gaps, we're done
				time.Sleep(5 * time.Second)
				continue
			}

			// Sync a batch of blocks from the gap
			if err := b.syncBatch(gap); err != nil {
				// Log error and retry
				time.Sleep(1 * time.Second)
				continue
			}
		}
	}
}

// syncBatch syncs a batch of blocks from a gap
func (b *BackwardSyncer) syncBatch(gap *types.BlockRange) error {
	batchSize := uint64(b.config.BatchSize)

	// Calculate batch range
	start := gap.End - batchSize
	if start < gap.Start {
		start = gap.Start
	}
	end := gap.End

	// Fetch blocks from P2P network
	if b.blockFetcher == nil {
		return fmt.Errorf("block fetcher not initialized")
	}

	blocks, err := b.blockFetcher.FetchBlocks(b.ctx, start, end)
	if err != nil {
		return fmt.Errorf("failed to fetch batch [%d, %d): %w", start, end, err)
	}

	// Validate blocks (light validation for backward sync performance)
	if b.validator != nil {
		for i, block := range blocks {
			// Get parent from previous block in batch or from storage
			var parent *types.Block
			if i > 0 {
				parent = blocks[i-1]
			} else if block.Header.Number > 0 {
				parent, _ = b.storage.GetBlock(b.ctx, block.Header.Number-1)
			}

			// Validate header only for performance (light validation)
			if err := b.validator.ValidateBlockHeader(b.ctx, block.Header, parent.Header); err != nil {
				return fmt.Errorf("block %d header validation failed: %w", block.Header.Number, err)
			}
		}
	}

	// Store the blocks
	if err := b.storage.StoreBlocks(b.ctx, blocks); err != nil {
		return fmt.Errorf("failed to store batch [%d, %d): %w", start, end, err)
	}

	// Update metrics
	b.downloadedBlocks.Add(uint64(len(blocks)))
	b.blocksPerSec.Add(uint64(len(blocks)))

	return nil
}
