package types

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Checkpoint represents a trusted blockchain checkpoint
type Checkpoint struct {
	BlockNumber   uint64      `json:"block_number"`
	BlockHash     common.Hash `json:"block_hash"`
	StateRoot     common.Hash `json:"state_root"`
	Timestamp     uint64      `json:"timestamp"`
	ValidatorSigs []Signature `json:"validator_signatures"`
	ChainID       *big.Int    `json:"chain_id"`
}

// Signature represents a validator signature
type Signature struct {
	ValidatorID string `json:"validator_id"`
	PubKey      []byte `json:"pubkey"`
	Signature   []byte `json:"signature"`
}

// BlockRange represents a continuous range of blocks
type BlockRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// SyncStatus represents the current synchronization status
type SyncStatus struct {
	ForwardSync  ForwardSyncStatus  `json:"forward_sync"`
	BackwardSync BackwardSyncStatus `json:"backward_sync"`
	ValidatorReady bool             `json:"validator_ready"`
	StartTime      time.Time         `json:"start_time"`
	Uptime         time.Duration     `json:"uptime"`
}

// ForwardSyncStatus tracks forward synchronization progress
type ForwardSyncStatus struct {
	CurrentBlock uint64  `json:"current_block"`
	TargetBlock  uint64  `json:"target_block"`
	Progress     float64 `json:"progress"` // 0-100
	Syncing      bool    `json:"syncing"`
	BlocksPerSec float64 `json:"blocks_per_sec"`
}

// BackwardSyncStatus tracks backward synchronization progress
type BackwardSyncStatus struct {
	DownloadedRanges []BlockRange `json:"downloaded_ranges"`
	Progress         float64      `json:"progress"` // 0-100
	Syncing          bool         `json:"syncing"`
	BlocksPerSec     float64      `json:"blocks_per_sec"`
	TotalBlocks      uint64       `json:"total_blocks"`
	DownloadedBlocks uint64       `json:"downloaded_blocks"`
}

// PeerInfo represents information about a P2P peer
type PeerInfo struct {
	ID             string       `json:"id"`
	Address        string       `json:"address"`
	Capabilities   PeerCaps     `json:"capabilities"`
	Reputation     float64      `json:"reputation"`
	Connected      bool         `json:"connected"`
	LastSeen       time.Time    `json:"last_seen"`
}

// PeerCaps represents peer capabilities
type PeerCaps struct {
	HistoricalData bool       `json:"historical_data"`
	FullNode       bool       `json:"full_node"`
	ArchiveNode    bool       `json:"archive_node"`
	BlockRange     BlockRange `json:"block_range"`
	ProtocolVersion string    `json:"protocol_version"`
}

// Header represents a block header
type Header struct {
	Number      uint64      `json:"number"`
	Hash        common.Hash `json:"hash"`
	ParentHash  common.Hash `json:"parent_hash"`
	Coinbase    common.Address `json:"coinbase"`    // Miner/validator address
	StateRoot   common.Hash `json:"state_root"`
	TxHash      common.Hash `json:"tx_hash"`       // Transaction root hash
	ReceiptHash common.Hash `json:"receipt_hash"`  // Receipt root hash
	GasLimit    uint64      `json:"gas_limit"`
	GasUsed     uint64      `json:"gas_used"`
	Timestamp   uint64      `json:"timestamp"`
	ExtraData   []byte      `json:"extra_data"`
	MixDigest   common.Hash `json:"mix_digest"`
	Nonce       uint64      `json:"nonce"`
}

// Transaction represents a blockchain transaction
type Transaction struct {
	Hash     common.Hash    `json:"hash"`
	Nonce    uint64         `json:"nonce"` // Changed from int64 to uint64 for RLP compatibility
	From     common.Address `json:"from"`
	To       common.Address `json:"to"`
	Value    *big.Int       `json:"value"`
	Gas      uint64         `json:"gas"`
	GasPrice *big.Int       `json:"gas_price"`
	Data     []byte         `json:"data"`
	V        *big.Int       `json:"v"`
	R        *big.Int       `json:"r"`
	S        *big.Int       `json:"s"`
}

// Block represents a blockchain block
type Block struct {
	Header       *Header        `json:"header"`
	Transactions []*Transaction `json:"transactions"`
	Size         uint64         `json:"size"`
}

// EmptyRootHash is the hash of an empty trie
var EmptyRootHash = common.HexToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b47e18b2452b1162f28c3b61c899469")

// Validator represents a blockchain validator
type Validator struct {
	ID      string      `json:"id"`
	Address common.Address `json:"address"`
	PubKey  []byte      `json:"pubkey"`
	Stake   *big.Int    `json:"stake"`
	Active  bool        `json:"active"`
}

// ChainConfig represents blockchain-specific configuration
type ChainConfig struct {
	Name              string `json:"name"`
	ChainID           *big.Int `json:"chain_id"`
	CheckpointSource  string `json:"checkpoint_source"`
	CheckpointContract common.Address `json:"checkpoint_contract,omitempty"`
	ValidatorSetSize  int    `json:"validator_set_size"`
	BlockTime         uint64 `json:"block_time"` // seconds
}

// GapInfo represents missing block ranges
type GapInfo struct {
	MissingRanges []BlockRange `json:"missing_ranges"`
	TotalGap      uint64       `json:"total_gap"`
	LargestGap    BlockRange   `json:"largest_gap"`
}
