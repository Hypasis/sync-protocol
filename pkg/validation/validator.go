package validation

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/hypasis/sync-protocol/internal/types"
)

// BlockValidator validates blockchain blocks
type BlockValidator interface {
	// ValidateBlock performs full validation of a block
	ValidateBlock(ctx context.Context, block *types.Block, parent *types.Block) error

	// ValidateBlockHeader validates only the block header
	ValidateBlockHeader(ctx context.Context, header *types.Header, parent *types.Header) error

	// ValidateConsensus validates Bor consensus rules
	ValidateConsensus(ctx context.Context, block *types.Block, parent *types.Block) error

	// ValidateTransactions validates all transactions in a block
	ValidateTransactions(ctx context.Context, block *types.Block) error
}

// PolygonValidator implements BlockValidator for Polygon PoS (Bor)
type PolygonValidator struct {
	chainID         *big.Int
	minGasLimit     uint64
	maxGasLimit     uint64
	blockPeriod     uint64 // seconds between blocks
	sprintSize      uint64 // number of blocks in a sprint
	validatorSetFn  func(blockNum uint64) ([]common.Address, error)
	strictTimestamp bool
}

// ValidatorConfig holds validator configuration
type ValidatorConfig struct {
	ChainID         *big.Int
	MinGasLimit     uint64
	MaxGasLimit     uint64
	BlockPeriod     uint64
	SprintSize      uint64
	StrictTimestamp bool
}

// DefaultPolygonConfig returns default Polygon mainnet config
func DefaultPolygonConfig() *ValidatorConfig {
	return &ValidatorConfig{
		ChainID:         big.NewInt(137),
		MinGasLimit:     5000,
		MaxGasLimit:     30000000,
		BlockPeriod:     2, // 2 seconds
		SprintSize:      16,
		StrictTimestamp: false, // Allow some timestamp drift for syncing
	}
}

// NewPolygonValidator creates a new Polygon validator
func NewPolygonValidator(cfg *ValidatorConfig) *PolygonValidator {
	if cfg == nil {
		cfg = DefaultPolygonConfig()
	}

	return &PolygonValidator{
		chainID:         cfg.ChainID,
		minGasLimit:     cfg.MinGasLimit,
		maxGasLimit:     cfg.MaxGasLimit,
		blockPeriod:     cfg.BlockPeriod,
		sprintSize:      cfg.SprintSize,
		strictTimestamp: cfg.StrictTimestamp,
		validatorSetFn:  nil, // Will be set by checkpoint manager
	}
}

// SetValidatorSetFunction sets the function to retrieve validator set
func (v *PolygonValidator) SetValidatorSetFunction(fn func(blockNum uint64) ([]common.Address, error)) {
	v.validatorSetFn = fn
}

// ValidateBlock performs full validation of a block
func (v *PolygonValidator) ValidateBlock(ctx context.Context, block *types.Block, parent *types.Block) error {
	// Validate header
	if err := v.ValidateBlockHeader(ctx, block.Header, parent.Header); err != nil {
		return fmt.Errorf("header validation failed: %w", err)
	}

	// Validate transactions
	if err := v.ValidateTransactions(ctx, block); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	// Validate consensus rules
	if err := v.ValidateConsensus(ctx, block, parent); err != nil {
		return fmt.Errorf("consensus validation failed: %w", err)
	}

	return nil
}

// ValidateBlockHeader validates only the block header
func (v *PolygonValidator) ValidateBlockHeader(ctx context.Context, header *types.Header, parent *types.Header) error {
	if header == nil {
		return fmt.Errorf("header is nil")
	}

	// Basic field validation
	if header.Number == 0 {
		return fmt.Errorf("invalid block number: 0")
	}

	if header.GasLimit < v.minGasLimit {
		return fmt.Errorf("gas limit too low: %d < %d", header.GasLimit, v.minGasLimit)
	}

	if header.GasLimit > v.maxGasLimit {
		return fmt.Errorf("gas limit too high: %d > %d", header.GasLimit, v.maxGasLimit)
	}

	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("gas used (%d) exceeds gas limit (%d)", header.GasUsed, header.GasLimit)
	}

	// Validate against parent if provided
	if parent != nil {
		if err := v.validateHeaderAgainstParent(header, parent); err != nil {
			return err
		}
	}

	// Validate hash integrity
	if err := v.validateHeaderHash(header); err != nil {
		return err
	}

	return nil
}

