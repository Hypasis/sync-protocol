package checkpoint

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hypasis/sync-protocol/internal/types"
)

// MockSource is a mock checkpoint source for testing
type MockSource struct {
	checkpoints map[uint64]*types.Checkpoint
}

// NewMockSource creates a new mock checkpoint source
func NewMockSource() *MockSource {
	return &MockSource{
		checkpoints: generateMockCheckpoints(),
	}
}

// FetchLatest returns the latest checkpoint
func (s *MockSource) FetchLatest(ctx context.Context) (*types.Checkpoint, error) {
	// Find the highest block number
	var latest *types.Checkpoint
	var maxBlock uint64

	for blockNum, cp := range s.checkpoints {
		if blockNum > maxBlock {
			maxBlock = blockNum
			latest = cp
		}
	}

	if latest == nil {
		return nil, fmt.Errorf("no checkpoints available")
	}

	return latest, nil
}

// FetchByNumber returns a checkpoint at specific block number
func (s *MockSource) FetchByNumber(ctx context.Context, blockNum uint64) (*types.Checkpoint, error) {
	cp, ok := s.checkpoints[blockNum]
	if !ok {
		return nil, fmt.Errorf("checkpoint not found for block %d", blockNum)
	}

	return cp, nil
}

// FetchRange returns checkpoints in a range
func (s *MockSource) FetchRange(ctx context.Context, start, end uint64) ([]*types.Checkpoint, error) {
	var cps []*types.Checkpoint

	for blockNum := start; blockNum <= end; blockNum++ {
		if cp, ok := s.checkpoints[blockNum]; ok {
			cps = append(cps, cp)
		}
	}

	return cps, nil
}

// generateMockCheckpoints generates mock checkpoints for testing
func generateMockCheckpoints() map[uint64]*types.Checkpoint {
	checkpoints := make(map[uint64]*types.Checkpoint)

	// Generate checkpoints every 10000 blocks
	for i := uint64(10000); i <= 100000; i += 10000 {
		checkpoints[i] = &types.Checkpoint{
			BlockNumber: i,
			BlockHash:   common.BytesToHash([]byte(fmt.Sprintf("blockhash-%d", i))),
			StateRoot:   common.BytesToHash([]byte(fmt.Sprintf("stateroot-%d", i))),
			Timestamp:   1700000000 + i,
			ChainID:     big.NewInt(137), // Polygon
			ValidatorSigs: []types.Signature{
				{
					ValidatorID: "validator1",
					PubKey:      []byte("pubkey1"),
					Signature:   []byte("sig1"),
				},
				{
					ValidatorID: "validator2",
					PubKey:      []byte("pubkey2"),
					Signature:   []byte("sig2"),
				},
				{
					ValidatorID: "validator3",
					PubKey:      []byte("pubkey3"),
					Signature:   []byte("sig3"),
				},
			},
		}
	}

	return checkpoints
}
