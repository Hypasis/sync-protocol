package p2p

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hypasis/sync-protocol/internal/types"
	"github.com/hypasis/sync-protocol/pkg/storage"
)

func TestDevP2PServer_InitAndEnode(t *testing.T) {
	store := storage.NewMemoryStorage()
	fetcher, err := NewBlockFetcher([]string{"https://polygon-rpc.com"}, 5*time.Second, 1)
	if err != nil {
		t.Fatalf("failed to create fetcher: %v", err)
	}

	cfg := &DevP2PConfig{
		ListenAddr: "127.0.0.1:0",
		NetworkID:  137,
		Name:       "hypasis-test-node",
		MaxPeers:   10,
	}

	server, err := NewDevP2PServer(cfg, store, fetcher)
	if err != nil {
		t.Fatalf("failed to create DevP2P server: %v", err)
	}

	if server == nil {
		t.Fatal("DevP2P server is nil")
	}

	if server.GetPeerCount() != 0 {
		t.Errorf("expected 0 connected peers, got %d", server.GetPeerCount())
	}
}

func TestDevP2PServer_BlockHeadersAndBodiesHandling(t *testing.T) {
	store := storage.NewMemoryStorage()
	ctx := context.Background()

	// Populate mock block in storage
	mockBlock := &types.Block{
		Header: &types.Header{
			Number:     100,
			Hash:       common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
			ParentHash: common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
			GasLimit:   30000000,
			GasUsed:    21000,
			Timestamp:  uint64(time.Now().Unix()),
		},
		Transactions: []*types.Transaction{
			{
				Hash:     common.HexToHash("0xa1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"),
				Nonce:    1,
				From:     common.HexToAddress("0x1111111111111111111111111111111111111111"),
				To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
				Value:    big.NewInt(1000),
				Gas:      21000,
				GasPrice: big.NewInt(30000000000),
			},
		},
	}

	if err := store.StoreBlock(ctx, mockBlock); err != nil {
		t.Fatalf("failed to store block: %v", err)
	}

	fetched, err := store.GetBlock(ctx, 100)
	if err != nil || fetched == nil {
		t.Fatalf("failed to fetch stored block: %v", err)
	}

	if fetched.Header.Number != 100 {
		t.Errorf("expected block number 100, got %d", fetched.Header.Number)
	}
}
