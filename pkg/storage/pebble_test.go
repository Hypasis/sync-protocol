package storage

import (
	"context"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hypasis/sync-protocol/internal/types"
)

// Helper function to create a temporary test directory
func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "pebble-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir
}

// Helper function to cleanup test directory
func cleanupTempDir(t *testing.T, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		t.Logf("Warning: failed to cleanup temp dir %s: %v", dir, err)
	}
}

// Helper function to create a test block
func createTestBlock(number uint64) *types.Block {
	return &types.Block{
		Header: &types.Header{
			Number:     number,
			Hash:       common.BytesToHash([]byte{byte(number)}),
			ParentHash: common.BytesToHash([]byte{byte(number - 1)}),
			StateRoot:  common.BytesToHash([]byte{byte(number), 0x01}),
			Timestamp:  1000000 + number,
			GasLimit:   8000000,
			GasUsed:    0,
		},
		Transactions: []*types.Transaction{},
		Size:         1024 + number,
	}
}

// TestNewPebbleStorage tests creating a new PebbleDB storage
func TestNewPebbleStorage(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create PebbleDB storage: %v", err)
	}
	defer storage.Close()

	if storage.db == nil {
		t.Error("Database is nil")
	}

	if storage.gapTracker == nil {
		t.Error("Gap tracker is nil")
	}
}

// TestStoreAndGetBlock tests storing and retrieving a single block
func TestStoreAndGetBlock(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	block := createTestBlock(100)

	// Store block
	if err := storage.StoreBlock(ctx, block); err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	// Retrieve block by number
	retrieved, err := storage.GetBlock(ctx, 100)
	if err != nil {
		t.Fatalf("Failed to get block: %v", err)
	}

	// Verify block data
	if retrieved.Header.Number != block.Header.Number {
		t.Errorf("Block number mismatch: got %d, want %d", retrieved.Header.Number, block.Header.Number)
	}
	if retrieved.Header.Hash != block.Header.Hash {
		t.Errorf("Block hash mismatch: got %s, want %s", retrieved.Header.Hash.Hex(), block.Header.Hash.Hex())
	}
	if retrieved.Header.Timestamp != block.Header.Timestamp {
		t.Errorf("Block timestamp mismatch: got %d, want %d", retrieved.Header.Timestamp, block.Header.Timestamp)
	}
}

// TestGetBlockByHash tests retrieving a block by hash
func TestGetBlockByHash(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	block := createTestBlock(200)

	// Store block
	if err := storage.StoreBlock(ctx, block); err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	// Retrieve block by hash
	retrieved, err := storage.GetBlockByHash(ctx, block.Header.Hash)
	if err != nil {
		t.Fatalf("Failed to get block by hash: %v", err)
	}

	if retrieved.Header.Number != block.Header.Number {
		t.Errorf("Block number mismatch: got %d, want %d", retrieved.Header.Number, block.Header.Number)
	}
}

// TestHasBlock tests checking block existence
func TestHasBlock(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	block := createTestBlock(300)

	// Block should not exist initially
	if storage.HasBlock(ctx, 300) {
		t.Error("Block should not exist")
	}

	// Store block
	if err := storage.StoreBlock(ctx, block); err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	// Block should exist now
	if !storage.HasBlock(ctx, 300) {
		t.Error("Block should exist")
	}
}

// TestStoreBlocks tests batch storing blocks
func TestStoreBlocks(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Create multiple blocks
	blocks := make([]*types.Block, 100)
	for i := 0; i < 100; i++ {
		blocks[i] = createTestBlock(uint64(i + 1000))
	}

	// Store blocks in batch
	if err := storage.StoreBlocks(ctx, blocks); err != nil {
		t.Fatalf("Failed to store blocks: %v", err)
	}

	// Verify all blocks are stored
	for i := 0; i < 100; i++ {
		if !storage.HasBlock(ctx, uint64(i+1000)) {
			t.Errorf("Block %d should exist", i+1000)
		}
	}

	// Verify block count
	count := storage.GetBlockCount()
	if count != 100 {
		t.Errorf("Block count mismatch: got %d, want 100", count)
	}
}

// TestGetBlocks tests retrieving a range of blocks
func TestGetBlocks(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Store blocks 2000-2010
	blocks := make([]*types.Block, 10)
	for i := 0; i < 10; i++ {
		blocks[i] = createTestBlock(uint64(i + 2000))
	}
	if err := storage.StoreBlocks(ctx, blocks); err != nil {
		t.Fatalf("Failed to store blocks: %v", err)
	}

	// Retrieve range
	retrieved, err := storage.GetBlocks(ctx, 2000, 2010)
	if err != nil {
		t.Fatalf("Failed to get blocks: %v", err)
	}

	if len(retrieved) != 10 {
		t.Errorf("Retrieved block count mismatch: got %d, want 10", len(retrieved))
	}
}

// TestMetadata tests min, max, and count metadata
func TestMetadata(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Initially should be 0
	if min := storage.GetMinBlock(); min != 0 {
		t.Errorf("Initial min should be 0, got %d", min)
	}

	// Store blocks
	blocks := []*types.Block{
		createTestBlock(5000),
		createTestBlock(5005),
		createTestBlock(4995),
	}
	for _, block := range blocks {
		if err := storage.StoreBlock(ctx, block); err != nil {
			t.Fatalf("Failed to store block: %v", err)
		}
	}

	// Check min (should be 4995)
	if min := storage.GetMinBlock(); min != 4995 {
		t.Errorf("Min block mismatch: got %d, want 4995", min)
	}

	// Check max (should be 5005)
	if max := storage.GetMaxBlock(); max != 5005 {
		t.Errorf("Max block mismatch: got %d, want 5005", max)
	}

	// Check count
	if count := storage.GetBlockCount(); count != 3 {
		t.Errorf("Block count mismatch: got %d, want 3", count)
	}
}

