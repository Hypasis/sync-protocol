package main

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hypasis/sync-protocol/internal/types"
	"github.com/hypasis/sync-protocol/pkg/storage"
)

func main() {
	var (
		concurrency = flag.Int("concurrency", 100, "Number of concurrent simulated client workers")
		requests    = flag.Int("requests", 1000, "Total number of requests per worker")
		targetURL   = flag.String("url", "http://localhost:8080/api/v1/status", "Target URL endpoint to benchmark")
		mode        = flag.String("mode", "http", "Benchmark mode: 'http' or 'storage'")
	)
	flag.Parse()

	fmt.Printf("🚀 Hypasis Sync Protocol Benchmark Engine\n")
	fmt.Printf(" Mode: %s | Concurrency: %d workers | Requests/worker: %d | Total: %d\n\n",
		*mode, *concurrency, *requests, (*concurrency)*(*requests))

	if *mode == "storage" {
		runStorageBenchmark(*concurrency, *requests)
		return
	}

	runHTTPBenchmark(*targetURL, *concurrency, *requests)
}

func runHTTPBenchmark(targetURL string, concurrency, requestsPerWorker int) {
	totalRequests := concurrency * requestsPerWorker
	var (
		completedReqs uint64
		failedReqs    uint64
		latencies     = make([]time.Duration, 0, totalRequests)
		latMutex      sync.Mutex
		wg            sync.WaitGroup
		client        = &http.Client{Timeout: 5 * time.Second}
	)

	startTime := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < requestsPerWorker; i++ {
				reqStart := time.Now()
				resp, err := client.Get(targetURL)
				duration := time.Since(reqStart)

				if err != nil || resp == nil || resp.StatusCode >= 400 {
					atomic.AddUint64(&failedReqs, 1)
					if resp != nil {
						resp.Body.Close()
					}
					continue
				}
				resp.Body.Close()
				atomic.AddUint64(&completedReqs, 1)

				latMutex.Lock()
				latencies = append(latencies, duration)
				latMutex.Unlock()
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	printBenchmarkResults(totalRequests, completedReqs, failedReqs, totalDuration, latencies)
}

func runStorageBenchmark(concurrency, requestsPerWorker int) {
	store := storage.NewMemoryStorage()
	ctx := context.Background()

	// Seed block
	mockBlock := &types.Block{
		Header: &types.Header{
			Number:    100000,
			Hash:      common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
			GasLimit:  30000000,
			GasUsed:   21000,
			Timestamp: uint64(time.Now().Unix()),
		},
		Transactions: []*types.Transaction{
			{
				Hash:     common.HexToHash("0xa1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"),
				Value:    big.NewInt(1000),
				Gas:      21000,
				GasPrice: big.NewInt(30000000000),
			},
		},
	}
	_ = store.StoreBlock(ctx, mockBlock)

	totalRequests := concurrency * requestsPerWorker
	var (
		completedReqs uint64
		failedReqs    uint64
		latencies     = make([]time.Duration, 0, totalRequests)
		latMutex      sync.Mutex
		wg            sync.WaitGroup
	)

	startTime := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < requestsPerWorker; i++ {
				reqStart := time.Now()
				blk, err := store.GetBlock(ctx, 100000)
				duration := time.Since(reqStart)

				if err != nil || blk == nil {
					atomic.AddUint64(&failedReqs, 1)
					continue
				}
				atomic.AddUint64(&completedReqs, 1)

				latMutex.Lock()
				latencies = append(latencies, duration)
				latMutex.Unlock()
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	printBenchmarkResults(totalRequests, completedReqs, failedReqs, totalDuration, latencies)
}

func printBenchmarkResults(total int, success, failed uint64, duration time.Duration, latencies []time.Duration) {
	rps := float64(success) / duration.Seconds()

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	var p50, p95, p99 time.Duration
	if len(latencies) > 0 {
		p50 = latencies[len(latencies)*50/100]
		p95 = latencies[len(latencies)*95/100]
		p99 = latencies[len(latencies)*99/100]
	}

	fmt.Printf("📊 Benchmark Results:\n")
	fmt.Printf("  Total Executed:    %d\n", total)
	fmt.Printf("  Successful:        %d (%.2f%%)\n", success, float64(success)/float64(total)*100)
	fmt.Printf("  Failed:            %d\n", failed)
	fmt.Printf("  Total Duration:    %v\n", duration)
	fmt.Printf("  Throughput (RPS):  %.2f req/sec\n", rps)
	fmt.Printf("  Latency p50:       %v\n", p50)
	fmt.Printf("  Latency p95:       %v\n", p95)
	fmt.Printf("  Latency p99:       %v\n", p99)
}
