package storage

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/hypasis/sync-protocol/internal/types"
)

// PebbleStorage implements the Storage interface using PebbleDB
type PebbleStorage struct {
	db         *pebble.DB
	gapTracker *GapTracker
	mu         sync.RWMutex
}

// Key prefixes for different data types
const (
	prefixBlockByNumber = "block:num:"
	prefixBlockByHash   = "block:hash:"
	prefixMetaMin       = "meta:min"
	prefixMetaMax       = "meta:max"
	prefixMetaCount     = "meta:count"
	prefixGapRanges     = "gaps:ranges"
)

// PebbleOptions configures PebbleDB
type PebbleOptions struct {
	CacheSize       int64 // Cache size in bytes (default: 2GB)
	WriteBufferSize int   // Write buffer size in bytes (default: 256MB)
	MaxOpenFiles    int   // Max open files (default: 1024)
}

// DefaultPebbleOptions returns default configuration
func DefaultPebbleOptions() *PebbleOptions {
	return &PebbleOptions{
		CacheSize:       2 * 1024 * 1024 * 1024, // 2GB
		WriteBufferSize: 256 * 1024 * 1024,      // 256MB
		MaxOpenFiles:    1024,
	}
}

// NewPebbleStorage creates a new PebbleDB-backed storage
func NewPebbleStorage(dataDir string, opts *PebbleOptions) (*PebbleStorage, error) {
	if opts == nil {
		opts = DefaultPebbleOptions()
	}

	// Configure PebbleDB
	dbOpts := &pebble.Options{
		Cache:                       pebble.NewCache(opts.CacheSize),
		MemTableSize:                uint64(opts.WriteBufferSize),
		MaxOpenFiles:                opts.MaxOpenFiles,
		DisableWAL:                  false, // Enable WAL for durability
		BytesPerSync:                1 << 20, // 1MB
		L0CompactionThreshold:       2,
		L0StopWritesThreshold:       12,
		LBaseMaxBytes:               64 << 20, // 64MB
		MaxConcurrentCompactions:    func() int { return 4 },
		Merger: &pebble.Merger{
			Name: "noop",
		},
	}

	// Open database
	db, err := pebble.Open(dataDir, dbOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open pebble db: %w", err)
	}

	storage := &PebbleStorage{
		db:         db,
		gapTracker: NewGapTracker(),
	}

	// Load gap tracker state from database
	if err := storage.loadGapTracker(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load gap tracker: %w", err)
	}

	return storage, nil
}

// StoreBlock stores a single block
func (s *PebbleStorage) StoreBlock(ctx context.Context, block *types.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.storeBlockInternal(block)
}

// storeBlockInternal stores a block without locking (internal use)
func (s *PebbleStorage) storeBlockInternal(block *types.Block) error {
	// Serialize block using RLP
	data, err := rlp.EncodeToBytes(block)
	if err != nil {
		return fmt.Errorf("failed to encode block: %w", err)
	}

	// Create batch for atomic operations
	batch := s.db.NewBatch()
	defer batch.Close()

	// Store block by number
	numKey := makeBlockNumberKey(block.Header.Number)
	if err := batch.Set(numKey, data, pebble.Sync); err != nil {
		return fmt.Errorf("failed to set block by number: %w", err)
	}

	// Store block hash -> number mapping
	hashKey := makeBlockHashKey(block.Header.Hash)
	numBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(numBytes, block.Header.Number)
	if err := batch.Set(hashKey, numBytes, pebble.Sync); err != nil {
		return fmt.Errorf("failed to set block hash mapping: %w", err)
	}

	// Update metadata
	if err := s.updateMetadata(batch, block.Header.Number); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	// Commit batch
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	// Update gap tracker
	s.gapTracker.MarkAvailable(block.Header.Number, block.Header.Number+1)

	// Persist gap tracker
	if err := s.saveGapTracker(); err != nil {
		return fmt.Errorf("failed to save gap tracker: %w", err)
	}

	return nil
}