// TestGapTracking tests gap tracking functionality
func TestGapTracking(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Store blocks 0-10 and 20-30 (gap at 10-20)
	blocks1 := make([]*types.Block, 11)
	for i := 0; i <= 10; i++ {
		blocks1[i] = createTestBlock(uint64(i))
	}
	if err := storage.StoreBlocks(ctx, blocks1); err != nil {
		t.Fatalf("Failed to store first batch: %v", err)
	}

	blocks2 := make([]*types.Block, 11)
	for i := 0; i <= 10; i++ {
		blocks2[i] = createTestBlock(uint64(i + 20))
	}
	if err := storage.StoreBlocks(ctx, blocks2); err != nil {
		t.Fatalf("Failed to store second batch: %v", err)
	}

	// Check available ranges
	ranges := storage.GetAvailableRanges()
	if len(ranges) != 2 {
		t.Errorf("Expected 2 available ranges, got %d", len(ranges))
	}

	// Get next gap (should be the largest gap)
	gap := storage.GetNextGap()
	if gap == nil {
		t.Fatal("Expected a gap, got nil")
	}
	if gap.Start != 11 || gap.End != 20 {
		t.Errorf("Gap mismatch: got [%d, %d), want [11, 20)", gap.Start, gap.End)
	}
}

// TestPersistence tests that data persists across reopens
func TestPersistence(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	ctx := context.Background()

	// Create storage and store blocks
	storage1, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create first storage: %v", err)
	}

	blocks := make([]*types.Block, 50)
	for i := 0; i < 50; i++ {
		blocks[i] = createTestBlock(uint64(i + 10000))
	}
	if err := storage1.StoreBlocks(ctx, blocks); err != nil {
		storage1.Close()
		t.Fatalf("Failed to store blocks: %v", err)
	}

	// Close storage
	if err := storage1.Close(); err != nil {
		t.Fatalf("Failed to close storage: %v", err)
	}

	// Reopen storage
	storage2, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to reopen storage: %v", err)
	}
	defer storage2.Close()

	// Verify all blocks are still there
	for i := 0; i < 50; i++ {
		if !storage2.HasBlock(ctx, uint64(i+10000)) {
			t.Errorf("Block %d should exist after reopen", i+10000)
		}
	}

	// Verify metadata
	if count := storage2.GetBlockCount(); count != 50 {
		t.Errorf("Block count after reopen: got %d, want 50", count)
	}

	// Verify gap tracker persisted
	ranges := storage2.GetAvailableRanges()
	if len(ranges) == 0 {
		t.Error("Gap tracker should have available ranges after reopen")
	}
}

// TestConcurrentAccess tests concurrent reads and writes
func TestConcurrentAccess(t *testing.T) {
	dir := createTempDir(t)
	defer cleanupTempDir(t, dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Store initial blocks
	initialBlocks := make([]*types.Block, 100)
	for i := 0; i < 100; i++ {
		initialBlocks[i] = createTestBlock(uint64(i + 20000))
	}
	if err := storage.StoreBlocks(ctx, initialBlocks); err != nil {
		t.Fatalf("Failed to store initial blocks: %v", err)
	}

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(offset int) {
			blocks := make([]*types.Block, 10)
			for j := 0; j < 10; j++ {
				blocks[j] = createTestBlock(uint64(20100 + offset*10 + j))
			}
			if err := storage.StoreBlocks(ctx, blocks); err != nil {
				t.Errorf("Concurrent write failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				storage.GetBlock(ctx, uint64(20000+j))
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
}

// BenchmarkStoreBlock benchmarks single block storage
func BenchmarkStoreBlock(b *testing.B) {
	dir, _ := os.MkdirTemp("", "pebble-bench-*")
	defer os.RemoveAll(dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		block := createTestBlock(uint64(i))
		if err := storage.StoreBlock(ctx, block); err != nil {
			b.Fatalf("Failed to store block: %v", err)
		}
	}
}

// BenchmarkStoreBlocks benchmarks batch block storage
func BenchmarkStoreBlocks(b *testing.B) {
	dir, _ := os.MkdirTemp("", "pebble-bench-*")
	defer os.RemoveAll(dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()
	batchSize := 100

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		blocks := make([]*types.Block, batchSize)
		for j := 0; j < batchSize; j++ {
			blocks[j] = createTestBlock(uint64(i*batchSize + j))
		}
		if err := storage.StoreBlocks(ctx, blocks); err != nil {
			b.Fatalf("Failed to store blocks: %v", err)
		}
	}

	b.ReportMetric(float64(b.N*batchSize)/b.Elapsed().Seconds(), "blocks/sec")
}

// BenchmarkGetBlock benchmarks block retrieval
func BenchmarkGetBlock(b *testing.B) {
	dir, _ := os.MkdirTemp("", "pebble-bench-*")
	defer os.RemoveAll(dir)

	storage, err := NewPebbleStorage(dir, nil)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Pre-populate with blocks
	blocks := make([]*types.Block, 10000)
	for i := 0; i < 10000; i++ {
		blocks[i] = createTestBlock(uint64(i))
	}
	if err := storage.StoreBlocks(ctx, blocks); err != nil {
		b.Fatalf("Failed to populate: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		blockNum := uint64(i % 10000)
		if _, err := storage.GetBlock(ctx, blockNum); err != nil {
			b.Fatalf("Failed to get block: %v", err)
		}
	}
}
