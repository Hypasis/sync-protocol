package p2p

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionPool manages a pool of connections to handle massive concurrent requests
type ConnectionPool struct {
	maxConnections int
	activeConns    atomic.Int64
	waitingConns   atomic.Int64
	totalRequests  atomic.Uint64

	semaphore chan struct{}
	metrics   *PoolMetrics

	mu      sync.RWMutex
	clients map[string]*PooledConnection
}

// PooledConnection represents a connection in the pool
type PooledConnection struct {
	ID           string
	ClientIP     string
	UserAgent    string
	Connected    time.Time
	LastActivity time.Time
	RequestCount uint64
	BytesSent    uint64
	BytesRecv    uint64
	Active       bool
}

// PoolMetrics tracks connection pool metrics
type PoolMetrics struct {
	TotalConnections    atomic.Uint64
	ActiveConnections   atomic.Int64
	RejectedConnections atomic.Uint64
	AvgWaitTime         atomic.Int64 // in milliseconds
	PeakConnections     atomic.Int64
	TotalRequests       atomic.Uint64
	RequestsPerSecond   atomic.Uint64
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(maxConnections int) *ConnectionPool {
	return &ConnectionPool{
		maxConnections: maxConnections,
		semaphore:      make(chan struct{}, maxConnections),
		clients:        make(map[string]*PooledConnection),
		metrics:        &PoolMetrics{},
	}
}

// Acquire acquires a connection slot from the pool
func (p *ConnectionPool) Acquire(ctx context.Context, clientID, clientIP, userAgent string) (*PooledConnection, error) {
	startWait := time.Now()
	p.waitingConns.Add(1)
	defer p.waitingConns.Add(-1)

	// Try to acquire semaphore slot
	select {
	case p.semaphore <- struct{}{}:
		// Acquired slot
		waitDuration := time.Since(startWait)
		p.metrics.AvgWaitTime.Store(waitDuration.Milliseconds())

		conn := &PooledConnection{
			ID:           clientID,
			ClientIP:     clientIP,
			UserAgent:    userAgent,
			Connected:    time.Now(),
			LastActivity: time.Now(),
			Active:       true,
		}

		p.mu.Lock()
		p.clients[clientID] = conn
		p.mu.Unlock()

		activeCount := p.activeConns.Add(1)
		p.metrics.ActiveConnections.Store(activeCount)
		p.metrics.TotalConnections.Add(1)

		// Update peak connections
		for {
			peak := p.metrics.PeakConnections.Load()
			if activeCount <= peak {
				break
			}
			if p.metrics.PeakConnections.CompareAndSwap(peak, activeCount) {
				break
			}
		}

		return conn, nil

	case <-ctx.Done():
		p.metrics.RejectedConnections.Add(1)
		return nil, fmt.Errorf("connection pool timeout: %w", ctx.Err())
	}
}

// Release releases a connection slot back to the pool
func (p *ConnectionPool) Release(clientID string) {
	p.mu.Lock()
	if conn, ok := p.clients[clientID]; ok {
		conn.Active = false
		delete(p.clients, clientID)
	}
	p.mu.Unlock()

	<-p.semaphore
	activeCount := p.activeConns.Add(-1)
	p.metrics.ActiveConnections.Store(activeCount)
}

// RecordRequest records a request from a connection
func (p *ConnectionPool) RecordRequest(clientID string, bytesSent, bytesRecv uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, ok := p.clients[clientID]; ok {
		conn.RequestCount++
		conn.BytesSent += bytesSent
		conn.BytesRecv += bytesRecv
		conn.LastActivity = time.Now()
	}

	p.metrics.TotalRequests.Add(1)
}

// GetMetrics returns current pool metrics
func (p *ConnectionPool) GetMetrics() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	activeConns := make([]map[string]interface{}, 0, len(p.clients))
	for _, conn := range p.clients {
		if conn.Active {
			activeConns = append(activeConns, map[string]interface{}{
				"id":            conn.ID,
				"client_ip":     conn.ClientIP,
				"user_agent":    conn.UserAgent,
				"connected":     conn.Connected.Unix(),
				"last_activity": conn.LastActivity.Unix(),
				"requests":      conn.RequestCount,
				"bytes_sent":    conn.BytesSent,
				"bytes_recv":    conn.BytesRecv,
			})
		}
	}

	return map[string]interface{}{
		"max_connections":       p.maxConnections,
		"active_connections":    p.activeConns.Load(),
		"waiting_connections":   p.waitingConns.Load(),
		"total_connections":     p.metrics.TotalConnections.Load(),
		"rejected_connections":  p.metrics.RejectedConnections.Load(),
		"peak_connections":      p.metrics.PeakConnections.Load(),
		"total_requests":        p.metrics.TotalRequests.Load(),
		"requests_per_second":   p.metrics.RequestsPerSecond.Load(),
		"avg_wait_time_ms":      p.metrics.AvgWaitTime.Load(),
		"active_clients":        activeConns,
		"utilization_percent":   float64(p.activeConns.Load()) / float64(p.maxConnections) * 100,
	}
}

