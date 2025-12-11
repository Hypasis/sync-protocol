package validation

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hypasis/sync-protocol/internal/types"
)

// ChainConfig holds chain-specific validation rules
type ChainConfig struct {
	ChainID         *big.Int
	ChainName       string
	BlockPeriod     uint64 // seconds between blocks
	SprintSize      uint64 // blocks per sprint
	MinGasLimit     uint64
	MaxGasLimit     uint64
	MaxCodeSize     uint64 // maximum bytecode size
	MaxTxSize       uint64 // maximum transaction size
	MaxExtraDataLen uint64 // maximum extra data length
}

// Polygon mainnet configuration
var PolygonMainnetConfig = &ChainConfig{
	ChainID:         big.NewInt(137),
	ChainName:       "polygon-mainnet",
	BlockPeriod:     2,  // 2 seconds
	SprintSize:      16, // 16 blocks per sprint
	MinGasLimit:     5000,
	MaxGasLimit:     30000000,
	MaxCodeSize:     24576,  // 24 KB
	MaxTxSize:       131072, // 128 KB
	MaxExtraDataLen: 65,     // 32 bytes vanity + 65 bytes signature
}

// Polygon Amoy testnet configuration
var PolygonAmoyConfig = &ChainConfig{
	ChainID:         big.NewInt(80002),
	ChainName:       "polygon-amoy",
	BlockPeriod:     2,
	SprintSize:      16,
	MinGasLimit:     5000,
	MaxGasLimit:     30000000,
	MaxCodeSize:     24576,
	MaxTxSize:       131072,
	MaxExtraDataLen: 65,
}

// Polygon Mumbai testnet configuration (deprecated but included for reference)
var PolygonMumbaiConfig = &ChainConfig{
	ChainID:         big.NewInt(80001),
	ChainName:       "polygon-mumbai",
	BlockPeriod:     2,
	SprintSize:      16,
	MinGasLimit:     5000,
	MaxGasLimit:     30000000,
	MaxCodeSize:     24576,
	MaxTxSize:       131072,
	MaxExtraDataLen: 65,
}

// GetChainConfig returns the config for a given chain ID
func GetChainConfig(chainID *big.Int) *ChainConfig {
	switch chainID.Int64() {
	case 137:
		return PolygonMainnetConfig
	case 80002:
		return PolygonAmoyConfig
	case 80001:
		return PolygonMumbaiConfig
	default:
		// Return mainnet config as default
		return PolygonMainnetConfig
	}
}

// BorConsensusRules defines Bor consensus validation rules
type BorConsensusRules struct {
	config *ChainConfig
}

// NewBorConsensusRules creates new Bor consensus rules
func NewBorConsensusRules(cfg *ChainConfig) *BorConsensusRules {
	return &BorConsensusRules{
		config: cfg,
	}
}

// IsSprintEnd checks if a block is the end of a sprint
func (r *BorConsensusRules) IsSprintEnd(blockNumber uint64) bool {
	return blockNumber%r.config.SprintSize == 0
}

// IsSprintStart checks if a block is the start of a sprint
func (r *BorConsensusRules) IsSprintStart(blockNumber uint64) bool {
	return blockNumber%r.config.SprintSize == 1
}

// GetSprintNumber returns the sprint number for a block
func (r *BorConsensusRules) GetSprintNumber(blockNumber uint64) uint64 {
	return blockNumber / r.config.SprintSize
}

// ValidateBlockPeriod validates that block timestamp follows block period rules
func (r *BorConsensusRules) ValidateBlockPeriod(header, parent *types.Header) bool {
	timeDiff := header.Timestamp - parent.Timestamp
	// Allow some flexibility: between blockPeriod/2 and blockPeriod*2
	minPeriod := r.config.BlockPeriod / 2
	maxPeriod := r.config.BlockPeriod * 3
	return timeDiff >= minPeriod && timeDiff <= maxPeriod
}

// ValidateGasLimit validates gas limit is within bounds
func (r *BorConsensusRules) ValidateGasLimit(gasLimit uint64) bool {
	return gasLimit >= r.config.MinGasLimit && gasLimit <= r.config.MaxGasLimit
}

// ValidateExtraDataLength validates extra data length
func (r *BorConsensusRules) ValidateExtraDataLength(extraData []byte) bool {
	return uint64(len(extraData)) <= r.config.MaxExtraDataLen
}

// Bor-specific constants
const (
	// Extra data
	ExtraVanityLength = 32 // Fixed number of extra-data prefix bytes reserved for signer vanity
	ExtraSealLength   = 65 // Fixed number of extra-data suffix bytes reserved for signer seal

	// Difficulty
	DiffInTurn    = 2 // Block difficulty for in-turn signatures
	DiffNoTurn    = 1 // Block difficulty for out-of-turn signatures
	DifficultyBig = 0 // Default difficulty

	// Validator selection
	WiggleTime = 500 // Random delay (per signer) to allow concurrent signers (milliseconds)
)

// CalcProducerDelay calculates the delay a producer should wait before sealing
func CalcProducerDelay(blockNumber uint64, succession int) uint64 {
	// In-turn validator has no delay, out-of-turn validators wait
	delay := uint64(succession) * WiggleTime
	return delay
}

