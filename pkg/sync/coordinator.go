package sync

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/hypasis/sync-protocol/internal/types"
	"github.com/hypasis/sync-protocol/pkg/checkpoint"
	"github.com/hypasis/sync-protocol/pkg/config"
	"github.com/hypasis/sync-protocol/pkg/storage"
	"github.com/hypasis/sync-protocol/pkg/validation"
)

// Coordinator manages forward and backward sync processes
type Coordinator struct {
	config       *config.SyncConfig
	storage      storage.Storage
	checkpoint   *checkpoint.Manager
	blockFetcher BlockFetcher

	forwardSync  *ForwardSyncer
	backwardSync *BackwardSyncer

	status     *types.SyncStatus
	statusLock sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCoordinator creates a new sync coordinator
func NewCoordinator(
	cfg *config.SyncConfig,
	storage storage.Storage,
	checkpointMgr *checkpoint.Manager,
	fetcher BlockFetcher,
) *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())

	coord := &Coordinator{
		config:       cfg,
		storage:      storage,
		checkpoint:   checkpointMgr,
		blockFetcher: fetcher,
		ctx:          ctx,
		cancel:       cancel,
		status: &types.SyncStatus{
			StartTime: time.Now(),
		},
	}

	// Create validators based on configuration
	var forwardValidator validation.BlockValidator
	var backwardValidator validation.BlockValidator

	// Parse validation levels from config
	forwardValidationLevel := validation.ParseValidationLevel(cfg.Forward.ValidationLevel)
	backwardValidationLevel := validation.ParseValidationLevel(cfg.Backward.ValidationLevel)

	// Get chain ID from checkpoint (if available)
	var chainID *big.Int
	if cp := checkpointMgr.GetLatest(); cp != nil && cp.ChainID != nil {
		chainID = cp.ChainID
	} else {
		chainID = big.NewInt(137) // Default to Polygon mainnet
	}

	// Create forward validator based on level
	switch forwardValidationLevel {
	case validation.ValidationNone:
		forwardValidator = validation.NewNoOpValidator()
	case validation.ValidationHeader:
		forwardValidator = validation.NewLightValidator(&validation.ValidatorConfig{
			ChainID:     chainID,
			MinGasLimit: 5000,
			MaxGasLimit: 30000000,
			BlockPeriod: 2,
			SprintSize:  16,
		})
	default: // ValidationLight or ValidationFull
		forwardValidator = validation.NewPolygonValidator(&validation.ValidatorConfig{
			ChainID:     chainID,
			MinGasLimit: 5000,
			MaxGasLimit: 30000000,
			BlockPeriod: 2,
			SprintSize:  16,
		})
	}

	// Create backward validator (typically lighter for performance)
	switch backwardValidationLevel {
	case validation.ValidationNone:
		backwardValidator = validation.NewNoOpValidator()
	default:
		// Use light validator for backward sync (header only)
		backwardValidator = validation.NewLightValidator(&validation.ValidatorConfig{
			ChainID:     chainID,
			MinGasLimit: 5000,
			MaxGasLimit: 30000000,
			BlockPeriod: 2,
			SprintSize:  16,
		})
	}

	// Initialize forward syncer with validator
	coord.forwardSync = NewForwardSyncer(
		&cfg.Forward,
		storage,
		checkpointMgr,
		fetcher,
		forwardValidator,
	)

	// Initialize backward syncer with validator
	coord.backwardSync = NewBackwardSyncer(
		&cfg.Backward,
		storage,
		checkpointMgr,
		fetcher,
		backwardValidator,
	)

	return coord
}

// Start starts the sync coordinator
func (c *Coordinator) Start() error {
	// Start forward sync (high priority)
	if c.config.Forward.Enabled {
		if err := c.forwardSync.Start(c.ctx); err != nil {
			return fmt.Errorf("failed to start forward sync: %w", err)
		}
	}

	// Start backward sync (low priority, background)
	if c.config.Backward.Enabled {
		if err := c.backwardSync.Start(c.ctx); err != nil {
			return fmt.Errorf("failed to start backward sync: %w", err)
		}
	}

	// Start status updater
	c.wg.Add(1)
	go c.statusUpdater()

	return nil
}

// Stop stops the sync coordinator
func (c *Coordinator) Stop() error {
	c.cancel()

	// Stop syncers
	if c.forwardSync != nil {
		c.forwardSync.Stop()
	}
	if c.backwardSync != nil {
		c.backwardSync.Stop()
	}

	c.wg.Wait()
	return nil
}

// GetStatus returns the current sync status
func (c *Coordinator) GetStatus() types.SyncStatus {
	c.statusLock.RLock()
	defer c.statusLock.RUnlock()

	status := *c.status
	status.Uptime = time.Since(c.status.StartTime)

	return status
}

// IsValidatorReady checks if the node is ready for validator duties
func (c *Coordinator) IsValidatorReady() bool {
	status := c.GetStatus()

	// Validator is ready when:
	// 1. Forward sync is caught up (within 10 blocks of target)
	// 2. Forward sync progress > 99%
	return status.ForwardSync.Syncing &&
		(status.ForwardSync.TargetBlock-status.ForwardSync.CurrentBlock) < 10 &&
		status.ForwardSync.Progress > 99.0
}

// PauseBackwardSync pauses backward synchronization
func (c *Coordinator) PauseBackwardSync() error {
	if c.backwardSync != nil {
		c.backwardSync.Pause()
	}
	return nil
}

// ResumeBackwardSync resumes backward synchronization
func (c *Coordinator) ResumeBackwardSync() error {
	if c.backwardSync != nil {
		c.backwardSync.Resume()
	}
	return nil
}

// statusUpdater periodically updates the sync status
func (c *Coordinator) statusUpdater() {
	defer c.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.updateStatus()
		}
	}
}

// updateStatus updates the sync status from syncers
func (c *Coordinator) updateStatus() {
	c.statusLock.Lock()
	defer c.statusLock.Unlock()

	// Update forward sync status
	if c.forwardSync != nil {
		c.status.ForwardSync = c.forwardSync.GetStatus()
	}

	// Update backward sync status
	if c.backwardSync != nil {
		c.status.BackwardSync = c.backwardSync.GetStatus()
	}

	// Update validator ready status
	c.status.ValidatorReady = c.IsValidatorReady()
}
