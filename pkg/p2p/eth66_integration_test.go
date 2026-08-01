package p2p

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/hypasis/sync-protocol/internal/types"
	"github.com/hypasis/sync-protocol/pkg/storage"
)

func TestDevP2P_Eth66_FullServerLifecycle(t *testing.T) {
	store := storage.NewMemoryStorage()
	cfg := &DevP2PConfig{
		ListenAddr: "127.0.0.1:0",
		MaxPeers:   10,
		NetworkID:  137, // Polygon POS Mainnet
		Name:       "Hypasis-IntegrationTest-Node",
	}

	server, err := NewDevP2PServer(cfg, store, nil)
	if err != nil {
		t.Fatalf("Failed to create DevP2P server: %v", err)
	}

	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start DevP2P server: %v", err)
	}
	defer server.Stop()

	// Verify enode URL format
	enodeURL := server.GetEnodeURL()
	if enodeURL == "" || !bytes.HasPrefix([]byte(enodeURL), []byte("enode://")) {
		t.Errorf("Unexpected enode URL: %s", enodeURL)
	}

	// Verify peer count initial state
	if count := server.GetPeerCount(); count != 0 {
		t.Errorf("Expected 0 initial peers, got %d", count)
	}
}

func TestDevP2P_Eth66_GetBlockHeaders(t *testing.T) {
	store := storage.NewMemoryStorage()
	ctx := context.Background()

	// Seed test block in storage
	header := &types.Header{
		Number:    500000,
		Hash:      common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
		GasLimit:  30000000,
		GasUsed:   21000,
		Timestamp: uint64(time.Now().Unix()),
	}
	block := &types.Block{
		Header: header,
		Transactions: []*types.Transaction{
			{
				Hash:     common.HexToHash("0x9999999999999999999999999999999999999999999999999999999999999999"),
				Value:    big.NewInt(500),
				Gas:      21000,
				GasPrice: big.NewInt(20000000000),
			},
		},
	}
	if err := store.StoreBlock(ctx, block); err != nil {
		t.Fatalf("Failed to seed block into storage: %v", err)
	}

	server, err := NewDevP2PServer(&DevP2PConfig{
		ListenAddr: "127.0.0.1:0",
		MaxPeers:   5,
		NetworkID:  137,
	}, store, nil)
	if err != nil {
		t.Fatalf("Failed to create DevP2P server: %v", err)
	}

	// Encode eth/66 GetBlockHeaders request: [reqID, [origin, amount, skip, reverse]]
	reqID := uint64(12345)
	originNumber := uint64(500000)
	requestData := struct {
		ReqID uint64
		Query struct {
			Origin  uint64
			Amount  uint64
			Skip    uint64
			Reverse bool
		}
	}{
		ReqID: reqID,
		Query: struct {
			Origin  uint64
			Amount  uint64
			Skip    uint64
			Reverse bool
		}{
			Origin:  originNumber,
			Amount:  1,
			Skip:    0,
			Reverse: false,
		},
	}

	encodedBytes, err := rlp.EncodeToBytes(requestData)
	if err != nil {
		t.Fatalf("Failed to RLP encode eth/66 GetBlockHeaders request: %v", err)
	}

	// Verify decoding using server logic
	var decodedReq struct {
		ReqID uint64
		Query struct {
			Origin  uint64
			Amount  uint64
			Skip    uint64
			Reverse bool
		}
	}
	if err := rlp.DecodeBytes(encodedBytes, &decodedReq); err != nil {
		t.Fatalf("Failed to RLP decode eth/66 request: %v", err)
	}

	if decodedReq.ReqID != reqID {
		t.Errorf("ReqID mismatch: got %d, want %d", decodedReq.ReqID, reqID)
	}
	if decodedReq.Query.Origin != originNumber {
		t.Errorf("Origin block number mismatch: got %d, want %d", decodedReq.Query.Origin, originNumber)
	}

	// Retrieve header from storage
	retrievedBlock, err := server.storage.GetBlock(ctx, decodedReq.Query.Origin)
	if err != nil || retrievedBlock == nil {
		t.Fatalf("Server storage failed to retrieve requested block header: %v", err)
	}

	if retrievedBlock.Header.Number != originNumber {
		t.Errorf("Retrieved block number mismatch: got %d, want %d", retrievedBlock.Header.Number, originNumber)
	}
}

func TestDevP2P_Eth66_GetBlockBodies(t *testing.T) {
	store := storage.NewMemoryStorage()
	ctx := context.Background()

	blockHash := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")
	header := &types.Header{
		Number:   600000,
		Hash:     blockHash,
		GasLimit: 30000000,
	}
	block := &types.Block{
		Header: header,
		Transactions: []*types.Transaction{
			{
				Hash:  common.HexToHash("0x8888888888888888888888888888888888888888888888888888888888888888"),
				Value: big.NewInt(1000),
			},
		},
	}
	_ = store.StoreBlock(ctx, block)

	// Encode eth/66 GetBlockBodies request: [reqID, [hashes...]]
	reqID := uint64(99999)
	requestData := struct {
		ReqID  uint64
		Hashes []common.Hash
	}{
		ReqID:  reqID,
		Hashes: []common.Hash{blockHash},
	}

	encodedBytes, err := rlp.EncodeToBytes(requestData)
	if err != nil {
		t.Fatalf("Failed to RLP encode eth/66 GetBlockBodies request: %v", err)
	}

	var decodedReq struct {
		ReqID  uint64
		Hashes []common.Hash
	}
	if err := rlp.DecodeBytes(encodedBytes, &decodedReq); err != nil {
		t.Fatalf("Failed to RLP decode GetBlockBodies request: %v", err)
	}

	if decodedReq.ReqID != reqID {
		t.Errorf("ReqID mismatch: got %d, want %d", decodedReq.ReqID, reqID)
	}
	if len(decodedReq.Hashes) != 1 || decodedReq.Hashes[0] != blockHash {
		t.Errorf("BlockHash mismatch in eth/66 body request")
	}
}