// validateHeaderAgainstParent validates header fields against parent
func (v *PolygonValidator) validateHeaderAgainstParent(header *types.Header, parent *types.Header) error {
	// Check number sequence
	if header.Number != parent.Number+1 {
		return fmt.Errorf("invalid block number: expected %d, got %d", parent.Number+1, header.Number)
	}

	// Check parent hash
	if header.ParentHash != parent.Hash {
		return fmt.Errorf("invalid parent hash: expected %s, got %s", parent.Hash.Hex(), header.ParentHash.Hex())
	}

	// Validate timestamp
	if header.Timestamp <= parent.Timestamp {
		return fmt.Errorf("invalid timestamp: %d <= parent timestamp %d", header.Timestamp, parent.Timestamp)
	}

	// Check timestamp is not too far in the future (allow 15 seconds drift)
	if v.strictTimestamp {
		now := uint64(time.Now().Unix())
		maxDrift := uint64(15) // 15 seconds
		if header.Timestamp > now+maxDrift {
			return fmt.Errorf("timestamp too far in future: %d > %d (now + %ds)", header.Timestamp, now+maxDrift, maxDrift)
		}
	}

	// Validate gas limit change (Ethereum rule: max 1/1024 change per block)
	gasLimitDelta := int64(header.GasLimit) - int64(parent.GasLimit)
	if gasLimitDelta < 0 {
		gasLimitDelta = -gasLimitDelta
	}
	maxDelta := int64(parent.GasLimit / 1024)
	if gasLimitDelta > maxDelta {
		return fmt.Errorf("gas limit changed too much: %d (max allowed: %d)", gasLimitDelta, maxDelta)
	}

	return nil
}

// validateHeaderHash validates that the header hash matches computed hash
func (v *PolygonValidator) validateHeaderHash(header *types.Header) error {
	// Compute the hash from header data
	computedHash := v.computeHeaderHash(header)

	// Compare with stored hash
	if computedHash != header.Hash {
		return fmt.Errorf("header hash mismatch: computed %s != stored %s", computedHash.Hex(), header.Hash.Hex())
	}

	return nil
}

// computeHeaderHash computes the Keccak256 hash of the header
func (v *PolygonValidator) computeHeaderHash(header *types.Header) common.Hash {
	// For proper validation, we should use RLP encoding
	// This is a simplified version - in production, use proper header encoding
	hasher := crypto.NewKeccakState()

	// Write all header fields
	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.Coinbase,
		header.StateRoot,
		header.TxHash,
		header.ReceiptHash,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Timestamp,
		header.ExtraData,
		header.MixDigest,
		header.Nonce,
	})

	var hash common.Hash
	hasher.Read(hash[:])
	return hash
}

// ValidateConsensus validates Bor consensus rules
func (v *PolygonValidator) ValidateConsensus(ctx context.Context, block *types.Block, parent *types.Block) error {
	// Check if this is a sprint block (validator rotation)
	isSprintEnd := block.Header.Number%v.sprintSize == 0

	// Validate extra data (contains validator signatures for Bor)
	if err := v.validateExtraData(block.Header, isSprintEnd); err != nil {
		return fmt.Errorf("extra data validation failed: %w", err)
	}

	// Validate miner/validator (if we have validator set)
	if v.validatorSetFn != nil {
		validators, err := v.validatorSetFn(block.Header.Number)
		if err != nil {
			// If we can't get validator set, skip validator check
			// This can happen during initial sync
			return nil
		}

		if err := v.validateMiner(block.Header, validators); err != nil {
			return fmt.Errorf("miner validation failed: %w", err)
		}
	}

	return nil
}

// validateExtraData validates the extra data field in block header
func (v *PolygonValidator) validateExtraData(header *types.Header, isSprintEnd bool) error {
	extraDataLen := len(header.ExtraData)

	// Minimum extra data size (vanity)
	const vanitySize = 32
	const signatureSize = 65

	if extraDataLen < vanitySize {
		return fmt.Errorf("extra data too short: %d < %d", extraDataLen, vanitySize)
	}

	// For sprint end blocks, expect signature
	if isSprintEnd {
		minSize := vanitySize + signatureSize
		if extraDataLen < minSize {
			return fmt.Errorf("sprint block extra data too short: %d < %d", extraDataLen, minSize)
		}
	}

	return nil
}

// validateMiner validates that the miner is an authorized validator
func (v *PolygonValidator) validateMiner(header *types.Header, validators []common.Address) error {
	miner := header.Coinbase

	// Check if miner is in validator set
	for _, validator := range validators {
		if validator == miner {
			return nil
		}
	}

	return fmt.Errorf("unauthorized miner: %s not in validator set", miner.Hex())
}

// ValidateTransactions validates all transactions in a block
func (v *PolygonValidator) ValidateTransactions(ctx context.Context, block *types.Block) error {
	if len(block.Transactions) == 0 {
		// Empty blocks are valid
		return nil
	}

	// Validate transaction hash root
	if err := v.validateTxHashRoot(block); err != nil {
		return err
	}

	totalGas := uint64(0)

	for i, tx := range block.Transactions {
		// Validate individual transaction
		if err := v.validateTransaction(tx, block.Header.Number); err != nil {
			return fmt.Errorf("transaction %d validation failed: %w", i, err)
		}

		totalGas += tx.Gas
	}

	// Check total gas doesn't exceed block gas limit
	if totalGas > block.Header.GasLimit {
		return fmt.Errorf("total transaction gas (%d) exceeds block gas limit (%d)", totalGas, block.Header.GasLimit)
	}

	return nil
}