// GetActiveCount returns the number of active connections
func (p *ConnectionPool) GetActiveCount() int64 {
	return p.activeConns.Load()
}

// GetWaitingCount returns the number of waiting connections
func (p *ConnectionPool) GetWaitingCount() int64 {
	return p.waitingConns.Load()
}

// IsAvailable checks if the pool has available slots
func (p *ConnectionPool) IsAvailable() bool {
	return p.activeConns.Load() < int64(p.maxConnections)
}

// GetUtilization returns the pool utilization percentage
func (p *ConnectionPool) GetUtilization() float64 {
	return float64(p.activeConns.Load()) / float64(p.maxConnections) * 100
}

// CleanupStale removes stale connections (inactive for > 5 minutes)
func (p *ConnectionPool) CleanupStale(maxIdleTime time.Duration) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	removed := 0
	now := time.Now()

	for id, conn := range p.clients {
		if conn.Active && now.Sub(conn.LastActivity) > maxIdleTime {
			conn.Active = false
			delete(p.clients, id)
			p.activeConns.Add(-1)
			<-p.semaphore
			removed++
		}
	}

	if removed > 0 {
		p.metrics.ActiveConnections.Store(p.activeConns.Load())
	}

	return removed
}

// GetConnectionByID returns connection details by ID
func (p *ConnectionPool) GetConnectionByID(clientID string) *PooledConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if conn, ok := p.clients[clientID]; ok {
		return conn
	}
	return nil
}

// GetAllConnections returns all active connections
func (p *ConnectionPool) GetAllConnections() []*PooledConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conns := make([]*PooledConnection, 0, len(p.clients))
	for _, conn := range p.clients {
		if conn.Active {
			conns = append(conns, conn)
		}
	}
	return conns
}

// UpdateRequestsPerSecond updates the requests per second metric
func (p *ConnectionPool) UpdateRequestsPerSecond(rps uint64) {
	p.metrics.RequestsPerSecond.Store(rps)
}

// StartMetricsCollector starts background metrics collection
func (p *ConnectionPool) StartMetricsCollector(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastRequests uint64

	for {
		select {
		case <-ticker.C:
			currentRequests := p.metrics.TotalRequests.Load()
			rps := currentRequests - lastRequests
			p.metrics.RequestsPerSecond.Store(rps)
			lastRequests = currentRequests

			// Cleanup stale connections every 60 seconds
			if time.Now().Unix()%60 == 0 {
				removed := p.CleanupStale(5 * time.Minute)
				if removed > 0 {
					fmt.Printf("🧹 Cleaned up %d stale connections\n", removed)
				}
			}

		case <-ctx.Done():
			return
		}
	}
}