// StoreBlocks stores multiple blocks in a batch
func (s *PebbleStorage) StoreBlocks(ctx context.Context, blocks []*types.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(blocks) == 0 {
		return nil
	}

	// Create batch for all blocks
	batch := s.db.NewBatch()
	defer batch.Close()

	// Track min/max for batch metadata update
	minBlock := blocks[0].Header.Number
	maxBlock := blocks[0].Header.Number

	for _, block := range blocks {
		// Serialize block
		data, err := rlp.EncodeToBytes(block)
		if err != nil {
			return fmt.Errorf("failed to encode block %d: %w", block.Header.Number, err)
		}

		// Store block by number
		numKey := makeBlockNumberKey(block.Header.Number)
		if err := batch.Set(numKey, data, pebble.NoSync); err != nil {
			return fmt.Errorf("failed to set block %d by number: %w", block.Header.Number, err)
		}

		// Store block hash -> number mapping
		hashKey := makeBlockHashKey(block.Header.Hash)
		numBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(numBytes, block.Header.Number)
		if err := batch.Set(hashKey, numBytes, pebble.NoSync); err != nil {
			return fmt.Errorf("failed to set block %d hash mapping: %w", block.Header.Number, err)
		}

		// Track min/max
		if block.Header.Number < minBlock {
			minBlock = block.Header.Number
		}
		if block.Header.Number > maxBlock {
			maxBlock = block.Header.Number
		}

		// Update gap tracker
		s.gapTracker.MarkAvailable(block.Header.Number, block.Header.Number+1)
	}

	// Update metadata once for the entire batch
	if err := s.updateMetadataBatch(batch, minBlock, maxBlock, uint64(len(blocks))); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	// Commit batch with sync
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	// Persist gap tracker once after all blocks
	if err := s.saveGapTracker(); err != nil {
		return fmt.Errorf("failed to save gap tracker: %w", err)
	}

	return nil
}

// GetBlock retrieves a block by number
func (s *PebbleStorage) GetBlock(ctx context.Context, number uint64) (*types.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := makeBlockNumberKey(number)
	data, closer, err := s.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, fmt.Errorf("block %d not found", number)
		}
		return nil, fmt.Errorf("failed to get block: %w", err)
	}
	defer closer.Close()

	var block types.Block
	if err := rlp.DecodeBytes(data, &block); err != nil {
		return nil, fmt.Errorf("failed to decode block: %w", err)
	}

	return &block, nil
}

// GetBlockByHash retrieves a block by hash
func (s *PebbleStorage) GetBlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get block number from hash mapping
	hashKey := makeBlockHashKey(hash)
	numBytes, closer, err := s.db.Get(hashKey)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, fmt.Errorf("block with hash %s not found", hash.Hex())
		}
		return nil, fmt.Errorf("failed to get block number: %w", err)
	}
	closer.Close()

	blockNum := binary.BigEndian.Uint64(numBytes)

	// Get block by number
	return s.GetBlock(ctx, blockNum)
}

// HasBlock checks if a block exists
func (s *PebbleStorage) HasBlock(ctx context.Context, number uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := makeBlockNumberKey(number)
	_, closer, err := s.db.Get(key)
	if err != nil {
		return false
	}
	closer.Close()
	return true
}

// GetBlocks retrieves a range of blocks
func (s *PebbleStorage) GetBlocks(ctx context.Context, start, end uint64) ([]*types.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	blocks := make([]*types.Block, 0, end-start)

	for i := start; i < end; i++ {
		block, err := s.GetBlock(ctx, i)
		if err != nil {
			// Skip missing blocks
			continue
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

// GetGaps returns missing block ranges
func (s *PebbleStorage) GetGaps() []types.BlockRange {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gapTracker.GetGaps()
}

// GetAvailableRanges returns available block ranges
func (s *PebbleStorage) GetAvailableRanges() []types.BlockRange {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gapTracker.GetAvailableRanges()
}

// GetNextGap returns the largest gap
func (s *PebbleStorage) GetNextGap() *types.BlockRange {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gapTracker.GetNextGap()
}

// GetMinBlock returns the minimum block number
func (s *PebbleStorage) GetMinBlock() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, closer, err := s.db.Get([]byte(prefixMetaMin))
	if err != nil {
		return 0
	}
	defer closer.Close()

	return binary.BigEndian.Uint64(data)
}

// GetMaxBlock returns the maximum block number
func (s *PebbleStorage) GetMaxBlock() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, closer, err := s.db.Get([]byte(prefixMetaMax))
	if err != nil {
		return 0
	}
	defer closer.Close()

	return binary.BigEndian.Uint64(data)
}

// GetBlockCount returns the total number of blocks
func (s *PebbleStorage) GetBlockCount() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, closer, err := s.db.Get([]byte(prefixMetaCount))
	if err != nil {
		return 0
	}
	defer closer.Close()

	return binary.BigEndian.Uint64(data)
}

// Close closes the database
func (s *PebbleStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Save gap tracker before closing
	if err := s.saveGapTracker(); err != nil {
		return fmt.Errorf("failed to save gap tracker on close: %w", err)
	}

	return s.db.Close()
}

