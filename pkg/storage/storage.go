package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hypasis/sync-protocol/internal/types"
	"github.com/hypasis/sync-protocol/pkg/config"
)

// Storage interface for blockchain data storage
type Storage interface {
	// Block operations
	StoreBlock(ctx context.Context, block *types.Block) error
	GetBlock(ctx context.Context, number uint64) (*types.Block, error)
	GetBlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error)
	HasBlock(ctx context.Context, number uint64) bool

	// Batch operations
	StoreBlocks(ctx context.Context, blocks []*types.Block) error
	GetBlocks(ctx context.Context, start, end uint64) ([]*types.Block, error)

	// Gap tracking
	GetGaps() []types.BlockRange
	GetAvailableRanges() []types.BlockRange
	GetNextGap() *types.BlockRange

	// Metadata
	GetMinBlock() uint64
	GetMaxBlock() uint64
	GetBlockCount() uint64

	// Lifecycle
	Close() error
}

// NewStorage creates a new storage instance based on configuration
func NewStorage(cfg *config.StorageConfig) (Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("storage config is nil")
	}

	engine := strings.ToLower(cfg.Engine)

	switch engine {
	case "pebble", "pebbledb":
		// Parse PebbleDB options from config
		opts := &PebbleOptions{
			CacheSize:       parseSizeString(cfg.CacheSize, 2*1024*1024*1024),       // default 2GB
			WriteBufferSize: int(parseSizeString(cfg.WriteBufferSize, 256*1024*1024)), // default 256MB
			MaxOpenFiles:    cfg.MaxOpenFiles,
		}
		if opts.MaxOpenFiles == 0 {
			opts.MaxOpenFiles = 1024 // default
		}

		return NewPebbleStorage(cfg.DataDir, opts)

	case "memory":
		return NewMemoryStorage(), nil

	default:
		return nil, fmt.Errorf("unsupported storage engine: %s (supported: pebble, memory)", engine)
	}
}

// parseSizeString parses size strings like "2GB", "256MB" into bytes
func parseSizeString(s string, defaultValue int64) int64 {
	if s == "" {
		return defaultValue
	}

	s = strings.TrimSpace(strings.ToUpper(s))

	// Extract number and unit
	var value int64
	var unit string

	fmt.Sscanf(s, "%d%s", &value, &unit)

	switch unit {
	case "GB", "G":
		return value * 1024 * 1024 * 1024
	case "MB", "M":
		return value * 1024 * 1024
	case "KB", "K":
		return value * 1024
	case "B", "":
		return value
	default:
		return defaultValue
	}
}

// MemoryStorage is an in-memory implementation (for testing/development)
type MemoryStorage struct {
	blocks      map[uint64]*types.Block
	blockHashes map[common.Hash]*types.Block
	gapTracker  *GapTracker
	mu          sync.RWMutex
}

// NewMemoryStorage creates a new in-memory storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		blocks:      make(map[uint64]*types.Block),
		blockHashes: make(map[common.Hash]*types.Block),
		gapTracker:  NewGapTracker(),
	}
}

// StoreBlock stores a single block
func (s *MemoryStorage) StoreBlock(ctx context.Context, block *types.Block) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.blocks[block.Header.Number] = block
	s.blockHashes[block.Header.Hash] = block
	s.gapTracker.MarkAvailable(block.Header.Number, block.Header.Number+1)

	return nil
}

// GetBlock retrieves a block by number
func (s *MemoryStorage) GetBlock(ctx context.Context, number uint64) (*types.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	block, ok := s.blocks[number]
	if !ok {
		return nil, fmt.Errorf("block %d not found", number)
	}

	return block, nil
}

// GetBlockByHash retrieves a block by hash
func (s *MemoryStorage) GetBlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	block, ok := s.blockHashes[hash]
	if !ok {
		return nil, fmt.Errorf("block with hash %s not found", hash.Hex())
	}

	return block, nil
}

// HasBlock checks if a block exists
func (s *MemoryStorage) HasBlock(ctx context.Context, number uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.blocks[number]
	return ok
}

// StoreBlocks stores multiple blocks
func (s *MemoryStorage) StoreBlocks(ctx context.Context, blocks []*types.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, block := range blocks {
		if block == nil {
			continue
		}

		s.blocks[block.Header.Number] = block
		s.blockHashes[block.Header.Hash] = block
	}

	// Update gap tracker with the range
	if len(blocks) > 0 {
		start := blocks[0].Header.Number
		end := blocks[len(blocks)-1].Header.Number + 1
		s.gapTracker.MarkAvailable(start, end)
	}

	return nil
}

// GetBlocks retrieves blocks in a range
func (s *MemoryStorage) GetBlocks(ctx context.Context, start, end uint64) ([]*types.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	blocks := make([]*types.Block, 0, end-start)

	for i := start; i < end; i++ {
		if block, ok := s.blocks[i]; ok {
			blocks = append(blocks, block)
		}
	}

	return blocks, nil
}

// GetGaps returns all missing block ranges
func (s *MemoryStorage) GetGaps() []types.BlockRange {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gapTracker.GetGaps()
}

// GetAvailableRanges returns all available block ranges
func (s *MemoryStorage) GetAvailableRanges() []types.BlockRange {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gapTracker.GetAvailableRanges()
}

// GetNextGap returns the next gap to fill
func (s *MemoryStorage) GetNextGap() *types.BlockRange {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gapTracker.GetNextGap()
}

// GetMinBlock returns the minimum block number
func (s *MemoryStorage) GetMinBlock() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gapTracker.GetMinBlock()
}

// GetMaxBlock returns the maximum block number
func (s *MemoryStorage) GetMaxBlock() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gapTracker.GetMaxBlock()
}

// GetBlockCount returns the total number of stored blocks
func (s *MemoryStorage) GetBlockCount() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return uint64(len(s.blocks))
}

// Close closes the storage
func (s *MemoryStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.blocks = nil
	s.blockHashes = nil

	return nil
}
