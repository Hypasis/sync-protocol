package p2p

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	internalTypes "github.com/hypasis/sync-protocol/internal/types"
)

// BlockFetcher fetches blocks from upstream RPC endpoints
type BlockFetcher struct {
	clients        []*ethclient.Client
	endpoints      []string
	currentIndex   int
	timeout        time.Duration
	maxRetries     int
	mu             sync.RWMutex
	healthStatus   map[string]bool
	requestCount   map[string]uint64
	errorCount     map[string]uint64
}

// BlockFetcherConfig configures the block fetcher
type BlockFetcherConfig struct {
	UpstreamRPCs []string
	Timeout      time.Duration
	MaxRetries   int
}

// NewBlockFetcher creates a new block fetcher
func NewBlockFetcher(endpoints []string, timeout time.Duration, maxRetries int) (*BlockFetcher, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no upstream RPC endpoints provided")
	}

	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if maxRetries == 0 {
		maxRetries = 3
	}

	fetcher := &BlockFetcher{
		clients:      make([]*ethclient.Client, 0, len(endpoints)),
		endpoints:    endpoints,
		timeout:      timeout,
		maxRetries:   maxRetries,
		healthStatus: make(map[string]bool),
		requestCount: make(map[string]uint64),
		errorCount:   make(map[string]uint64),
	}

	// Initialize clients for all endpoints
	for _, endpoint := range endpoints {
		client, err := ethclient.Dial(endpoint)
		if err != nil {
			fmt.Printf("Warning: Failed to connect to %s: %v\n", endpoint, err)
			fetcher.healthStatus[endpoint] = false
			continue
		}
		fetcher.clients = append(fetcher.clients, client)
		fetcher.healthStatus[endpoint] = true
	}

	if len(fetcher.clients) == 0 {
		return nil, fmt.Errorf("failed to connect to any upstream RPC endpoint")
	}

	return fetcher, nil
}

// FetchBlock fetches a single block by number
func (f *BlockFetcher) FetchBlock(ctx context.Context, blockNum uint64) (*internalTypes.Block, error) {
	var lastErr error

	for retry := 0; retry < f.maxRetries; retry++ {
		client := f.getNextHealthyClient()
		if client == nil {
			return nil, fmt.Errorf("no healthy clients available")
		}

		// Create context with timeout
		fetchCtx, cancel := context.WithTimeout(ctx, f.timeout)

		// Fetch block from Ethereum client
		ethBlock, err := client.BlockByNumber(fetchCtx, big.NewInt(int64(blockNum)))
		cancel()

		if err == nil {
			// Convert to internal block type
			block := convertEthBlockToInternal(ethBlock)

			// Update metrics
			f.incrementRequestCount(f.getCurrentEndpoint())

			return block, nil
		}

		lastErr = err
		f.incrementErrorCount(f.getCurrentEndpoint())

		// Mark endpoint as unhealthy if too many errors
		if f.getErrorRate(f.getCurrentEndpoint()) > 0.5 {
			f.markUnhealthy(f.getCurrentEndpoint())
		}

		// Wait before retry with exponential backoff
		if retry < f.maxRetries-1 {
			backoff := time.Duration(1<<uint(retry)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return nil, fmt.Errorf("failed to fetch block %d after %d retries: %w", blockNum, f.maxRetries, lastErr)
}

// FetchBlocks fetches a range of blocks in batch
func (f *BlockFetcher) FetchBlocks(ctx context.Context, start, end uint64) ([]*internalTypes.Block, error) {
	if start >= end {
		return nil, fmt.Errorf("invalid range: start=%d, end=%d", start, end)
	}

	count := end - start
	blocks := make([]*internalTypes.Block, 0, count)
	errChan := make(chan error, count)
	blockChan := make(chan *internalTypes.Block, count)

	// Limit concurrent fetches
	maxConcurrent := 10
	semaphore := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup

	// Fetch blocks concurrently
	for i := start; i < end; i++ {
		wg.Add(1)
		go func(blockNum uint64) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			block, err := f.FetchBlock(ctx, blockNum)
			if err != nil {
				errChan <- fmt.Errorf("block %d: %w", blockNum, err)
				return
			}
			blockChan <- block
		}(i)
	}

	// Wait for all fetches to complete
	go func() {
		wg.Wait()
		close(blockChan)
		close(errChan)
	}()

	// Collect results
	var errors []error
	for {
		select {
		case block, ok := <-blockChan:
			if !ok {
				blockChan = nil
			} else {
				blocks = append(blocks, block)
			}
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
			} else {
				errors = append(errors, err)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		if blockChan == nil && errChan == nil {
			break
		}
	}

	// Return error if we got too many failures
	if len(errors) > 0 {
		if len(errors) > int(count)/2 {
			return nil, fmt.Errorf("too many errors fetching blocks: %d/%d failed", len(errors), count)
		}
		// Log errors but continue with partial results
		fmt.Printf("Warning: %d blocks failed to fetch\n", len(errors))
	}

	return blocks, nil
}

// FetchLatestBlockNumber fetches the latest block number from the network
func (f *BlockFetcher) FetchLatestBlockNumber(ctx context.Context) (uint64, error) {
	var lastErr error

	for retry := 0; retry < f.maxRetries; retry++ {
		client := f.getNextHealthyClient()
		if client == nil {
			return 0, fmt.Errorf("no healthy clients available")
		}

		fetchCtx, cancel := context.WithTimeout(ctx, f.timeout)
		header, err := client.HeaderByNumber(fetchCtx, nil)
		cancel()

		if err == nil {
			f.incrementRequestCount(f.getCurrentEndpoint())
			return header.Number.Uint64(), nil
		}

		lastErr = err
		f.incrementErrorCount(f.getCurrentEndpoint())

		if retry < f.maxRetries-1 {
			backoff := time.Duration(1<<uint(retry)) * time.Second
			time.Sleep(backoff)
		}
	}

	return 0, fmt.Errorf("failed to fetch latest block number: %w", lastErr)
}

// getNextHealthyClient returns the next healthy client using round-robin
func (f *BlockFetcher) getNextHealthyClient() *ethclient.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.clients) == 0 {
		return nil
	}

	// Try to find a healthy client
	startIndex := f.currentIndex
	for {
		if f.currentIndex >= len(f.clients) {
			f.currentIndex = 0
		}

		endpoint := f.endpoints[f.currentIndex]
		if f.healthStatus[endpoint] {
			client := f.clients[f.currentIndex]
			f.currentIndex++
			return client
		}

		f.currentIndex++
		if f.currentIndex == startIndex {
			// No healthy clients found
			return nil
		}
	}
}

// getCurrentEndpoint returns the current endpoint
func (f *BlockFetcher) getCurrentEndpoint() string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	index := f.currentIndex - 1
	if index < 0 {
		index = 0
	}
	if index >= len(f.endpoints) {
		index = len(f.endpoints) - 1
	}

	return f.endpoints[index]
}