// updateMetadataBatch updates min, max, and count metadata for a batch of blocks
// Note: This method reads directly from DB to avoid deadlock (caller holds lock)
func (s *PebbleStorage) updateMetadataBatch(batch *pebble.Batch, minBlock, maxBlock, count uint64) error {
	// Update min - read directly from DB without lock
	minBytes := make([]byte, 8)
	data, closer, err := s.db.Get([]byte(prefixMetaMin))
	currentMin := uint64(0)
	if err == nil {
		currentMin = binary.BigEndian.Uint64(data)
		closer.Close()
	}
	if currentMin == 0 || minBlock < currentMin {
		binary.BigEndian.PutUint64(minBytes, minBlock)
		if err := batch.Set([]byte(prefixMetaMin), minBytes, pebble.NoSync); err != nil {
			return err
		}
	}

	// Update max - read directly from DB without lock
	maxBytes := make([]byte, 8)
	data, closer, err = s.db.Get([]byte(prefixMetaMax))
	currentMax := uint64(0)
	if err == nil {
		currentMax = binary.BigEndian.Uint64(data)
		closer.Close()
	}
	if maxBlock > currentMax {
		binary.BigEndian.PutUint64(maxBytes, maxBlock)
		if err := batch.Set([]byte(prefixMetaMax), maxBytes, pebble.NoSync); err != nil {
			return err
		}
	}

	// Update count - read directly from DB without lock
	countBytes := make([]byte, 8)
	data, closer, err = s.db.Get([]byte(prefixMetaCount))
	currentCount := uint64(0)
	if err == nil {
		currentCount = binary.BigEndian.Uint64(data)
		closer.Close()
	}
	binary.BigEndian.PutUint64(countBytes, currentCount+count)
	if err := batch.Set([]byte(prefixMetaCount), countBytes, pebble.NoSync); err != nil {
		return err
	}

	return nil
}

// updateMetadata updates min, max, and count metadata
// Note: This method reads directly from DB to avoid deadlock (caller holds lock)
func (s *PebbleStorage) updateMetadata(batch *pebble.Batch, blockNum uint64) error {
	// Update min - read directly from DB without lock
	minBytes := make([]byte, 8)
	data, closer, err := s.db.Get([]byte(prefixMetaMin))
	currentMin := uint64(0)
	if err == nil {
		currentMin = binary.BigEndian.Uint64(data)
		closer.Close()
	}
	if currentMin == 0 || blockNum < currentMin {
		binary.BigEndian.PutUint64(minBytes, blockNum)
		if err := batch.Set([]byte(prefixMetaMin), minBytes, pebble.NoSync); err != nil {
			return err
		}
	}

	// Update max - read directly from DB without lock
	maxBytes := make([]byte, 8)
	data, closer, err = s.db.Get([]byte(prefixMetaMax))
	currentMax := uint64(0)
	if err == nil {
		currentMax = binary.BigEndian.Uint64(data)
		closer.Close()
	}
	if blockNum > currentMax {
		binary.BigEndian.PutUint64(maxBytes, blockNum)
		if err := batch.Set([]byte(prefixMetaMax), maxBytes, pebble.NoSync); err != nil {
			return err
		}
	}

	// Update count - read directly from DB without lock
	countBytes := make([]byte, 8)
	data, closer, err = s.db.Get([]byte(prefixMetaCount))
	currentCount := uint64(0)
	if err == nil {
		currentCount = binary.BigEndian.Uint64(data)
		closer.Close()
	}
	binary.BigEndian.PutUint64(countBytes, currentCount+1)
	if err := batch.Set([]byte(prefixMetaCount), countBytes, pebble.NoSync); err != nil {
		return err
	}

	return nil
}

// saveGapTracker persists gap tracker state to database
func (s *PebbleStorage) saveGapTracker() error {
	ranges := s.gapTracker.GetAvailableRanges()
	data, err := rlp.EncodeToBytes(ranges)
	if err != nil {
		return fmt.Errorf("failed to encode gap ranges: %w", err)
	}

	if err := s.db.Set([]byte(prefixGapRanges), data, pebble.Sync); err != nil {
		return fmt.Errorf("failed to save gap ranges: %w", err)
	}

	return nil
}

// loadGapTracker loads gap tracker state from database
func (s *PebbleStorage) loadGapTracker() error {
	data, closer, err := s.db.Get([]byte(prefixGapRanges))
	if err != nil {
		if err == pebble.ErrNotFound {
			// No existing gap data, start fresh
			return nil
		}
		return fmt.Errorf("failed to get gap ranges: %w", err)
	}
	defer closer.Close()

	var ranges []types.BlockRange
	if err := rlp.DecodeBytes(data, &ranges); err != nil {
		return fmt.Errorf("failed to decode gap ranges: %w", err)
	}

	// Restore gap tracker state
	for _, r := range ranges {
		s.gapTracker.MarkAvailable(r.Start, r.End)
	}

	return nil
}

// Helper functions for key generation
func makeBlockNumberKey(number uint64) []byte {
	key := make([]byte, len(prefixBlockByNumber)+8)
	copy(key, []byte(prefixBlockByNumber))
	binary.BigEndian.PutUint64(key[len(prefixBlockByNumber):], number)
	return key
}

func makeBlockHashKey(hash common.Hash) []byte {
	key := make([]byte, len(prefixBlockByHash)+len(hash))
	copy(key, []byte(prefixBlockByHash))
	copy(key[len(prefixBlockByHash):], hash[:])
	return key
}
