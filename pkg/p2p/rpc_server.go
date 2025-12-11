package p2p

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/hypasis/sync-protocol/pkg/storage"
	internalTypes "github.com/hypasis/sync-protocol/internal/types"
)

// RPCServer serves blocks to Bor clients via JSON-RPC
type RPCServer struct {
	storage     storage.Storage
	fetcher     *BlockFetcher
	server      *rpc.Server
	httpServer  *http.Server
	listenAddr  string
	chainID     *big.Int
}

// RPCConfig configures the RPC server
type RPCConfig struct {
	Listen  string
	ChainID int64
}

// NewRPCServer creates a new RPC server
func NewRPCServer(cfg *RPCConfig, store storage.Storage, fetcher *BlockFetcher) *RPCServer {
	return &RPCServer{
		storage:    store,
		fetcher:    fetcher,
		listenAddr: cfg.Listen,
		chainID:    big.NewInt(cfg.ChainID),
	}
}

// Start starts the RPC server
func (s *RPCServer) Start(ctx context.Context) error {
	// Create RPC server
	s.server = rpc.NewServer()

	// Register eth namespace
	ethAPI := &EthAPI{
		storage: s.storage,
		fetcher: s.fetcher,
		chainID: s.chainID,
	}
	if err := s.server.RegisterName("eth", ethAPI); err != nil {
		return fmt.Errorf("failed to register eth API: %w", err)
	}

	// Register net namespace
	netAPI := &NetAPI{chainID: s.chainID}
	if err := s.server.RegisterName("net", netAPI); err != nil {
		return fmt.Errorf("failed to register net API: %w", err)
	}

	// Register web3 namespace
	web3API := &Web3API{}
	if err := s.server.RegisterName("web3", web3API); err != nil {
		return fmt.Errorf("failed to register web3 API: %w", err)
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle("/", s.server)

	s.httpServer = &http.Server{
		Addr:    s.listenAddr,
		Handler: mux,
	}

	// Start listening
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.listenAddr, err)
	}

	fmt.Printf("🌐 RPC server listening on %s\n", s.listenAddr)

	// Start server in background
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("RPC server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the RPC server
func (s *RPCServer) Stop() error {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// EthAPI implements the eth_ RPC methods
type EthAPI struct {
	storage storage.Storage
	fetcher *BlockFetcher
	chainID *big.Int
}

// BlockNumber returns the latest block number
func (api *EthAPI) BlockNumber() (hexutil.Uint64, error) {
	maxBlock := api.storage.GetMaxBlock()
	return hexutil.Uint64(maxBlock), nil
}

// GetBlockByNumber returns a block by number
func (api *EthAPI) GetBlockByNumber(ctx context.Context, number rpc.BlockNumber, fullTx bool) (map[string]interface{}, error) {
	var blockNum uint64

	if number == rpc.LatestBlockNumber || number == rpc.PendingBlockNumber {
		blockNum = api.storage.GetMaxBlock()
	} else if number == rpc.EarliestBlockNumber {
		blockNum = api.storage.GetMinBlock()
	} else {
		blockNum = uint64(number)
	}

	// Try to get from storage first
	block, err := api.storage.GetBlock(ctx, blockNum)
	if err != nil {
		// If not in storage, try to fetch from upstream
		if api.fetcher != nil {
			block, err = api.fetcher.FetchBlock(ctx, blockNum)
			if err != nil {
				return nil, fmt.Errorf("block not found: %w", err)
			}

			// Store the fetched block
			if storeErr := api.storage.StoreBlock(ctx, block); storeErr != nil {
				fmt.Printf("Warning: failed to store fetched block: %v\n", storeErr)
			}
		} else {
			return nil, fmt.Errorf("block %d not found", blockNum)
		}
	}

	return blockToRPCResponse(block, fullTx), nil
}

// GetBlockByHash returns a block by hash
func (api *EthAPI) GetBlockByHash(ctx context.Context, hash common.Hash, fullTx bool) (map[string]interface{}, error) {
	block, err := api.storage.GetBlockByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("block not found: %w", err)
	}

	return blockToRPCResponse(block, fullTx), nil
}

// Syncing returns sync status or false if not syncing
func (api *EthAPI) Syncing() (interface{}, error) {
	minBlock := api.storage.GetMinBlock()
	maxBlock := api.storage.GetMaxBlock()
	gaps := api.storage.GetGaps()

	// If we have gaps, we're syncing
	if len(gaps) > 0 {
		return map[string]interface{}{
			"startingBlock": hexutil.Uint64(minBlock),
			"currentBlock":  hexutil.Uint64(maxBlock),
			"highestBlock":  hexutil.Uint64(maxBlock + 1000), // Estimate
		}, nil
	}

	// Not syncing
	return false, nil
}

// ChainId returns the chain ID
func (api *EthAPI) ChainId() (*hexutil.Big, error) {
	return (*hexutil.Big)(api.chainID), nil
}

// GetBalance returns account balance (placeholder)
func (api *EthAPI) GetBalance(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (*hexutil.Big, error) {
	// Placeholder - would need state database for real implementation
	return (*hexutil.Big)(big.NewInt(0)), nil
}

// GetTransactionCount returns account nonce (placeholder)
func (api *EthAPI) GetTransactionCount(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (*hexutil.Uint64, error) {
	// Placeholder - would need state database
	count := hexutil.Uint64(0)
	return &count, nil
}

// NetAPI implements the net_ RPC methods
type NetAPI struct {
	chainID *big.Int
}

// Version returns the network ID
func (api *NetAPI) Version() string {
	return api.chainID.String()
}

// Listening returns true if client is actively listening for network connections
func (api *NetAPI) Listening() bool {
	return true
}

// PeerCount returns the number of connected peers
func (api *NetAPI) PeerCount() hexutil.Uint {
	// Placeholder - would track actual peers
	return hexutil.Uint(0)
}

// Web3API implements the web3_ RPC methods
type Web3API struct{}

// ClientVersion returns the client version
func (api *Web3API) ClientVersion() string {
	return "Hypasis/v0.1.0"
}

// Sha3 returns Keccak-256 hash of the given data
func (api *Web3API) Sha3(input hexutil.Bytes) hexutil.Bytes {
	return common.BytesToHash(input).Bytes()
}

// blockToRPCResponse converts internal block to RPC response format
func blockToRPCResponse(block *internalTypes.Block, fullTx bool) map[string]interface{} {
	response := map[string]interface{}{
		"number":           hexutil.Uint64(block.Header.Number),
		"hash":             block.Header.Hash,
		"parentHash":       block.Header.ParentHash,
		"stateRoot":        block.Header.StateRoot,
		"timestamp":        hexutil.Uint64(block.Header.Timestamp),
		"size":             hexutil.Uint64(block.Size),
		"transactionsRoot": block.Header.TxHash,
		"receiptsRoot":     block.Header.ReceiptHash,
		"miner":            block.Header.Coinbase,
		"difficulty":       (*hexutil.Big)(big.NewInt(0)), // Placeholder
		"totalDifficulty":  (*hexutil.Big)(big.NewInt(0)), // Placeholder
		"gasLimit":         hexutil.Uint64(block.Header.GasLimit),
		"gasUsed":          hexutil.Uint64(block.Header.GasUsed),
		"extraData":        hexutil.Bytes(block.Header.ExtraData),
		"nonce":            hexutil.Uint64(block.Header.Nonce),
	}

	// Add transactions
	if fullTx {
		// Return full transaction objects
		txs := make([]map[string]interface{}, len(block.Transactions))
		for i, tx := range block.Transactions {
			txs[i] = map[string]interface{}{
				"hash":     tx.Hash,
				"nonce":    hexutil.Uint64(tx.Nonce),
				"from":     tx.From,
				"to":       tx.To,
				"value":    (*hexutil.Big)(tx.Value),
				"gas":      hexutil.Uint64(tx.Gas),
				"gasPrice": (*hexutil.Big)(tx.GasPrice),
				"input":    hexutil.Bytes(tx.Data),
			}
		}
		response["transactions"] = txs
	} else {
		// Just return transaction hashes
		txHashes := make([]common.Hash, len(block.Transactions))
		for i, tx := range block.Transactions {
			txHashes[i] = tx.Hash
		}
		response["transactions"] = txHashes
	}

	return response
}