// incrementRequestCount increments the request count for an endpoint
func (f *BlockFetcher) incrementRequestCount(endpoint string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requestCount[endpoint]++
}

// incrementErrorCount increments the error count for an endpoint
func (f *BlockFetcher) incrementErrorCount(endpoint string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errorCount[endpoint]++
}

// getErrorRate returns the error rate for an endpoint
func (f *BlockFetcher) getErrorRate(endpoint string) float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	requests := f.requestCount[endpoint]
	errors := f.errorCount[endpoint]

	if requests == 0 {
		return 0
	}

	return float64(errors) / float64(requests)
}

// markUnhealthy marks an endpoint as unhealthy
func (f *BlockFetcher) markUnhealthy(endpoint string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.healthStatus[endpoint] = false
	fmt.Printf("Marked endpoint %s as unhealthy\n", endpoint)
}

// Close closes all client connections
func (f *BlockFetcher) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, client := range f.clients {
		client.Close()
	}
	f.clients = nil
}

// convertEthBlockToInternal converts go-ethereum Block to internal Block type
func convertEthBlockToInternal(ethBlock *types.Block) *internalTypes.Block {
	// Serialize transactions
	// Convert transactions
	txs := make([]*internalTypes.Transaction, len(ethBlock.Transactions()))
	for i, tx := range ethBlock.Transactions() {
		txs[i] = &internalTypes.Transaction{
			Hash:     tx.Hash(),
			Nonce:    tx.Nonce(),
			To:       *tx.To(),
			Value:    tx.Value(),
			Gas:      tx.Gas(),
			GasPrice: tx.GasPrice(),
			Data:     tx.Data(),
		}
	}

	// Create block with header
	return &internalTypes.Block{
		Header: &internalTypes.Header{
			Number:      ethBlock.NumberU64(),
			Hash:        ethBlock.Hash(),
			ParentHash:  ethBlock.ParentHash(),
			Coinbase:    ethBlock.Coinbase(),
			StateRoot:   ethBlock.Root(),
			TxHash:      ethBlock.TxHash(),
			ReceiptHash: ethBlock.ReceiptHash(),
			GasLimit:    ethBlock.GasLimit(),
			GasUsed:     ethBlock.GasUsed(),
			Timestamp:   ethBlock.Time(),
			ExtraData:   ethBlock.Extra(),
			MixDigest:   ethBlock.MixDigest(),
			Nonce:       ethBlock.Nonce(),
		},
		Transactions: txs,
		Size:         ethBlock.Size(),
	}
}

// GetStats returns statistics about the fetcher
func (f *BlockFetcher) GetStats() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["endpoints"] = len(f.endpoints)
	stats["healthy_endpoints"] = 0

	for endpoint, healthy := range f.healthStatus {
		if healthy {
			stats["healthy_endpoints"] = stats["healthy_endpoints"].(int) + 1
		}

		stats[endpoint] = map[string]interface{}{
			"healthy":  healthy,
			"requests": f.requestCount[endpoint],
			"errors":   f.errorCount[endpoint],
		}
	}

	return stats
}