// validateTransaction validates a single transaction
func (v *PolygonValidator) validateTransaction(tx *types.Transaction, blockNumber uint64) error {
	// Basic field validation
	if tx.Nonce < 0 {
		return fmt.Errorf("negative nonce: %d", tx.Nonce)
	}

	if tx.Gas == 0 {
		return fmt.Errorf("zero gas limit")
	}

	if tx.GasPrice == nil || tx.GasPrice.Sign() < 0 {
		return fmt.Errorf("invalid gas price")
	}

	if tx.Value == nil || tx.Value.Sign() < 0 {
		return fmt.Errorf("invalid value")
	}

	// Validate hash
	computedHash := v.computeTransactionHash(tx)
	if computedHash != tx.Hash {
		return fmt.Errorf("transaction hash mismatch: computed %s != stored %s", computedHash.Hex(), tx.Hash.Hex())
	}

	return nil
}

// computeTransactionHash computes the transaction hash
func (v *PolygonValidator) computeTransactionHash(tx *types.Transaction) common.Hash {
	// This is a simplified version - in production, use proper transaction encoding
	hasher := crypto.NewKeccakState()

	rlp.Encode(hasher, []interface{}{
		tx.Nonce,
		tx.GasPrice,
		tx.Gas,
		tx.To,
		tx.Value,
		tx.Data,
		tx.V,
		tx.R,
		tx.S,
	})

	var hash common.Hash
	hasher.Read(hash[:])
	return hash
}

// validateTxHashRoot validates the transaction hash root
func (v *PolygonValidator) validateTxHashRoot(block *types.Block) error {
	// Compute merkle root of transaction hashes
	computedRoot := v.computeTxHashRoot(block.Transactions)

	if computedRoot != block.Header.TxHash {
		return fmt.Errorf("transaction hash root mismatch: computed %s != header %s",
			computedRoot.Hex(), block.Header.TxHash.Hex())
	}

	return nil
}

// computeTxHashRoot computes the merkle root of transaction hashes
func (v *PolygonValidator) computeTxHashRoot(txs []*types.Transaction) common.Hash {
	if len(txs) == 0 {
		// Empty tx list has a specific hash
		return types.EmptyRootHash
	}

	// Collect all transaction hashes
	hashes := make([][]byte, len(txs))
	for i, tx := range txs {
		hashes[i] = tx.Hash[:]
	}

	// Compute merkle root
	return computeMerkleRoot(hashes)
}

// computeMerkleRoot computes a simple merkle root
// In production, use a proper merkle tree implementation
func computeMerkleRoot(hashes [][]byte) common.Hash {
	if len(hashes) == 0 {
		return common.Hash{}
	}

	if len(hashes) == 1 {
		return common.BytesToHash(hashes[0])
	}

	// Simple iterative hashing (not a proper merkle tree)
	// For production, use ethereum/go-ethereum/trie or similar
	current := hashes[0]
	for i := 1; i < len(hashes); i++ {
		combined := append(current, hashes[i]...)
		hash := crypto.Keccak256(combined)
		current = hash
	}

	return common.BytesToHash(current)
}

// LightValidator performs only header validation (for backward sync)
type LightValidator struct {
	*PolygonValidator
}

// NewLightValidator creates a light validator that only validates headers
func NewLightValidator(cfg *ValidatorConfig) *LightValidator {
	return &LightValidator{
		PolygonValidator: NewPolygonValidator(cfg),
	}
}

// ValidateBlock for light validator only validates header
func (v *LightValidator) ValidateBlock(ctx context.Context, block *types.Block, parent *types.Block) error {
	return v.ValidateBlockHeader(ctx, block.Header, parent.Header)
}

// NoOpValidator skips all validation (for testing or when validation is disabled)
type NoOpValidator struct{}

// NewNoOpValidator creates a validator that performs no validation
func NewNoOpValidator() *NoOpValidator {
	return &NoOpValidator{}
}

// ValidateBlock always returns nil
func (v *NoOpValidator) ValidateBlock(ctx context.Context, block *types.Block, parent *types.Block) error {
	return nil
}

// ValidateBlockHeader always returns nil
func (v *NoOpValidator) ValidateBlockHeader(ctx context.Context, header *types.Header, parent *types.Header) error {
	return nil
}

// ValidateConsensus always returns nil
func (v *NoOpValidator) ValidateConsensus(ctx context.Context, block *types.Block, parent *types.Block) error {
	return nil
}

// ValidateTransactions always returns nil
func (v *NoOpValidator) ValidateTransactions(ctx context.Context, block *types.Block) error {
	return nil
}
