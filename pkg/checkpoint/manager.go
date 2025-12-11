package checkpoint

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hypasis/sync-protocol/internal/types"
	"github.com/hypasis/sync-protocol/pkg/config"
)

// Manager manages blockchain checkpoints
type Manager struct {
	config    *config.CheckpointConfig
	source    Source
	validator Validator

	// Cache of verified checkpoints
	checkpoints map[uint64]*types.Checkpoint
	latest      *types.Checkpoint
	mu          sync.RWMutex

	// Background tasks
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Source interface for fetching checkpoints
type Source interface {
	// FetchLatest fetches the latest checkpoint
	FetchLatest(ctx context.Context) (*types.Checkpoint, error)

	// FetchByNumber fetches a checkpoint at specific block
	FetchByNumber(ctx context.Context, blockNum uint64) (*types.Checkpoint, error)

	// FetchRange fetches checkpoints in a range
	FetchRange(ctx context.Context, start, end uint64) ([]*types.Checkpoint, error)
}

// Validator interface for checkpoint verification
type Validator interface {
	// Verify verifies a checkpoint's signatures
	Verify(ctx context.Context, checkpoint *types.Checkpoint) error

	// GetValidatorSet returns the validator set at a block height
	GetValidatorSet(ctx context.Context, blockNum uint64) ([]types.Validator, error)
}

// NewManager creates a new checkpoint manager
func NewManager(cfg *config.CheckpointConfig, source Source, validator Validator) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		config:      cfg,
		source:      source,
		validator:   validator,
		checkpoints: make(map[uint64]*types.Checkpoint),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the checkpoint manager
func (m *Manager) Start() error {
	// Fetch and verify latest checkpoint
	if err := m.updateLatestCheckpoint(); err != nil {
		return fmt.Errorf("failed to fetch initial checkpoint: %w", err)
	}

	// Start background checkpoint updater
	m.wg.Add(1)
	go m.checkpointUpdater()

	return nil
}

// Stop stops the checkpoint manager
func (m *Manager) Stop() error {
	m.cancel()
	m.wg.Wait()
	return nil
}

// GetLatest returns the latest verified checkpoint
func (m *Manager) GetLatest() *types.Checkpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.latest
}

// GetByNumber returns a checkpoint at a specific block number
func (m *Manager) GetByNumber(blockNum uint64) (*types.Checkpoint, error) {
	m.mu.RLock()
	cp, ok := m.checkpoints[blockNum]
	m.mu.RUnlock()

	if ok {
		return cp, nil
	}

	// Not in cache, fetch from source
	ctx, cancel := context.WithTimeout(m.ctx, m.config.Timeout)
	defer cancel()

	cp, err := m.source.FetchByNumber(ctx, blockNum)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch checkpoint: %w", err)
	}

	// Verify the checkpoint
	if err := m.validator.Verify(ctx, cp); err != nil {
		return nil, fmt.Errorf("checkpoint verification failed: %w", err)
	}

	// Cache the verified checkpoint
	m.mu.Lock()
	m.checkpoints[blockNum] = cp
	m.mu.Unlock()

	return cp, nil
}

// GetAll returns all cached checkpoints
func (m *Manager) GetAll() []*types.Checkpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cps := make([]*types.Checkpoint, 0, len(m.checkpoints))
	for _, cp := range m.checkpoints {
		cps = append(cps, cp)
	}

	return cps
}

// checkpointUpdater periodically updates the latest checkpoint
func (m *Manager) checkpointUpdater() {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if err := m.updateLatestCheckpoint(); err != nil {
				// Log error but continue
				fmt.Printf("Failed to update checkpoint: %v\n", err)
			}
		}
	}
}

// updateLatestCheckpoint fetches and verifies the latest checkpoint
func (m *Manager) updateLatestCheckpoint() error {
	ctx, cancel := context.WithTimeout(m.ctx, m.config.Timeout)
	defer cancel()

	// Fetch latest checkpoint
	cp, err := m.source.FetchLatest(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch latest checkpoint: %w", err)
	}

	// Verify the checkpoint
	if err := m.validator.Verify(ctx, cp); err != nil {
		return fmt.Errorf("checkpoint verification failed: %w", err)
	}

	// Update latest
	m.mu.Lock()
	m.latest = cp
	m.checkpoints[cp.BlockNumber] = cp
	m.mu.Unlock()

	return nil
}

// DefaultValidator provides basic checkpoint validation
type DefaultValidator struct {
	threshold float64 // Signature threshold (e.g., 0.66 for 2/3)
}

// NewDefaultValidator creates a new default validator
func NewDefaultValidator() *DefaultValidator {
	return &DefaultValidator{
		threshold: 0.66, // 2/3 threshold
	}
}

// Verify implements the Validator interface
func (v *DefaultValidator) Verify(ctx context.Context, cp *types.Checkpoint) error {
	if cp == nil {
		return fmt.Errorf("checkpoint is nil")
	}

	// Basic validation
	if cp.BlockNumber == 0 {
		return fmt.Errorf("invalid block number")
	}

	if cp.BlockHash == (common.Hash{}) {
		return fmt.Errorf("invalid block hash")
	}

	if len(cp.ValidatorSigs) == 0 {
		return fmt.Errorf("no validator signatures")
	}

	// Get validator set
	validators, err := v.GetValidatorSet(ctx, cp.BlockNumber)
	if err != nil {
		return fmt.Errorf("failed to get validator set: %w", err)
	}

	// Create signature verifier
	sigVerifier := NewSignatureVerifier(cp.ChainID)

	// Create validator map for easy lookup
	validatorMap := make(map[string]*types.Validator)
	totalStake := big.NewInt(0)
	for i := range validators {
		validatorMap[validators[i].ID] = &validators[i]
		totalStake.Add(totalStake, validators[i].Stake)
	}

	// Verify signatures
	signedStake := big.NewInt(0)

	for _, sig := range cp.ValidatorSigs {
		validator, ok := validatorMap[sig.ValidatorID]
		if !ok {
			// Unknown validator, skip
			continue
		}

		// Verify the signature using cryptographic verification
		err := sigVerifier.VerifyCheckpointSignature(cp, validator, sig.Signature)
		if err != nil {
			// Invalid signature, log but continue checking others
			fmt.Printf("Warning: Invalid signature from validator %s: %v\n", sig.ValidatorID, err)
			continue
		}

		// Valid signature, add stake
		signedStake.Add(signedStake, validator.Stake)
	}

	// Check threshold (2/3+ by stake)
	threshold := new(big.Int).Mul(totalStake, big.NewInt(66))
	threshold.Div(threshold, big.NewInt(100))

	if signedStake.Cmp(threshold) < 0 {
		return fmt.Errorf("insufficient signatures: got %s stake, need %s (66%% of %s total)",
			signedStake.String(), threshold.String(), totalStake.String())
	}

	return nil
}

// GetValidatorSet returns the validator set at a block height
func (v *DefaultValidator) GetValidatorSet(ctx context.Context, blockNum uint64) ([]types.Validator, error) {
	// TODO: Implement actual validator set fetching
	// For now, return mock validator set
	return []types.Validator{
		{
			ID:     "validator1",
			Stake:  big.NewInt(1000000),
			Active: true,
		},
		{
			ID:     "validator2",
			Stake:  big.NewInt(1000000),
			Active: true,
		},
		{
			ID:     "validator3",
			Stake:  big.NewInt(1000000),
			Active: true,
		},
	}, nil
}