// IsDifficultyValid checks if block difficulty is valid
func IsDifficultyValid(difficulty uint64, isInTurn bool) bool {
	if isInTurn {
		return difficulty == DiffInTurn
	}
	return difficulty == DiffNoTurn
}

// GenesisValidators returns the genesis validator set
// In production, this would be loaded from chain config
var GenesisValidators = []common.Address{
	common.HexToAddress("0x0000000000000000000000000000000000000000"),
}

// ValidationLevel defines the level of validation to perform
type ValidationLevel int

const (
	// ValidationNone skips all validation
	ValidationNone ValidationLevel = iota

	// ValidationHeader validates only block headers
	ValidationHeader

	// ValidationLight validates headers + basic transaction checks
	ValidationLight

	// ValidationFull performs complete validation
	ValidationFull
)

// String returns string representation of validation level
func (v ValidationLevel) String() string {
	switch v {
	case ValidationNone:
		return "none"
	case ValidationHeader:
		return "header"
	case ValidationLight:
		return "light"
	case ValidationFull:
		return "full"
	default:
		return "unknown"
	}
}

// ParseValidationLevel parses a string into ValidationLevel
func ParseValidationLevel(s string) ValidationLevel {
	switch s {
	case "none":
		return ValidationNone
	case "header":
		return ValidationHeader
	case "light":
		return ValidationLight
	case "full":
		return ValidationFull
	default:
		return ValidationFull // Default to full validation
	}
}

// BlockReward calculates the block reward for Polygon
// Polygon doesn't have block rewards in the traditional sense,
// but this can be used for accounting purposes
func BlockReward(blockNumber uint64) *big.Int {
	// Polygon doesn't have mining rewards, returns 0
	return big.NewInt(0)
}

// TransactionFee calculates the fee for a transaction
func TransactionFee(gasUsed uint64, gasPrice *big.Int) *big.Int {
	fee := new(big.Int).Mul(big.NewInt(int64(gasUsed)), gasPrice)
	return fee
}

// IsEIP1559Compatible checks if a block number is after EIP-1559 activation
// For Polygon, EIP-1559 was activated at a specific block
func IsEIP1559Compatible(chainID *big.Int, blockNumber uint64) bool {
	// Polygon mainnet: EIP-1559 activated at block 23850000
	// Polygon Mumbai: EIP-1559 activated at block 29368000
	if chainID.Int64() == 137 {
		return blockNumber >= 23850000
	}
	if chainID.Int64() == 80001 {
		return blockNumber >= 29368000
	}
	// Amoy testnet has EIP-1559 from genesis
	if chainID.Int64() == 80002 {
		return true
	}
	return false
}

// ValidateEIP1559Header validates EIP-1559 specific fields
func ValidateEIP1559Header(header *types.Header, parent *types.Header) error {
	// EIP-1559 validation would go here
	// For now, this is a placeholder
	return nil
}

// BorForkRules defines fork-specific rules for Bor
type BorForkRules struct {
	chainID *big.Int
}

// NewBorForkRules creates new Bor fork rules
func NewBorForkRules(chainID *big.Int) *BorForkRules {
	return &BorForkRules{
		chainID: chainID,
	}
}

// IsHomestead checks if we're past Homestead fork
func (f *BorForkRules) IsHomestead(blockNumber uint64) bool {
	// All Polygon blocks are post-Homestead
	return true
}

// IsEIP150 checks if we're past EIP-150 fork
func (f *BorForkRules) IsEIP150(blockNumber uint64) bool {
	// All Polygon blocks have EIP-150
	return true
}

// IsEIP155 checks if we're past EIP-155 fork (replay protection)
func (f *BorForkRules) IsEIP155(blockNumber uint64) bool {
	// All Polygon blocks have EIP-155
	return true
}

// IsEIP158 checks if we're past EIP-158 fork (state trie clearing)
func (f *BorForkRules) IsEIP158(blockNumber uint64) bool {
	// All Polygon blocks have EIP-158
	return true
}

// IsByzantium checks if we're past Byzantium fork
func (f *BorForkRules) IsByzantium(blockNumber uint64) bool {
	// All Polygon blocks are post-Byzantium
	return true
}

// IsConstantinople checks if we're past Constantinople fork
func (f *BorForkRules) IsConstantinople(blockNumber uint64) bool {
	// All Polygon blocks are post-Constantinople
	return true
}

// IsIstanbul checks if we're past Istanbul fork
func (f *BorForkRules) IsIstanbul(blockNumber uint64) bool {
	// All Polygon blocks are post-Istanbul
	return true
}

// IsBerlin checks if we're past Berlin fork
func (f *BorForkRules) IsBerlin(blockNumber uint64) bool {
	// All Polygon blocks are post-Berlin
	return true
}

// IsLondon checks if we're past London fork (EIP-1559)
func (f *BorForkRules) IsLondon(blockNumber uint64) bool {
	return IsEIP1559Compatible(f.chainID, blockNumber)
}
